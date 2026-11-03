package pxc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

func TestShouldRecreateJob(t *testing.T) {
	tests := map[string]struct {
		job      BackupScheduleJob
		schedule pxcv1.PXCScheduledBackupSchedule
		expected bool
	}{
		"no need to recreate": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
			},
			expected: false,
		},
		"no need to recreate (with retention)": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
					Retention: &pxcv1.PXCScheduledBackupRetention{
						Count:             3,
						DeleteFromStorage: true,
					},
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
				Retention: &pxcv1.PXCScheduledBackupRetention{
					Count:             5,
					DeleteFromStorage: true,
				},
			},
			expected: false,
		},
		"storage changed": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage-1",
				Schedule:    "10 4 * * *",
			},
			expected: true,
		},
		"schedule changed": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 5 * * *",
			},
			expected: true,
		},
		"deleteFromStorage changed": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
					Retention: &pxcv1.PXCScheduledBackupRetention{
						Count:             3,
						DeleteFromStorage: false,
					},
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
				Retention: &pxcv1.PXCScheduledBackupRetention{
					Count:             3,
					DeleteFromStorage: true,
				},
			},
			expected: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := shouldRecreateBackupJob(tt.schedule, tt.job)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestOldScheduledBackups covers which backups count against a schedule's
// `keep` retention. Incremental backups must not: they are removed by the
// cascade that runs when their base is reaped, and counting them would let
// rotation delete a base out from under its own chain.
func TestOldScheduledBackups(t *testing.T) {
	const (
		clusterName = "cluster1"
		namespace   = "pxc"
		ancestor    = "daily-backup"
	)

	baseTime := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)

	cluster := func(crVersion string) *pxcv1.PerconaXtraDBCluster {
		return &pxcv1.PerconaXtraDBCluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: namespace},
			Spec:       pxcv1.PerconaXtraDBClusterSpec{CRVersion: crVersion},
		}
	}

	backup := func(name string, backupType pxcv1.PXCBackupType, state pxcv1.PXCBackupState, age int) *pxcv1.PerconaXtraDBClusterBackup {
		return &pxcv1.PerconaXtraDBClusterBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				CreationTimestamp: metav1.NewTime(baseTime.Add(time.Duration(age) * time.Minute)),
				Labels: map[string]string{
					naming.LabelPerconaClusterName:        clusterName,
					naming.LabelPerconaBackupAncestorName: ancestor,
				},
			},
			Spec:   pxcv1.PXCBackupSpec{PXCCluster: clusterName},
			Status: pxcv1.PXCBackupStatus{Type: backupType, State: state},
		}
	}

	tests := map[string]struct {
		crVersion string
		keep      int
		backups   []client.Object
		expected  []string
	}{
		"nothing to reap when fewer full backups than keep": {
			keep: 3,
			backups: []client.Object{
				backup("full-1", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 1),
				backup("full-2", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 2),
			},
			expected: nil,
		},
		"oldest full backups beyond keep are reaped": {
			keep: 1,
			backups: []client.Object{
				backup("full-1", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 1),
				backup("full-2", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 2),
				backup("full-3", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 3),
			},
			expected: []string{"full-1", "full-2"},
		},
		"incrementals do not count towards keep": {
			keep: 2,
			backups: []client.Object{
				backup("full-1", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 1),
				backup("incr-1", pxcv1.BackupTypeIncremental, pxcv1.BackupSucceeded, 2),
				backup("incr-2", pxcv1.BackupTypeIncremental, pxcv1.BackupSucceeded, 3),
				backup("full-2", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 4),
				backup("incr-3", pxcv1.BackupTypeIncremental, pxcv1.BackupSucceeded, 5),
			},
			expected: nil,
		},
		"incrementals are never reaped directly": {
			keep: 1,
			backups: []client.Object{
				backup("full-1", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 1),
				backup("incr-1", pxcv1.BackupTypeIncremental, pxcv1.BackupSucceeded, 2),
				backup("incr-2", pxcv1.BackupTypeIncremental, pxcv1.BackupSucceeded, 3),
				backup("full-2", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 4),
			},
			expected: []string{"full-1"},
		},
		"backups with an empty status type count as full": {
			keep: 1,
			backups: []client.Object{
				backup("legacy-1", "", pxcv1.BackupSucceeded, 1),
				backup("legacy-2", "", pxcv1.BackupSucceeded, 2),
			},
			expected: []string{"legacy-1"},
		},
		"unsuccessful backups are never reaped": {
			keep: 1,
			backups: []client.Object{
				backup("failed-1", pxcv1.BackupTypeFull, pxcv1.BackupFailed, 1),
				backup("running-1", pxcv1.BackupTypeFull, pxcv1.BackupRunning, 2),
				backup("full-1", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 3),
			},
			expected: nil,
		},
		"clusters below 1.21.0 keep counting incrementals": {
			crVersion: "1.20.0",
			keep:      2,
			backups: []client.Object{
				backup("full-1", pxcv1.BackupTypeFull, pxcv1.BackupSucceeded, 1),
				backup("incr-1", pxcv1.BackupTypeIncremental, pxcv1.BackupSucceeded, 2),
				backup("incr-2", pxcv1.BackupTypeIncremental, pxcv1.BackupSucceeded, 3),
			},
			expected: []string{"full-1"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			crVersion := tt.crVersion
			if crVersion == "" {
				crVersion = "1.21.0"
			}

			scheme := runtime.NewScheme()
			require.NoError(t, pxcv1.SchemeBuilder.AddToScheme(scheme))

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.backups...).
				Build()

			r := &ReconcilePerconaXtraDBCluster{client: cl, scheme: scheme}

			old, err := r.oldScheduledBackups(t.Context(), cluster(crVersion), ancestor, tt.keep)
			require.NoError(t, err)

			names := make([]string, 0, len(old))
			for i := range old {
				names = append(names, old[i].Name)
			}

			assert.ElementsMatch(t, tt.expected, names)
		})
	}
}
