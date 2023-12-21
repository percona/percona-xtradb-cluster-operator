package backup

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

// NewPVC returns the list of PersistentVolumeClaims for the backups
func NewPVC(cr *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) *corev1.PersistentVolumeClaim {
    // Copy from the original labels to the backup labels
    labels := make(map[string]string)
    for key, value := range cluster.Spec.Backup.Storages[cr.Spec.StorageName].Labels {
        labels[key] = value
    }

    labels["type"] = "xtrabackup"
    labels["cluster"] = cr.Spec.PXCCluster
    labels["backup-name"] = cr.Name

	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenName63(cr),
			Namespace: cr.Namespace,
			Labels: labels,
		},
	}
}
