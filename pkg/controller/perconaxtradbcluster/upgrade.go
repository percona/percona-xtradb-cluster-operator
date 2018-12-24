package perconaxtradbcluster

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app"
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) error {
	currentSet := sfs.StatefulSet()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: currentSet.Name, Namespace: currentSet.Namespace}, currentSet)

	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	newContainers := []corev1.Container{}
	var currentAppC, currentPMMC *corev1.Container

	for _, c := range currentSet.Spec.Template.Spec.Containers {
		if c.Name == "pmm-client" {
			newc := c
			currentPMMC = &newc
		} else {
			newc := c
			currentAppC = &newc
		}
	}

	// change the pod size
	size := podSpec.Size
	if *currentSet.Spec.Replicas != size {
		// logrus.Infof("Scaling containers from %d to %d", *currentSet.Spec.Replicas, size)
		currentSet.Spec.Replicas = &size
	}

	appC, err := updateApp(currentAppC, sfs, podSpec, cr)
	if err != nil {
		return fmt.Errorf("upgradePod/updateApp error: %v", err)
	}
	newContainers = append(newContainers, appC)

	if cr.Spec.PMM.Enabled {
		if currentPMMC == nil {
			pmmC := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName)
			newContainers = append(newContainers, pmmC)
		} else {
			pmmC := updatePMM(*currentPMMC, cr)
			newContainers = append(newContainers, pmmC)
		}
	}

	currentSet.Spec.Template.Spec.Containers = newContainers

	currentSet.Spec.Template.Spec.Affinity = pxc.PodAffinity(podSpec.Affinity)

	return r.client.Update(context.TODO(), currentSet)
}

// updatePMM updateds only allowed properties of the pmm-client container
func updatePMM(c corev1.Container, with *api.PerconaXtraDBCluster) corev1.Container {
	pmm := with.Spec.PMM

	c.Image = pmm.Image

	for k, v := range c.Env {
		switch v.Name {
		case "PMM_SERVER":
			c.Env[k].Value = pmm.ServerHost
		case "PMM_USER":
			c.Env[k].Value = pmm.ServerUser
		case "PMM_PASSWORD":
			c.Env[k].ValueFrom = &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(with.Spec.SecretsName, "pmmserver"),
			}
		}
	}
	return c
}

// updatePMM updateds only allowed properties of the app (node, proxy etc.) container
// it returns initial container on error
func updateApp(c *corev1.Container, sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) (corev1.Container, error) {
	res, err := sfs.Resources(podSpec.Resources)
	if err != nil {
		return *c, fmt.Errorf("create resources error: %v", err)
	}

	if c == nil {
		appC := sfs.AppContainer(podSpec, cr.Spec.SecretsName)
		appC.Resources = res
		return appC, nil
	}

	if !reflect.DeepEqual(c.Resources, res) {
		c.Resources = res
	}
	c.Image = podSpec.Image

	return *c, nil
}
