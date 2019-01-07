package app

import (
	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCs returns the list of PersistentVolumeClaims for the pod
func PVCs(name string, vspec *api.VolumeSpec) []corev1.PersistentVolumeClaim {
	return []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: VolumeSpec(vspec),
		},
	}
}

// VolumeSpec returns volume claim based on the given spec
func VolumeSpec(vspec *api.VolumeSpec) corev1.PersistentVolumeClaimSpec {
	return corev1.PersistentVolumeClaimSpec{
		StorageClassName: vspec.StorageClass,
		AccessModes:      vspec.AccessModes,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: vspec.SizeParsed,
			},
		},
	}
}
