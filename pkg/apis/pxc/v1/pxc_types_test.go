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

func TestGetLoadBalancerClass(t *testing.T) {
	tests := map[string]struct {
		exposeType          corev1.ServiceType
		loadBalancerClass   *string
		expectedErrorString string
	}{
		"not a LoadBalancer type": {
			exposeType:          corev1.ServiceTypeClusterIP,
			expectedErrorString: "expose type ClusterIP is not LoadBalancer",
		},
		"load balancer class is nil": {
			exposeType:          corev1.ServiceTypeLoadBalancer,
			expectedErrorString: "",
		},
		"load balancer class is empty string": {
			exposeType:          corev1.ServiceTypeLoadBalancer,
			loadBalancerClass:   strPtr(""),
			expectedErrorString: "load balancer class not provided or is empty",
		},
		"valid load balancer class": {
			exposeType:        corev1.ServiceTypeLoadBalancer,
			loadBalancerClass: strPtr("my-lb-class"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			spec := ServiceExpose{
				Type:              tt.exposeType,
				LoadBalancerClass: tt.loadBalancerClass,
			}
			class, err := spec.GetLoadBalancerClass()
			if class != nil && class != tt.loadBalancerClass {
				t.Fatal("expected lb class:", tt.loadBalancerClass, "; got:", class)
			}
			if err != nil && err.Error() != tt.expectedErrorString {
				t.Fatal("expected err:", tt.expectedErrorString, "; got:", err.Error())
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
