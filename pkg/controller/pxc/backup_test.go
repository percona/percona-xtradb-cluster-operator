package pxc

import (
	"testing"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/stretchr/testify/assert"
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
