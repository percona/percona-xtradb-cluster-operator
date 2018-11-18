package app

import (
	"fmt"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func PVCs(name string, vspec *api.PodVolumeSpec) ([]corev1.PersistentVolumeClaim, error) {
	rvolStorage, err := resource.ParseQuantity(vspec.Size)
	if err != nil {
		return nil, fmt.Errorf("wrong storage resources: %v", err)
	}

	return []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "datadir",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: vspec.StorageClass,
				AccessModes:      vspec.AccessModes,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: rvolStorage,
					},
				},
			},
		},
	}, nil
}
