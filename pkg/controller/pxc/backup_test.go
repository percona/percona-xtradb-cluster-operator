package pxc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
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
		"backup type changed from full to logical": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
					Type:        pxcv1.BackupTypeFull,
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
				Type:        pxcv1.BackupTypeLogical,
			},
			expected: true,
		},
		"backup type changed from logical to full": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
					Type:        pxcv1.BackupTypeLogical,
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
				Type:        pxcv1.BackupTypeFull,
			},
			expected: true,
		},
		"backup type changed from not set to logical": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
				Type:        pxcv1.BackupTypeLogical,
			},
			expected: true,
		},
		"no need to recreate (backup type not set and full are the same)": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
				Type:        pxcv1.BackupTypeFull,
			},
			expected: false,
		},
		"no need to recreate (logical backup type unchanged)": {
			job: BackupScheduleJob{
				PXCScheduledBackupSchedule: pxcv1.PXCScheduledBackupSchedule{
					StorageName: "test-storage",
					Schedule:    "10 4 * * *",
					Type:        pxcv1.BackupTypeLogical,
				},
			},
			schedule: pxcv1.PXCScheduledBackupSchedule{
				StorageName: "test-storage",
				Schedule:    "10 4 * * *",
				Type:        pxcv1.BackupTypeLogical,
			},
			expected: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := shouldRecreateBackupJob(tt.schedule, tt.job)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNewScheduledBackup(t *testing.T) {
	startingDeadline := int64(300)

	tests := map[string]struct {
		schedule     pxcv1.PXCScheduledBackupSchedule
		finalizers   []string
		expectedType pxcv1.PXCBackupType
	}{
		"schedule without a type creates a full backup": {
			schedule: pxcv1.PXCScheduledBackupSchedule{
				Name:        "daily",
				StorageName: "s3-us-west",
				Schedule:    "0 0 * * *",
			},
			finalizers:   []string{"percona.com/delete-backup"},
			expectedType: pxcv1.BackupTypeFull,
		},
		"full schedule creates a full backup": {
			schedule: pxcv1.PXCScheduledBackupSchedule{
				Name:        "daily",
				StorageName: "s3-us-west",
				Schedule:    "0 0 * * *",
				Type:        pxcv1.BackupTypeFull,
			},
			finalizers:   []string{},
			expectedType: pxcv1.BackupTypeFull,
		},
		"logical schedule creates a logical backup": {
			schedule: pxcv1.PXCScheduledBackupSchedule{
				Name:        "daily-logical",
				StorageName: "s3-us-west",
				Schedule:    "0 1 * * *",
				Type:        pxcv1.BackupTypeLogical,
			},
			finalizers:   []string{"percona.com/delete-backup"},
			expectedType: pxcv1.BackupTypeLogical,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &pxcv1.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: "test-ns",
				},
				Spec: pxcv1.PerconaXtraDBClusterSpec{
					CRVersion: version.Version(),
					Backup: &pxcv1.BackupSpec{
						StartingDeadlineSeconds: &startingDeadline,
					},
				},
			}

			bcp := newScheduledBackup(cr, tt.schedule, tt.finalizers)

			assert.Equal(t, tt.expectedType, bcp.Spec.Type)
			assert.Equal(t, "cluster1", bcp.Spec.PXCCluster)
			assert.Equal(t, tt.schedule.StorageName, bcp.Spec.StorageName)
			assert.Equal(t, &startingDeadline, bcp.Spec.StartingDeadlineSeconds)
			assert.Equal(t, "test-ns", bcp.Namespace)
			// the rest of the name is the current time and a checksum of the schedule
			assert.True(t, strings.HasPrefix(bcp.Name, "cron-cluster1-"+tt.schedule.StorageName+"-"), "unexpected name %s", bcp.Name)
			assert.Equal(t, naming.LabelsScheduledBackup(cr, tt.schedule.Name), bcp.Labels)
			assert.Equal(t, tt.finalizers, bcp.Finalizers)
		})
	}
}
