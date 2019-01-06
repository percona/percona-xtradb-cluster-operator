package backup

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

// NewPVC returns the list of PersistentVolumeClaims for the backups
func NewPVC(cr *api.PerconaXtraDBBackup) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      genName63(cr),
			Namespace: cr.Namespace,
		},
	}
}

// PVCSpec returns the pvc spec
func PVCSpec(spec api.PXCBackupSpec) (corev1.PersistentVolumeClaimSpec, error) {
	rvolStorage, err := resource.ParseQuantity(spec.Volume.Size)
	if err != nil {
		return corev1.PersistentVolumeClaimSpec{}, fmt.Errorf("wrong storage resources: %v", err)
	}

	return corev1.PersistentVolumeClaimSpec{
		StorageClassName: spec.Volume.StorageClass,
		AccessModes: []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: rvolStorage,
			},
		},
	}, nil
}
