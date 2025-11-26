package pxcbackup

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/stretchr/testify/assert"
)

func Test_getPXCBackupStateFromJob(t *testing.T) {
	tests := []struct {
		name     string
		job      *batchv1.Job
		expected api.PXCBackupState
	}{
		{
			name: "Job with Ready = 1 should return BackupRunning",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready: ptr.To[int32](1),
				},
			},
			expected: api.BackupRunning,
		},
		{
			name: "Job with Ready = 0 and JobFailed condition True should return BackupFailed",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready: ptr.To[int32](0),
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: api.BackupFailed,
		},
		{
			name: "Job with Ready = 0 and JobComplete condition True should return BackupSucceeded",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready: ptr.To[int32](0),
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: api.BackupSucceeded,
		},
		{
			name: "Job with Ready = 0 and no conditions should return BackupStarting",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready:      ptr.To[int32](0),
					Conditions: []batchv1.JobCondition{},
				},
			},
			expected: api.BackupStarting,
		},
		{
			name: "Job with Ready = nil and no conditions should return BackupStarting",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready:      nil,
					Conditions: []batchv1.JobCondition{},
				},
			},
			expected: api.BackupStarting,
		},
		{
			name: "Job with Ready = 0 and conditions with Status False should return BackupStarting",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready: ptr.To[int32](0),
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: api.BackupStarting,
		},
		{
			name: "Job with Ready = 0 and JobFailed condition True should return BackupFailed even with other conditions",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready: ptr.To[int32](0),
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: api.BackupFailed,
		},
		{
			name: "Job with Ready = 0 and JobComplete condition True should return BackupSucceeded even with other conditions",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Ready: ptr.To[int32](0),
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: api.BackupSucceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPXCBackupStateFromJob(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}
