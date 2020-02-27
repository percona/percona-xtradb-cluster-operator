package pxc

import (
	"context"
	"crypto/md5"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	appsv1 "k8s.io/api/apps/v1"
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) error {
	currentSet := sfs.StatefulSet()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)

	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	// change the pod size
	currentSet.Spec.Replicas = &podSpec.Size

	switch cr.Spec.UpdateStrategy {
	case "OnDelete":
		currentSet.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{}
		currentSet.Spec.UpdateStrategy.Type = cr.Spec.UpdateStrategy
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

	return r.client.Update(context.TODO(), currentSet)
}

func (r *ReconcilePerconaXtraDBCluster) updateService(svc *corev1.Service, podSpec *api.PodSpec) error {
	currentService := svc
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentService.Name, Namespace: currentService.Namespace}, currentService)

	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	if podSpec.ServiceType != nil {
		switch *podSpec.ServiceType {
		case corev1.ServiceTypeClusterIP:
			currentService.Spec.Ports = []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql",
				},
			}
			currentService.Spec.Type = *podSpec.ServiceType
		default:
			currentService.Spec.Type = *podSpec.ServiceType
		}
	}

	return r.client.Update(context.TODO(), currentService)
}

func (r *ReconcilePerconaXtraDBCluster) getConfigHash(cr *api.PerconaXtraDBCluster) string {
	configString := cr.Spec.PXC.Configuration
	hash := fmt.Sprintf("%x", md5.Sum([]byte(configString)))

	return hash
}

func (r *ReconcilePerconaXtraDBCluster) getTLSHash(cr *api.PerconaXtraDBCluster, secretName string) (string, error) {
	if cr.Spec.PXC.AllowUnsafeConfig {
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
