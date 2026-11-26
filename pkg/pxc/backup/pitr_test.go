package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestGetLatestSuccessfulBackup(t *testing.T) {
	baseTime := time.Date(2026, 5, 18, 16, 3, 0, 0, time.UTC)

	tests := []struct {
		name        string
		clusterName string
		backups     []runtime.Object
		expected    string
		expectedErr error
	}{
		{
			name:        "returns latest successful backup for cluster when another cluster has newer backup",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newBackup("backcl2", "cluster2", api.BackupSucceeded, baseTime.Add(2*time.Minute)),
				newBackup("backcl1", "cluster1", api.BackupSucceeded, baseTime.Add(time.Minute)),
			},
			expected: "backcl1",
		},
		{
			name:        "returns latest successful backup for cluster",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newBackup("older", "cluster1", api.BackupSucceeded, baseTime.Add(time.Minute)),
				newBackup("latest", "cluster1", api.BackupSucceeded, baseTime.Add(2*time.Minute)),
				newBackup("some-other-cluster", "cluster2", api.BackupSucceeded, baseTime.Add(2*time.Minute)),
			},
			expected: "latest",
		},
		{
			name:        "ignores newer failed backup for cluster",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newBackup("aaa-failed", "cluster1", api.BackupFailed, baseTime.Add(3*time.Minute)),
				newBackup("zzz-succeeded", "cluster1", api.BackupSucceeded, baseTime.Add(2*time.Minute)),
			},
			expected: "zzz-succeeded",
		},
		{
			name:        "returns no backups when cluster has no successful backups",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newBackup("failed", "cluster1", api.BackupFailed, baseTime.Add(2*time.Minute)),
				newBackup("backcl2", "cluster2", api.BackupSucceeded, baseTime.Add(3*time.Minute)),
			},
			expectedErr: ErrNoBackups,
		},
		{
			name:        "error when no backups exist for a cluster",
			clusterName: "cluster1",
			backups:     []runtime.Object{},
			expectedErr: ErrNoBackups,
		},
		{
			name:        "returns no backup when one and only backup is running",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newBackup("running-backup", "cluster1", api.BackupRunning, baseTime.Add(2*time.Minute)),
			},
			expectedErr: ErrNoBackups,
		},
		{
			name:        "running backup is ignored",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newBackup("running-backup", "cluster1", api.BackupRunning, baseTime.Add(2*time.Minute)),
				newBackup("succeeded-backup", "cluster1", api.BackupSucceeded, baseTime.Add(3*time.Minute)),
			},
			expected: "succeeded-backup",
		},
		{
			name:        "newer logical backup is ignored in favour of an older full backup",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newTypedBackup("full", "cluster1", api.BackupTypeFull, baseTime.Add(time.Minute)),
				newTypedBackup("logical", "cluster1", api.BackupTypeLogical, baseTime.Add(2*time.Minute)),
			},
			expected: "full",
		},
		{
			name:        "backup without a type is a full backup and is preferred when it is newer than a logical one",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newTypedBackup("logical", "cluster1", api.BackupTypeLogical, baseTime.Add(time.Minute)),
				newTypedBackup("untyped", "cluster1", "", baseTime.Add(2*time.Minute)),
			},
			expected: "untyped",
		},
		{
			name:        "backup without a type is still chosen when a logical backup is newer",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newTypedBackup("untyped", "cluster1", "", baseTime.Add(time.Minute)),
				newTypedBackup("logical", "cluster1", api.BackupTypeLogical, baseTime.Add(2*time.Minute)),
			},
			expected: "untyped",
		},
		{
			name:        "returns no backups when cluster has only logical backups",
			clusterName: "cluster1",
			backups: []runtime.Object{
				newTypedBackup("logical1", "cluster1", api.BackupTypeLogical, baseTime.Add(time.Minute)),
				newTypedBackup("logical2", "cluster1", api.BackupTypeLogical, baseTime.Add(2*time.Minute)),
			},
			expectedErr: ErrNoBackups,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.clusterName,
					Namespace: "test-ns",
				},
			}

			backup, err := getLatestSuccessfulPhysicalBackup(t.Context(), buildBackupFakeClient(t, tt.backups...), cr)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, backup)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, backup)
			assert.Equal(t, tt.expected, backup.Name)
			assert.Equal(t, tt.clusterName, backup.Spec.PXCCluster)
			assert.Equal(t, api.BackupSucceeded, backup.Status.State)
		})
	}
}

func buildBackupFakeClient(t *testing.T, backups ...runtime.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, api.SchemeBuilder.AddToScheme(scheme))

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(backups...).
		WithIndex(&api.PerconaXtraDBClusterBackup{}, PXCClusterBackupField, func(obj client.Object) []string {
			backup, ok := obj.(*api.PerconaXtraDBClusterBackup)
			if !ok {
				return nil
			}
			return []string{backup.Spec.PXCCluster}
		}).
		Build()
}

func newBackup(name, clusterName string, state api.PXCBackupState, createdAt time.Time) *api.PerconaXtraDBClusterBackup {
	return &api.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "test-ns",
			CreationTimestamp: metav1.NewTime(createdAt),
		},
		Spec: api.PXCBackupSpec{
			PXCCluster: clusterName,
		},
		Status: api.PXCBackupStatus{
			State: state,
		},
	}
}

// newTypedBackup returns a succeeded backup of the given type.
func newTypedBackup(name, clusterName string, backupType api.PXCBackupType, createdAt time.Time) *api.PerconaXtraDBClusterBackup {
	backup := newBackup(name, clusterName, api.BackupSucceeded, createdAt)
	backup.Spec.Type = backupType
	backup.Status.Type = backupType
	return backup
}
