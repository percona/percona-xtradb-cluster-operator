package v1

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestReconcileAffinity(t *testing.T) {
	cases := []struct {
		name     string
		pod      *PodSpec
		desiered *PodSpec
	}{
		{
			name: "no affinity set",
			pod:  &PodSpec{},
			desiered: &PodSpec{
				Affinity: &PodAffinity{
					TopologyKey: &DefaultAffinityTopologyKey,
				},
			},
		},
		{
			name: "wrong antiAffinityTopologyKey",
			pod: &PodSpec{
				Affinity: &PodAffinity{
					TopologyKey: func(s string) *string { return &s }("beta.kubernetes.io/instance-type"),
				},
			},
			desiered: &PodSpec{
				Affinity: &PodAffinity{
					TopologyKey: &DefaultAffinityTopologyKey,
				},
			},
		},
		{
			name: "valid antiAffinityTopologyKey",
			pod: &PodSpec{
				Affinity: &PodAffinity{
					TopologyKey: func(s string) *string { return &s }("kubernetes.io/hostname"),
				},
			},
			desiered: &PodSpec{
				Affinity: &PodAffinity{
					TopologyKey: func(s string) *string { return &s }("kubernetes.io/hostname"),
				},
			},
		},
		{
			name: "valid antiAffinityTopologyKey with Advanced",
			pod: &PodSpec{
				Affinity: &PodAffinity{
					TopologyKey: func(s string) *string { return &s }("kubernetes.io/hostname"),
					Advanced: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{},
					},
				},
			},
			desiered: &PodSpec{
				Affinity: &PodAffinity{
					Advanced: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{},
					},
				},
			},
		},
	}

	for _, c := range cases {
		c.pod.reconcileAffinityOpts()
		if !reflect.DeepEqual(c.desiered.Affinity, c.pod.Affinity) {
			t.Errorf("case %q:\n want: %#v\n have: %#v", c.name, c.desiered.Affinity, c.pod.Affinity)
		}
	}
}

func TestGetRetention(t *testing.T) {
	tests := map[string]struct {
		input    PXCScheduledBackupSchedule
		expected *PXCScheduledBackupRetention
	}{
		"both keep and retention are configured - the retention config is used": {
			input: PXCScheduledBackupSchedule{
				Keep: 3,
				Retention: &PXCScheduledBackupRetention{
					Type:              pxcScheduledBackupRetentionCount,
					Count:             4,
					DeleteFromStorage: false,
				},
			},
			expected: &PXCScheduledBackupRetention{
				Type:              pxcScheduledBackupRetentionCount,
				Count:             4,
				DeleteFromStorage: false,
			},
		},
		"only keep is configured - the keep config is transformed to PXCScheduledBackupRetention": {
			input: PXCScheduledBackupSchedule{
				Keep: 10,
			},
			expected: &PXCScheduledBackupRetention{
				Type:              "count",
				Count:             10,
				DeleteFromStorage: true,
			},
		},
		"nothing is configure - return zero count": {
			input: PXCScheduledBackupSchedule{
				Keep: 0,
			},
			expected: &PXCScheduledBackupRetention{
				Type:              "count",
				Count:             0,
				DeleteFromStorage: true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := tc.input.GetRetention()

			if actual.Type != tc.expected.Type {
				t.Errorf("unexpected Type: got %q, want %q", actual.Type, tc.expected.Type)
			}

			if actual.Count != tc.expected.Count {
				t.Errorf("unexpected Count: got %d, want %d", actual.Count, tc.expected.Count)
			}

			if actual.DeleteFromStorage != tc.expected.DeleteFromStorage {
				t.Errorf("unexpected DeleteFromStorage: got %t, want %t", actual.DeleteFromStorage, tc.expected.DeleteFromStorage)
			}
		})
	}
}
