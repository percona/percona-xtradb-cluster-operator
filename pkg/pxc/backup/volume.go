package backup

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

// NewPVC returns the list of PersistentVolumeClaims for the backups
func NewPVC(cr *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) *corev1.PersistentVolumeClaim {
	var ls map[string]string
	if cluster.CompareVersionWith("1.16.0") >= 0 {
		ls = naming.LabelsCluster(cluster)
	}
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.BackupJobName(cr.Name),
			Namespace: cr.Namespace,
			Labels:    ls,
		},
	}
}
