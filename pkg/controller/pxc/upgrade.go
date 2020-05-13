package pxc

import (
	"context"
	"crypto/md5"
	"fmt"
	"sort"
	"strings"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, initContainers []corev1.Container) error {
	currentSet := sfs.StatefulSet()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)
	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	currentSet.Spec.UpdateStrategy = sfs.UpdateStrategy(cr)

	// change the pod size
	currentSet.Spec.Replicas = &podSpec.Size
	currentSet.Spec.Template.Spec.SecurityContext = podSpec.PodSecurityContext

	// embed DB configuration hash
	// TODO: code duplication with deploy function
	configHash := r.getConfigHash(cr)
	if currentSet.Spec.Template.Annotations == nil {
		currentSet.Spec.Template.Annotations = make(map[string]string)
	}
	if cr.CompareVersionWith("1.1.0") >= 0 {
		currentSet.Spec.Template.Annotations["percona.com/configuration-hash"] = configHash
	}

	err = r.reconcileConfigMap(cr)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update db config error: %v", err)
	}

	// change TLS secret configuration
	sslHash, err := r.getTLSHash(cr, cr.Spec.PXC.SSLSecretName)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	if cr.CompareVersionWith("1.1.0") >= 0 {
		currentSet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash
	}

	sslInternalHash, err := r.getTLSHash(cr, cr.Spec.PXC.SSLInternalSecretName)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	if !errors.IsNotFound(err) && cr.CompareVersionWith("1.1.0") >= 0 {
		currentSet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
	}

	var newContainers []corev1.Container
	var newInitContainers []corev1.Container

	// pmm container
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		pmmC, err := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName, cr)
		if err != nil {
			return fmt.Errorf("pmm container error: %v", err)
		}

		newContainers = append(newContainers, pmmC)
	}

	// application container
	appC, err := sfs.AppContainer(podSpec, cr.Spec.SecretsName, cr)
	if err != nil {
		return fmt.Errorf("app container error: %v", err)
	}

	newContainers = append(newContainers, appC)

	if len(initContainers) > 0 {
		newInitContainers = append(newInitContainers, initContainers...)
	}

	if podSpec.ForceUnsafeBootstrap {
		ic := appC.DeepCopy()
		ic.Name = ic.Name + "-init-unsafe"
		ic.ReadinessProbe = nil
		ic.LivenessProbe = nil
		ic.Command = []string{"/var/lib/mysql/unsafe-bootstrap.sh"}
		newInitContainers = append(newInitContainers, *ic)
	}

	// sidecars
	sideC, err := sfs.SidecarContainers(podSpec, cr.Spec.SecretsName)
	if err != nil {
		return fmt.Errorf("sidecar container error: %v", err)
	}
	newContainers = append(newContainers, sideC...)

	// volumes
	sfsVolume, err := sfs.Volumes(podSpec, cr)
	if err != nil {
		return fmt.Errorf("volumes error: %v", err)
	}

	currentSet.Spec.Template.Spec.Containers = newContainers
	currentSet.Spec.Template.Spec.InitContainers = newInitContainers
	currentSet.Spec.Template.Spec.Affinity = pxc.PodAffinity(podSpec.Affinity, sfs)
	currentSet.Spec.Template.Spec.Volumes = sfsVolume.Volumes

	err = r.client.Update(context.TODO(), currentSet)
	if err != nil {
		return fmt.Errorf("update error: %v", err)
	}

	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType {
		return nil
	}

	return r.smartUpdate(sfs, cr)
}

func (r *ReconcilePerconaXtraDBCluster) smartUpdate(sfs api.StatefulApp, cr *api.PerconaXtraDBCluster) error {
	if !isPXC(sfs) {
		return nil
	}

	if sfs.StatefulSet().Status.UpdatedReplicas >= sfs.StatefulSet().Status.Replicas {
		return nil
	}

	log.Info("statefullSet was changed, run smart update")

	if err := r.isBackupRunning(cr); err != nil {
		log.Error(err, "can't start 'SmartUpdate'")
		return nil
	}

	if sfs.StatefulSet().Status.ReadyReplicas < sfs.StatefulSet().Status.Replicas {
		return fmt.Errorf("can't start/continue 'SmartUpdate': waiting for all replicas are ready")
	}

	list := corev1.PodList{}
	if err := r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace:     sfs.StatefulSet().Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
		&list,
	); err != nil {
		return fmt.Errorf("get pod list: %v", err)
	}

	primary, err := r.getPrimaryPod(cr)
	if err != nil {
		return fmt.Errorf("get primary pod: %v", err)
	}

	log.Info(fmt.Sprintf("primary pod is %s", primary))

	waitLimit := 120
	if cr.Spec.PXC.LivenessInitialDelaySeconds != nil {
		waitLimit = int(*cr.Spec.PXC.LivenessInitialDelaySeconds)
	}

	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name > list.Items[j].Name
	})

	var primaryPod corev1.Pod
	for _, pod := range list.Items {
		pod := pod
		if strings.HasPrefix(primary, fmt.Sprintf("%s.%s.%s", pod.Name, sfs.StatefulSet().Name, sfs.StatefulSet().Namespace)) {
			primaryPod = pod
		} else {
			log.Info(fmt.Sprintf("apply changes to secondary pod %s", pod.Name))
			if err := r.applyNWait(cr, sfs.StatefulSet().Status.UpdateRevision, &pod, waitLimit); err != nil {
				return fmt.Errorf("failed to apply changes: %v", err)
			}
		}
	}

	log.Info(fmt.Sprintf("apply changes to primary pod %s", primaryPod.Name))
	if err := r.applyNWait(cr, sfs.StatefulSet().Status.UpdateRevision, &primaryPod, waitLimit); err != nil {
		return fmt.Errorf("failed to apply changes: %v", err)
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) applyNWait(cr *api.PerconaXtraDBCluster, updateRevision string, pod *corev1.Pod, waitLimit int) error {
	if pod.ObjectMeta.Labels["controller-revision-hash"] == updateRevision {
		log.Info(fmt.Sprintf("pod %s is already updated", pod.Name))
	} else {
		if err := r.client.Delete(context.TODO(), pod); err != nil {
			return fmt.Errorf("failed to delete pod: %v", err)
		}
	}

	if err := r.waitPodRestart(updateRevision, pod, waitLimit); err != nil {
		return fmt.Errorf("failed to wait pod: %v", err)
	}

	if err := r.waitPXCSynced(cr, pod.Status.PodIP, waitLimit); err != nil {
		return fmt.Errorf("failed to wait pxc sync: %v", err)
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) getPrimaryPod(cr *api.PerconaXtraDBCluster) (string, error) {
	user := "proxyadmin"
	host := fmt.Sprintf("%s-proxysql-unready.%s", cr.ObjectMeta.Name, cr.Namespace)

	var dbatabase queries.Database

	for i := 0; ; i++ {
		db, err := queries.New(r.client, cr.Namespace, cr.Spec.SecretsName, user, host, 6032)
		if err != nil && i < int(cr.Spec.ProxySQL.Size) {
			time.Sleep(time.Second)
		} else if err != nil && i == int(cr.Spec.ProxySQL.Size) {
			return "", err
		} else {
			dbatabase = db
			break
		}
	}

	defer dbatabase.Close()

	return dbatabase.PrimaryHost()
}

func (r *ReconcilePerconaXtraDBCluster) waitPXCSynced(cr *api.PerconaXtraDBCluster, podIP string, waitLimit int) error {
	user := "root"

	dbatabase, err := queries.New(r.client, cr.Namespace, cr.Spec.SecretsName, user, podIP, 3306)
	if err != nil {
		return fmt.Errorf("failed to access PXC database: %v", err)
	}

	defer dbatabase.Close()

	for i := 0; i < waitLimit; i++ {
		state, err := dbatabase.WsrepLocalStateComment()
		if err != nil {
			return fmt.Errorf("failed to get wsrep local state: %v", err)
		}

		if state == "Synced" {
			return nil
		}

		time.Sleep(time.Second * 1)
	}

	return fmt.Errorf("reach pod wait limit")
}

func (r *ReconcilePerconaXtraDBCluster) waitPodRestart(updateRevision string, pod *corev1.Pod, waitLimit int) error {
	for i := 0; i < waitLimit; i++ {
		time.Sleep(time.Second * 1)

		err := r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}

		ready := false
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == "pxc" {
				ready = container.Ready
			}
		}

		if pod.Status.Phase == corev1.PodRunning && pod.ObjectMeta.Labels["controller-revision-hash"] == updateRevision && ready {
			log.Info(fmt.Sprintf("pod %s started", pod.Name))
			return nil
		}
	}

	return fmt.Errorf("reach pod wait limit")
}

func isPXC(sfs api.StatefulApp) bool {
	return sfs.Labels()["app.kubernetes.io/component"] == "pxc"
}

func (r *ReconcilePerconaXtraDBCluster) isBackupRunning(cr *api.PerconaXtraDBCluster) error {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := r.client.List(context.TODO(), &client.ListOptions{Namespace: cr.Namespace}, &bcpList); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get backup object: %v", err)
	}

	for _, bcp := range bcpList.Items {
		if bcp.Status.State == api.BackupRunning || bcp.Status.State == api.BackupStarting {
			return fmt.Errorf("backup %s is running", bcp.Name)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) getConfigHash(cr *api.PerconaXtraDBCluster) string {
	configString := cr.Spec.PXC.Configuration
	hash := fmt.Sprintf("%x", md5.Sum([]byte(configString)))

	return hash
}

func (r *ReconcilePerconaXtraDBCluster) getTLSHash(cr *api.PerconaXtraDBCluster, secretName string) (string, error) {
	if cr.Spec.AllowUnsafeConfig {
		return "", nil
	}

	secretObj := corev1.Secret{}
	if err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&secretObj,
	); err != nil {
		return "", err
	}

	secretString := fmt.Sprintln(secretObj.Data)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(secretString)))

	return hash, nil
}
