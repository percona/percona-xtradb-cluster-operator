package pxcrestore

import (
	"testing"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidatePITRTarget(t *testing.T) {
	mockBackup := &api.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-backup",
		},
		Spec: api.PXCBackupSpec{
			PXCCluster: "test-cluster",
		},
		Status: api.PXCBackupStatus{
			State: api.BackupSucceeded,
		},
	}

	mockRestore := &api.PerconaXtraDBClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-restore",
		},
		Spec: api.PerconaXtraDBClusterRestoreSpec{},
	}

	testCases := []struct {
		desc        string
		backup      *api.PerconaXtraDBClusterBackup
		restore     *api.PerconaXtraDBClusterRestore
		expectedErr error
	}{
		{
			desc:        "PITR disabled",
			backup:      mockBackup,
			restore:     mockRestore,
			expectedErr: nil,
		},
		{
			desc: "LatestRestorableTime not known",
			backup: func() *api.PerconaXtraDBClusterBackup {
				backup := mockBackup.DeepCopy()
				backup.Status.LatestRestorableTime = nil
				return backup
			}(),
			restore: func() *api.PerconaXtraDBClusterRestore {
				restore := mockRestore.DeepCopy()
				restore.Spec.PITR = &api.PITR{
					Type: api.PITRTypeDate,
					Date: "2021-01-01 00:00:00",
				}
				return restore
			}(),
			expectedErr: errors.New("latest restorable time is not known"),
		},
		{
			desc: "Target datetime is after the latest restorable time",
			backup: func() *api.PerconaXtraDBClusterBackup {
				backup := mockBackup.DeepCopy()
				latestRestorableTime, _ := time.Parse(time.DateTime, "2025-07-16 10:30:00")
				backup.Status.LatestRestorableTime = &metav1.Time{Time: latestRestorableTime}
				return backup
			}(),

			restore: func() *api.PerconaXtraDBClusterRestore {
				restore := mockRestore.DeepCopy()
				restore.Spec.PITR = &api.PITR{
					Type: api.PITRTypeDate,
					Date: "2025-07-16 11:30:00",
				}
				return restore
			}(),
			expectedErr: errors.New("target datetime is after the latest restorable time"),
		},
		{
			desc: "Target datetime is before the latest restorable time",
			backup: func() *api.PerconaXtraDBClusterBackup {
				backup := mockBackup.DeepCopy()
				latestRestorableTime, _ := time.Parse(time.DateTime, "2025-07-16 10:30:00")
				backup.Status.LatestRestorableTime = &metav1.Time{Time: latestRestorableTime}
				return backup
			}(),

			restore: func() *api.PerconaXtraDBClusterRestore {
				restore := mockRestore.DeepCopy()
				restore.Spec.PITR = &api.PITR{
					Type: api.PITRTypeDate,
					Date: "2025-07-16 09:30:00",
				}
				return restore
			}(),
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := validatePITRTarget(tc.backup, tc.restore)
			if tc.expectedErr != nil {
				assert.Error(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
