package pxc

import (
	"context"
	"crypto/md5"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/configmap"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) error {
	currentSet := sfs.StatefulSet()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)

	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	// change the pod size
	currentSet.Spec.Replicas = &podSpec.Size

	res, err := sfs.Resources(podSpec.Resources)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: create resources error: %v", err)
	}

	// change DB configuration
	configHash := r.getConfigHash(cr)
	if currentSet.Spec.Template.Annotations == nil {
		currentSet.Spec.Template.Annotations = make(map[string]string)
	}
	currentSet.Spec.Template.Annotations["cfg_hash"] = configHash

	err = r.updateConfigMap(cr)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update db config error: %v", err)
	}

	// change TLS secret configuration
	tlsHash, err := r.getTLSHash(cr)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}
	currentSet.Spec.Template.Annotations["ssl_hash"] = tlsHash

	var newContainers []corev1.Container
	var newInitContainers []corev1.Container

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

	// pmm container
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		pmmC := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName)
		newContainers = append(newContainers, pmmC)
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

func (r *ReconcilePerconaXtraDBCluster) updateConfigMap(cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.PXC.Configuration != "" {
		stsApp := statefulset.NewNode(cr)
		ls := stsApp.Labels()
		configMap := configmap.NewConfigMap(cr, ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"])
		err := setControllerReference(cr, configMap, r.scheme)
		if err != nil {
			return err
		}
		err = r.client.Update(context.TODO(), configMap)
		if err != nil {
			return fmt.Errorf("update ConfigMap: %v", err)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) getTLSHash(cr *api.PerconaXtraDBCluster) (string, error) {
	if cr.Spec.PXC.AllowUnsafeConfig {
		return "", nil
	}
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.PXC.SSLSecretName,
		},
		&secretObj,
	)
	if err != nil {
		return "", err
	}
	secretString := fmt.Sprintln(secretObj)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(secretString)))

	return hash, nil
}
