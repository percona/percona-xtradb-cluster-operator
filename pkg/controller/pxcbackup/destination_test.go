package pxcbackup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestBackupDestinationName(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2026, time.November, 10, 10, 0, 0, 0, time.UTC))

	tests := map[string]struct {
		backupType pxcv1.PXCBackupType
		expected   string
	}{
		"empty type is a full backup": {
			backupType: "",
			expected:   "cluster1-2026-11-10-10:00:00-full",
		},
		"full": {
			backupType: pxcv1.BackupTypeFull,
			expected:   "cluster1-2026-11-10-10:00:00-full",
		},
		"logical": {
			backupType: pxcv1.BackupTypeLogical,
			expected:   "cluster1-2026-11-10-10:00:00-logical",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &pxcv1.PerconaXtraDBClusterBackup{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: creationTime},
				Spec: pxcv1.PXCBackupSpec{
					PXCCluster: "cluster1",
					Type:       tt.backupType,
				},
			}

			assert.Equal(t, tt.expected, backupDestinationName(cr))
		})
	}
}
