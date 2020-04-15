package pxc

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	appsv1 "k8s.io/api/apps/v1"
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) error {
	currentSet := sfs.StatefulSet()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)
	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	startGeneration := currentSet.Generation

	// change the pod size
	currentSet.Spec.Replicas = &podSpec.Size

	switch {
	case cr.Spec.UpdateStrategy == "OnDelete" && !cr.Spec.SmartUpdateEnabled():
		currentSet.Spec.UpdateStrategy.Type = appsv1.OnDeleteStatefulSetStrategyType
	case cr.Spec.UpdateStrategy == "OnDelete" && cr.Spec.SmartUpdateEnabled():
		currentSet.Spec.UpdateStrategy.Type = appsv1.OnDeleteStatefulSetStrategyType
		if !isPXC(sfs) {
			// Use 'RollingUpdate' type for non PXC nodes if 'SmartUpdate' is being used
			currentSet.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
		}
	default:
		currentSet.Spec.UpdateStrategy.Type = cr.Spec.UpdateStrategy
	}

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

	if podSpec.ForceUnsafeBootstrap {
		ic := appC.DeepCopy()
		ic.Name = ic.Name + "-init"
		ic.ReadinessProbe = nil
		ic.LivenessProbe = nil
		ic.Command = []string{"/unsafe-bootstrap.sh"}
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

	if !cr.Spec.SmartUpdateEnabled() {
		return nil
	}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)
	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	if currentSet.updatedReplicas != nil && currentSet.updatedReplicas >= currentSet.replicas {
		return nil
	}

	log.Info("statefullSet was changed, run smart update")

	return r.smartUpdate(sfs, cr)
}

func (r *ReconcilePerconaXtraDBCluster) smartUpdate(sfs api.StatefulApp, cr *api.PerconaXtraDBCluster) error {
	if !isPXC(sfs) {
		return nil
	}

	primary, err := r.getPrimaryPod(cr)
	if err != nil {
		return fmt.Errorf("get primary pod: %v", err)
	}

	log.Info(fmt.Sprintf("primary pod is %s", primary))

	list := corev1.PodList{}
	err = r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace:     sfs.StatefulSet().Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
		&list,
	)
	if err != nil {
		return fmt.Errorf("get pod list: %v", err)
	}

	var primaryPod *corev1.Pod = nil
	for _, pod := range list.Items {
		pod := pod
		if strings.HasPrefix(primary, pod.Name) {
			primaryPod = &pod
		} else {
			log.Info(fmt.Sprintf("delete secondary pod %s", pod.Name))
			err := r.client.Delete(context.TODO(), &pod)
			if err != nil {
				return fmt.Errorf("delete pod: %v", err)
			}

			err = r.waitPodRestart(&pod)
			if err != nil {
				return fmt.Errorf("wait pod: %v", err)
			}
		}
	}

	log.Info(fmt.Sprintf("delete primary pod %s", primaryPod.Name))
	err = r.client.Delete(context.TODO(), primaryPod)
	if err != nil {
		return fmt.Errorf("delete primary pod: %v", err)
	}

	err = r.waitPodRestart(primaryPod)
	if err != nil {
		return fmt.Errorf("wait pod: %v", err)
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) getPrimaryPod(cr *api.PerconaXtraDBCluster) (string, error) {
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&secretObj,
	)
	if err != nil {
		return "", err
	}

	user := "proxyadmin"
	pass := string(secretObj.Data[user])
	host := fmt.Sprintf("%s-proxysql-unready", cr.ObjectMeta.Name)

	db, err := queries.New(user, pass, host, 6032)
	if err != nil {
		return "", err
	}
	defer db.Close()

	primary, err := db.PrimaryHost()
	return primary, err
}

func (r *ReconcilePerconaXtraDBCluster) waitPodRestart(pod *corev1.Pod) error {
	log.Info(fmt.Sprintf("wait pod %s restart", pod.Name))
	err := r.waitPodPhase(pod, corev1.PodPending, 120)
	if err != nil {
		return err
	}

	err = r.waitPodPhase(pod, corev1.PodRunning, 120)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("pod %s restarted", pod.Name))

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) waitPodPhase(pod *corev1.Pod, phase corev1.PodPhase, triesLimit int) error {
	i := 0
	for {
		if i >= triesLimit {
			return fmt.Errorf("reach wait pod %s phase limit", phase)
		}

		time.Sleep(time.Second * 1)

		err := r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}

		if pod.Status.Phase == phase {
			break
		}

		i++
	}

	return nil
}

func isPXC(sfs api.StatefulApp) bool {
	return sfs.Labels()["app.kubernetes.io/component"] == "pxc"
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
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&secretObj,
	)
	if err != nil {
		return "", err
	}
	secretString := fmt.Sprintln(secretObj.Data)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(secretString)))

	return hash, nil
}
