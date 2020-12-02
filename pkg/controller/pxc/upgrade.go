package pxc

import (
	"context"
	"crypto/md5"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"

	"github.com/pkg/errors"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, initContainers []corev1.Container) error {
	currentSet := sfs.StatefulSet()
	newAnnotations := currentSet.Spec.Template.Annotations // need this step to save all new annotations that was set to currentSet in this reconcile loop
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)
	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	currentSet.Spec.UpdateStrategy = sfs.UpdateStrategy(cr)

	// support annotation adjustements
	pxc.MergeTemplateAnnotations(currentSet, podSpec.Annotations)

	// change the pod size
	currentSet.Spec.Replicas = &podSpec.Size
	currentSet.Spec.Template.Spec.SecurityContext = podSpec.PodSecurityContext

	// embed DB configuration hash
	// TODO: code duplication with deploy function
	configHash := r.getConfigHash(cr, sfs)

	if currentSet.Spec.Template.Annotations == nil {
		currentSet.Spec.Template.Annotations = make(map[string]string)
	}

	pxc.MergeTemplateAnnotations(currentSet, newAnnotations)

	if cr.CompareVersionWith("1.1.0") >= 0 {
		currentSet.Spec.Template.Annotations["percona.com/configuration-hash"] = configHash
	}
	if cr.CompareVersionWith("1.5.0") >= 0 {
		currentSet.Spec.Template.Spec.ServiceAccountName = podSpec.ServiceAccountName
	}

	err = r.reconcileConfigMap(cr)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update db config error: %v", err)
	}

	// change TLS secret configuration
	sslHash, err := r.getSecretHash(cr, cr.Spec.PXC.SSLSecretName, cr.Spec.AllowUnsafeConfig)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	if sslHash != "" && cr.CompareVersionWith("1.1.0") >= 0 {
		currentSet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash
	}

	sslInternalHash, err := r.getSecretHash(cr, cr.Spec.PXC.SSLInternalSecretName, cr.Spec.AllowUnsafeConfig)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	if sslInternalHash != "" && cr.CompareVersionWith("1.1.0") >= 0 {
		currentSet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
	}

	vaultConfigHash, err := r.getSecretHash(cr, cr.Spec.VaultSecretName, true)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	if vaultConfigHash != "" && cr.CompareVersionWith("1.6.0") >= 0 && !isHAproxy(sfs) {
		currentSet.Spec.Template.Annotations["percona.com/vault-config-hash"] = vaultConfigHash
	}

	if isHAproxy(sfs) && cr.CompareVersionWith("1.6.0") >= 0 {
		delete(currentSet.Spec.Template.Annotations, "percona.com/ssl-internal-hash")
		delete(currentSet.Spec.Template.Annotations, "percona.com/ssl-hash")
	}

	var newContainers []corev1.Container
	var newInitContainers []corev1.Container

	secrets := cr.Spec.SecretsName
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secrets = "internal-" + cr.Name
	}

	// pmm container
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		pmmC, err := sfs.PMMContainer(cr.Spec.PMM, secrets, cr)
		if err != nil {
			return fmt.Errorf("pmm container error: %v", err)
		}
		if pmmC != nil {
			newContainers = append(newContainers, *pmmC)
		}
	}

	// log-collector container
	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Enabled && cr.CompareVersionWith("1.7.0") >= 0 {
		logCollectorC, err := sfs.LogCollectorContainer(cr.Spec.LogCollector, cr.Spec.LogCollectorSecretName, secrets, cr)
		if err != nil {
			return fmt.Errorf("logcollector container error: %v", err)
		}
		if logCollectorC != nil {
			newContainers = append(newContainers, logCollectorC...)
		}
	}

	// application container
	appC, err := sfs.AppContainer(podSpec, secrets, cr)
	if err != nil {
		return fmt.Errorf("app container error: %v", err)
	}

	newContainers = append(newContainers, appC)

	if len(initContainers) > 0 {
		newInitContainers = append(newInitContainers, initContainers...)
	}

	if podSpec.ForceUnsafeBootstrap {
		ic := appC.DeepCopy()
		res, err := app.CreateResources(podSpec.Resources)
		if err != nil {
			return errors.Wrap(err, "create resources")
		}
		ic.Resources = res
		ic.Name = ic.Name + "-init-unsafe"
		ic.ReadinessProbe = nil
		ic.LivenessProbe = nil
		ic.Command = []string{"/var/lib/mysql/unsafe-bootstrap.sh"}
		newInitContainers = append(newInitContainers, *ic)
	}

	// sidecars
	sideC, err := sfs.SidecarContainers(podSpec, secrets, cr)
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
	if sfsVolume != nil && sfsVolume.Volumes != nil {
		currentSet.Spec.Template.Spec.Volumes = sfsVolume.Volumes
	}

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

	log.Info("statefulSet was changed, run smart update")

	running, err := r.isBackupRunning(cr)
	if err != nil {
		log.Error(err, "can't start 'SmartUpdate'")
		return nil
	}
	if running {
		log.Info("can't start/continue 'SmartUpdate': backup is running")
		return nil
	}

	if sfs.StatefulSet().Status.ReadyReplicas < sfs.StatefulSet().Status.Replicas {
		log.Info("can't start/continue 'SmartUpdate': waiting for all replicas are ready")
		return nil
	}

	list := corev1.PodList{}
	if err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     sfs.StatefulSet().Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
	); err != nil {
		return fmt.Errorf("get pod list: %v", err)
	}

	primary, err := r.getPrimaryPod(cr)
	if err != nil {
		return fmt.Errorf("get primary pod: %v", err)
	}
	for _, pod := range list.Items {
		if pod.Status.PodIP == primary || pod.Name == primary {
			primary = fmt.Sprintf("%s.%s.%s", pod.Name, sfs.StatefulSet().Name, sfs.StatefulSet().Namespace)
			break
		}
	}

	log.Info(fmt.Sprintf("primary pod is %s", primary))

	waitLimit := 2 * 60 * 60 // 2 hours
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
			if err := r.applyNWait(cr, sfs.StatefulSet(), &pod, waitLimit); err != nil {
				return fmt.Errorf("failed to apply changes: %v", err)
			}
		}
	}

	log.Info(fmt.Sprintf("apply changes to primary pod %s", primaryPod.Name))
	if err := r.applyNWait(cr, sfs.StatefulSet(), &primaryPod, waitLimit); err != nil {
		return fmt.Errorf("failed to apply changes: %v", err)
	}

	log.Info("smart update finished")

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) applyNWait(cr *api.PerconaXtraDBCluster, sfs *appsv1.StatefulSet, pod *corev1.Pod, waitLimit int) error {
	if pod.ObjectMeta.Labels["controller-revision-hash"] == sfs.Status.UpdateRevision {
		log.Info(fmt.Sprintf("pod %s is already updated", pod.Name))
	} else {
		if err := r.client.Delete(context.TODO(), pod); err != nil {
			return fmt.Errorf("failed to delete pod: %v", err)
		}
	}

	if err := r.waitPodRestart(sfs.Status.UpdateRevision, pod, waitLimit); err != nil {
		return fmt.Errorf("failed to wait pod: %v", err)
	}

	if err := r.waitPXCSynced(cr, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, waitLimit); err != nil {
		return fmt.Errorf("failed to wait pxc sync: %v", err)
	}

	if err := r.waitUntilOnline(cr, sfs.Name, pod, waitLimit); err != nil {
		return fmt.Errorf("failed to wait pxc status: %v", err)
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) waitUntilOnline(cr *api.PerconaXtraDBCluster, sfsName string, pod *corev1.Pod, waitLimit int) error {
	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		time.Sleep(5 * time.Second)
		return nil
	}

	database, err := r.proxyDB(cr)
	if err != nil {
		return fmt.Errorf("failed to get proxySQL db: %v", err)
	}

	defer database.Close()

	podNamePrefix := fmt.Sprintf("%s.%s.%s", pod.Name, sfsName, cr.Namespace)

	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			statuses, err := database.Status(podNamePrefix, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace)
			if err != nil && err != queries.ErrNotFound {
				return false, fmt.Errorf("failed to get status: %v", err)
			}

			for _, status := range statuses {
				if status != "ONLINE" {
					return false, nil
				}
			}

			log.Info(fmt.Sprintf("pod %s is online", pod.Name))
			return true, nil
		})
}

// retry runs func "f" every "in" time until "limit" is reached
// it also doesn't have an extra tail wait after the limit is reached
// and f func runs first time instantly
func retry(in, limit time.Duration, f func() (bool, error)) error {
	fdone, err := f()
	if err != nil {
		return err
	}
	if fdone {
		return nil
	}

	done := time.NewTimer(limit)
	defer done.Stop()
	tk := time.NewTicker(in)
	defer tk.Stop()

	for {
		select {
		case <-done.C:
			return fmt.Errorf("reach pod wait limit")
		case <-tk.C:
			fdone, err := f()
			if err != nil {
				return err
			}
			if fdone {
				return nil
			}
		}
	}
}

func (r *ReconcilePerconaXtraDBCluster) proxyDB(cr *api.PerconaXtraDBCluster) (queries.Database, error) {
	var database queries.Database
	var user, host string
	var port, proxySize int32

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		user = "proxyadmin"
		host = fmt.Sprintf("%s-proxysql-unready.%s", cr.ObjectMeta.Name, cr.Namespace)
		proxySize = cr.Spec.ProxySQL.Size
		port = 6032
	} else if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		user = "monitor"
		host = fmt.Sprintf("%s-haproxy.%s", cr.Name, cr.Namespace)
		proxySize = cr.Spec.HAProxy.Size

		hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
		if err != nil {
			return database, errors.Wrap(err, "check if config has proxy_protocol_networks key")
		}

		if hasKey && cr.CompareVersionWith("1.6.0") >= 0 {
			port = 33062
		} else {
			port = 3306
		}
	} else {
		return database, errors.New("can't detect enabled proxy, please enable HAProxy or ProxySQL")
	}
	secrets := cr.Spec.SecretsName
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secrets = "internal-" + cr.Name
	}
	for i := 0; ; i++ {
		db, err := queries.New(r.client, cr.Namespace, secrets, user, host, port)
		if err != nil && i < int(proxySize) {
			time.Sleep(time.Second)
		} else if err != nil && i == int(proxySize) {
			return database, err
		} else {
			database = db
			break
		}
	}

	return database, nil
}

func (r *ReconcilePerconaXtraDBCluster) getPrimaryPod(cr *api.PerconaXtraDBCluster) (string, error) {
	database, err := r.proxyDB(cr)
	if err != nil {
		return "", fmt.Errorf("failed to get proxySQL db: %v", err)
	}

	defer database.Close()

	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		host, err := database.Hostname()
		if err != nil {
			return "", err
		}

		return host, nil
	}

	return database.PrimaryHost()
}

func (r *ReconcilePerconaXtraDBCluster) waitPXCSynced(cr *api.PerconaXtraDBCluster, host string, waitLimit int) error {
	user := "root"
	secrets := cr.Spec.SecretsName
	port := int32(3306)
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secrets = "internal-" + cr.Name
		port = int32(33062)
	}

	database, err := queries.New(r.client, cr.Namespace, secrets, user, host, port)
	if err != nil {
		return fmt.Errorf("failed to access PXC database: %v", err)
	}

	defer database.Close()

	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			state, err := database.WsrepLocalStateComment()
			if err != nil {
				return false, fmt.Errorf("failed to get wsrep local state: %v", err)
			}

			if state == "Synced" {
				return true, nil
			}

			return false, nil
		})
}

func (r *ReconcilePerconaXtraDBCluster) waitPodRestart(updateRevision string, pod *corev1.Pod, waitLimit int) error {
	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			err := r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
			if err != nil && !k8serrors.IsNotFound(err) {
				return false, err
			}

			ready := false
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == "pxc" {
					ready = container.Ready
				}
			}

			if pod.Status.Phase == corev1.PodFailed {
				return false, errors.Errorf("pod %s is in failed phase", pod.Name)
			}

			if pod.Status.Phase == corev1.PodRunning && pod.ObjectMeta.Labels["controller-revision-hash"] == updateRevision && ready {
				log.Info(fmt.Sprintf("pod %s is running", pod.Name))
				return true, nil
			}

			return false, nil
		})
}

func isPXC(sfs api.StatefulApp) bool {
	return sfs.Labels()["app.kubernetes.io/component"] == "pxc"
}

func isHAproxy(sfs api.StatefulApp) bool {
	return sfs.Labels()["app.kubernetes.io/component"] == "haproxy"
}

func (r *ReconcilePerconaXtraDBCluster) isBackupRunning(cr *api.PerconaXtraDBCluster) (bool, error) {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := r.client.List(context.TODO(), &bcpList, &client.ListOptions{Namespace: cr.Namespace}); err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get backup object: %v", err)
	}

	for _, bcp := range bcpList.Items {
		if bcp.Status.State == api.BackupRunning || bcp.Status.State == api.BackupStarting {
			return true, nil
		}
	}

	return false, nil
}

func (r *ReconcilePerconaXtraDBCluster) getConfigHash(cr *api.PerconaXtraDBCluster, sfs api.StatefulApp) string {
	configString := cr.Spec.PXC.Configuration
	if sfs.Labels()["app.kubernetes.io/component"] == "haproxy" {
		configString = cr.Spec.HAProxy.Configuration
	} else if sfs.Labels()["app.kubernetes.io/component"] == "proxysql" {
		configString = cr.Spec.ProxySQL.Configuration
	}
	hash := fmt.Sprintf("%x", md5.Sum([]byte(configString)))

	return hash
}

func (r *ReconcilePerconaXtraDBCluster) getSecretHash(cr *api.PerconaXtraDBCluster, secretName string, allowNonExistingSecret bool) (string, error) {
	secretObj := corev1.Secret{}
	if err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&secretObj,
	); err != nil && k8serrors.IsNotFound(err) && allowNonExistingSecret {
		return "", nil
	} else if err != nil {
		return "", err
	}

	secretString := fmt.Sprintln(secretObj.Data)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(secretString)))

	return hash, nil
}
