package backup

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

// PVC returns the list of PersistentVolumeClaims for the backups
func PVC(cr *api.PerconaXtraDBBackup) (corev1.PersistentVolumeClaim, error) {
	rvolStorage, err := resource.ParseQuantity(cr.Spec.Storage)
	if err != nil {
		return corev1.PersistentVolumeClaim{}, fmt.Errorf("wrong storage resources: %v", err)
	}

	return corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-volume",
			Namespace: cr.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: rvolStorage,
				},
			},
		},
	}, nil
}
