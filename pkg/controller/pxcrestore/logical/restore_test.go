package logical

import (
	"testing"

	"github.com/stretchr/testify/assert"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

const testDestination = api.PXCBackupDestination("s3://some-bucket/test-cluster-2026-11-10-10:00:00-logical")

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		cr          func(cr *api.PerconaXtraDBClusterRestore)
		cluster     func(cluster *api.PerconaXtraDBCluster)
		bcp         func(bcp *api.PerconaXtraDBClusterBackup)
		expectedErr string
	}{
		"valid restore": {},
		"point-in-time recovery is requested": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.PITR = &api.PITR{Type: "latest"}
			},
			expectedErr: "point-in-time recovery is not supported for logical backups",
		},
		"crVersion is too old": {
			cluster: func(cluster *api.PerconaXtraDBCluster) {
				cluster.Spec.CRVersion = "1.20.0"
			},
			expectedErr: "logical restores require crVersion 1.21.0 or newer",
		},
		"backup is on azure": {
			bcp: func(bcp *api.PerconaXtraDBClusterBackup) {
				bcp.Status.Destination = "azure://some-container/test-cluster-2026-11-10-10:00:00-logical"
			},
			expectedErr: "logical backups are supported only on s3 storage",
		},
		"backup is on a pvc": {
			bcp: func(bcp *api.PerconaXtraDBClusterBackup) {
				bcp.Status.Destination = "pvc/xb-test-backup"
			},
			expectedErr: "logical backups are supported only on s3 storage",
		},
		"cluster is initializing": {
			cluster: func(cluster *api.PerconaXtraDBCluster) {
				cluster.Status.Status = api.AppStateInit
			},
			expectedErr: "cluster must be ready for a logical restore, current status: initializing",
		},
		"cluster is paused": {
			cluster: func(cluster *api.PerconaXtraDBCluster) {
				cluster.Status.Status = api.AppStatePaused
			},
			expectedErr: "cluster must be ready for a logical restore, current status: paused",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBClusterRestore{}
			cluster := &api.PerconaXtraDBCluster{
				Spec:   api.PerconaXtraDBClusterSpec{CRVersion: "1.21.0"},
				Status: api.PerconaXtraDBClusterStatus{Status: api.AppStateReady},
			}
			bcp := &api.PerconaXtraDBClusterBackup{
				Status: api.PXCBackupStatus{
					Type:        api.BackupTypeLogical,
					Destination: testDestination,
				},
			}
			if tt.cr != nil {
				tt.cr(cr)
			}
			if tt.cluster != nil {
				tt.cluster(cluster)
			}
			if tt.bcp != nil {
				tt.bcp(bcp)
			}

			err := validate(cr, cluster, bcp)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestHasMetadata(t *testing.T) {
	const backupPath = "test-cluster-2026-11-10-10:00:00-logical/"

	tests := map[string]struct {
		objects  []storage.ObjectInfo
		expected bool
	}{
		"complete logical backup": {
			objects: []storage.ObjectInfo{
				{Name: backupPath + "metadata"},
				{Name: backupPath + "shop.orders.00000.sql"},
			},
			expected: true,
		},
		"logical backup that was interrupted": {
			objects: []storage.ObjectInfo{
				{Name: backupPath + "metadata.partial"},
				{Name: backupPath + "shop.orders.00000.sql"},
			},
			expected: false,
		},
		"physical backup": {
			objects: []storage.ObjectInfo{
				{Name: backupPath + "xtrabackup.stream.00000000000000000000"},
			},
			expected: false,
		},
		"metadata object of another backup": {
			objects: []storage.ObjectInfo{
				{Name: "other-backup/metadata"},
			},
			expected: false,
		},
		"no objects": {
			objects:  nil,
			expected: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			bcp := &api.PerconaXtraDBClusterBackup{
				Status: api.PXCBackupStatus{Destination: testDestination},
			}

			assert.Equal(t, tt.expected, hasMetadata(tt.objects, bcp))
		})
	}
}
