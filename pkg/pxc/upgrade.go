package pxc

import (
	"fmt"
	"reflect"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func (h *PXC) upgradePods(pod *api.PodSpec, sset *appsv1.StatefulSet) error {
	err := sdk.Get(sset)
	if err != nil {
		return fmt.Errorf("failed to get sate: %v", err)
	}

	changes := false

	resources, err := createResources(pod.Resources)
	if err != nil {
		return fmt.Errorf("createResources error: %v", err)
	}
	for i, c := range sset.Spec.Template.Spec.Containers {
		if !reflect.DeepEqual(c.Resources, resources) {
			changes = true
			sset.Spec.Template.Spec.Containers[i].Resources = resources
		}
	}
	if changes {
		logrus.Info("Update containers resources")
	}

	size := pod.Size
	if *sset.Spec.Replicas != size {
		changes = true
		logrus.Infof("Scaling containers from %d to %d", *sset.Spec.Replicas, size)
		sset.Spec.Replicas = &size
	}

	// !!! Forbidden: updates to statefulset spec for fields other than 'replicas', 'template', and 'updateStrategy' are forbidden.
	// if len(sset.Spec.VolumeClaimTemplates) > 0 {
	// 	pvc := sset.Spec.VolumeClaimTemplates[0]

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
		err = sdk.Update(sset)
		if err != nil {
			return fmt.Errorf("failed to update deployment: %v", err)
		}
	}

	return nil
}
