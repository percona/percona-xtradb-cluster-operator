package backup

import (
	"fmt"
	"strings"

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

	base := naming.BackupJobName(cr.Name)

	ts := cr.CreationTimestamp.UTC().Format("20060102150405")

	uidSuffix := strings.ToLower(string(cr.UID))
	if len(uidSuffix) > 8 {
		uidSuffix = uidSuffix[:8]
	}

	// Final name: xb-<name>-<YYYYMMDDhhmmss>-<uidsfx>
	name := fmt.Sprintf("%s-%s-%s", base, ts, uidSuffix)

	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
			Labels:    ls,
		},
	}
}
