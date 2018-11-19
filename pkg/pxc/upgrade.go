package pxc

import (
	"fmt"
	"reflect"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app"
)

func (h *PXC) updatePod(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) error {
	currentSet := sfs.StatefulSet()
	err := sdk.Get(currentSet)
	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	newContainers := []corev1.Container{}
	var currentAppC, currentPMMC *corev1.Container

	for _, c := range currentSet.Spec.Template.Spec.Containers {
		if c.Name == "pmm-client" {
			currentPMMC = &c
		} else {
			currentAppC = &c
		}
	}

	// app container not deployed yet, so no reason to continue
	if currentAppC == nil {
		return nil
	}

	changed := false

	// change the pod size
	size := podSpec.Size
	if *currentSet.Spec.Replicas != size {
		logrus.Infof("Scaling containers from %d to %d", *currentSet.Spec.Replicas, size)
		currentSet.Spec.Replicas = &size
		changed = true
	}

	if cr.Spec.PMM.Enabled {
		if currentPMMC == nil {
			pmmC := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName)
			changed = true
			newContainers = append(newContainers, pmmC)
		} else {
			pmmC, updated := updatePMM(*currentPMMC, cr)
			if updated {
				changed = true
				newContainers = append(newContainers, pmmC)
			}
		}
	}

	appC, updated, err := updateApp(*currentAppC, sfs, podSpec, cr)
	if err != nil {
		logrus.Errorln("upgradePod/updateApp error:", err)
	}
	newContainers = append(newContainers, appC)

	if updated {
		changed = true
	}

	if len(currentSet.Spec.Template.Spec.Containers) != len(newContainers) {
		changed = true
	}

	if changed == true && len(newContainers) > 0 {
		currentSet.Spec.Template.Spec.Containers = newContainers
	}

	if changed {
		logrus.Infof("update statefulset")
		return sdk.Update(currentSet)
	}

	return nil
}

// updatePMM updateds only allowed properties of the pmm-client container
func updatePMM(c corev1.Container, with *api.PerconaXtraDBCluster) (corev1.Container, bool) {
	changed := false
	pmm := with.Spec.PMM
	c.Image = pmm.Image
	for k, v := range c.Env {
		switch v.Name {
		case "PMM_SERVER":
			if c.Env[k].Value != pmm.ServerHost {
				c.Env[k].Value = pmm.ServerHost
				changed = true
			}
		case "PMM_USER":
			if c.Env[k].Value != pmm.ServerUser {
				c.Env[k].Value = pmm.ServerUser
				changed = true
			}
		case "PMM_PASSWORD":
			c.Env[k].ValueFrom = &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(with.Spec.SecretsName, "pmmserver"),
			}
		}
	}
	return c, changed
}

// updatePMM updateds only allowed properties of the app (node, proxy etc.) container
// it returns initial container on error
func updateApp(c corev1.Container, sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) (corev1.Container, bool, error) {
	changed := false

	res, err := sfs.Resources(podSpec.Resources)
	if err != nil {
		return c, changed, fmt.Errorf("create resources error: %v", err)
	}

	if !reflect.DeepEqual(c.Resources, res) {
		c.Resources = res
		changed = true
	}
	if c.Image != podSpec.Image {
		c.Image = podSpec.Image
		changed = true
	}

	return c, changed, nil
}
