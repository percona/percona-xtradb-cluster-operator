package perconaxtradbcluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
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

	res, err := sfs.Resources(podSpec.Resources)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: create resources error: %v", err)
	}

	var newContainers []corev1.Container

	// application container
	appC := sfs.AppContainer(podSpec, cr.Spec.SecretsName)
	appC.Resources = res
	newContainers = append(newContainers, appC)

	// pmm container
	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		pmmC := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName)
		newContainers = append(newContainers, pmmC)
	}

	// sidecars
	newContainers = append(newContainers, sfs.SidecarContainers(podSpec, cr.Spec.SecretsName)...)

	currentSet.Spec.Template.Spec.Containers = newContainers
	currentSet.Spec.Template.Spec.Affinity = pxc.PodAffinity(podSpec.Affinity, sfs)

	return r.client.Update(context.TODO(), currentSet)
}
