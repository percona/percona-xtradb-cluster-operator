package pxc

import (
	"fmt"
	"reflect"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func (h *PXC) upgradePods(podCR *api.PodSpec, pmmCR *api.PMMSpec, currentSet *appsv1.StatefulSet) error {
	err := sdk.Get(currentSet)
	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	changes := false

	resources, err := createResources(podCR.Resources)
	if err != nil {
		return fmt.Errorf("createResources error: %v", err)
	}
	for i, c := range currentSet.Spec.Template.Spec.Containers {
		if !reflect.DeepEqual(c.Resources, resources) {
			changes = true
			currentSet.Spec.Template.Spec.Containers[i].Resources = resources
		}
	}
	if changes {
		logrus.Info("Update containers resources")
	}

	size := podCR.Size
	if *currentSet.Spec.Replicas != size {
		changes = true
		logrus.Infof("Scaling containers from %d to %d", *currentSet.Spec.Replicas, size)
		currentSet.Spec.Replicas = &size
	}

	newContainers := []corev1.Container{}
	foundPMM := false
	for _, c := range currentSet.Spec.Template.Spec.Containers {
		if c.Name == "pmm-client" {
			foundPMM = true
			if !pmmCR.Enabled {
				continue
			}
			if c.Image != pmmCR.Image {
				c.Image = pmmCR.Image
				changes = true
			}
		} else {
			if c.Image != podCR.Image {
				c.Image = podCR.Image
				changes = true
			}
		}
		newContainers = append(newContainers, c)
	}

	if pmmCR.Enabled != !foundPMM {

	}

	if len(currentSet.Spec.Template.Spec.Containers) != len(newContainers) {
		changes = true
	}

	if changes == true && len(newContainers) > 0 {
		currentSet.Spec.Template.Spec.Containers = newContainers
	}

	// !!! Forbidden: updates to statefulset spec for fields other than 'replicas', 'template', and 'updateStrategy' are forbidden.
	// if len(currentSet.Spec.VolumeClaimTemplates) > 0 {
	// 	pvc := currentSet.Spec.VolumeClaimTemplates[0]

	// 	if pvcRes, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
	// 		rvolStorage, err := resource.ParseQuantity(pod.VolumeSpec.Size)
	// 		if err != nil {
	// 			return fmt.Errorf("wrong storage resources: %v", err)
	// 		}
	// 		// the deployed size is less than in spec
	// 		if pvcRes.Cmp(rvolStorage) == -1 {
	// 			changes = true
	// 			logrus.Infof("Upsizing volume from %v to %v", pvcRes, rvolStorage)
	// 			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = rvolStorage
	// 		}
	// 	}
	// }

	if changes {
		err = sdk.Update(currentSet)
		if err != nil {
			return fmt.Errorf("failed to update deployment: %v", err)
		}
	}

	return nil
}
