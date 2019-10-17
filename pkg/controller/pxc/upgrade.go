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
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) error {
	currentSet := sfs.StatefulSet()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)

	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	// change the pod size
	currentSet.Spec.Replicas = &podSpec.Size

	currentSet.Spec.UpdateStrategy.Type = cr.Spec.UpdateStrategy

	res, err := sfs.Resources(podSpec.Resources)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: create resources error: %v", err)
	}

	// embed DB configuration hash
	// TODO: code duplication with deploy function
	configHash := r.getConfigHash(cr)
	if currentSet.Spec.Template.Annotations == nil {
		currentSet.Spec.Template.Annotations = make(map[string]string)
	}
	currentSet.Spec.Template.Annotations["percona.com/configuration-hash"] = configHash

	err = r.reconcileConfigMap(cr)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update db config error: %v", err)
	}

	// change TLS secret configuration
	sslHash, err := r.getTLSHash(cr, cr.Spec.PXC.SSLSecretName)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	currentSet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash

	sslInternalHash, err := r.getTLSHash(cr, cr.Spec.PXC.SSLInternalSecretName)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	if !errors.IsNotFound(err) {
		currentSet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
	}

	var newContainers []corev1.Container
	var newInitContainers []corev1.Container

	// pmm container
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		pmmC := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName, !cr.VersionLessThan120())
		if !cr.VersionLessThan120() {
			res, err := sfs.Resources(cr.Spec.PMM.Resources)
			if err != nil {
				return fmt.Errorf("pmm container error: create resources error: %v", err)
			}
			pmmC.Resources = res
		}
		newContainers = append(newContainers, pmmC)
	}

	// application container
	appC := sfs.AppContainer(podSpec, cr.Spec.SecretsName)
	appC.Resources = res
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
	newContainers = append(newContainers, sfs.SidecarContainers(podSpec, cr.Spec.SecretsName)...)

	currentSet.Spec.Template.Spec.Containers = newContainers
	currentSet.Spec.Template.Spec.InitContainers = newInitContainers
	currentSet.Spec.Template.Spec.Affinity = pxc.PodAffinity(podSpec.Affinity, sfs)

	return r.client.Update(context.TODO(), currentSet)
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
