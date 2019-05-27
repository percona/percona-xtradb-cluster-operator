package pxc

import (
	"context"
	"crypto/md5"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
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
	err = r.handleSpecConfig(cr, currentSet)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update db config error: %v", err)
	}

	// change TLS secret configuration
	err = r.handleTLSSecret(cr, currentSet)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: update secret error: %v", err)
	}

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

func (r *ReconcilePerconaXtraDBCluster) handleSpecConfig(cr *api.PerconaXtraDBCluster, nodeSet *appsv1.StatefulSet) error {
	stsApp := statefulset.NewNode(cr)
	configMap := &corev1.ConfigMap{}
	if cr.Spec.PXC.Configuration != "" {
		ls := stsApp.Labels()
		configMap = configmap.NewConfigMap(cr, ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"])
	}
	configString := cr.Spec.PXC.Configuration
	hash := fmt.Sprintf("%x", md5.Sum([]byte(configString)))
	if nodeSet.Spec.Template.Annotations == nil {
		nodeSet.Spec.Template.Annotations = make(map[string]string)
	}

	if len(nodeSet.Spec.Template.Annotations["cfg_hash"]) > 0 && nodeSet.Spec.Template.Annotations["cfg_hash"] != hash {
		log.Info("new DB configuration")
		err := r.client.Update(context.TODO(), configMap)
		if err != nil {
			return fmt.Errorf("update ConfigMap: %v", err)
		}
	}
	nodeSet.Spec.Template.Annotations["cfg_hash"] = hash

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handleTLSSecret(cr *api.PerconaXtraDBCluster, nodeSet *appsv1.StatefulSet) error {
	if cr.Spec.PXC.AllowUnsafeConfig {
		return nil
	}
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: nodeSet.Namespace,
			Name:      cr.Spec.PXC.SSLSecretName,
		},
		&secretObj,
	)
	if err != nil {
		return fmt.Errorf("get Secret object: %v", err)
	}
	secretString := fmt.Sprintln(secretObj)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(secretString)))
	if nodeSet.Spec.Template.Annotations == nil {
		nodeSet.Spec.Template.Annotations = make(map[string]string)
	}
	nodeSet.Spec.Template.Annotations["ssl_hash"] = hash

	return nil
}
