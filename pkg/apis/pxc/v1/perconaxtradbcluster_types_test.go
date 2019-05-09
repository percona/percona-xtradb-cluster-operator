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
					TopologyKey: &defaultAffinityTopologyKey,
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
					TopologyKey: &defaultAffinityTopologyKey,
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
