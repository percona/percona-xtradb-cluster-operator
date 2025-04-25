package statefulset

import (
	"reflect"
	"testing"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSidecarContainers_HAProxy(t *testing.T) {
	tests := map[string]struct {
		spec            api.PodSpec
		secrets         string
		crVersion       string
		expectedName    string
		expectedImage   string
		expectedArgs    []string
		expectedEnvFrom []corev1.EnvFromSource
		expectError     bool
	}{
		"success - container construction": {
			spec: api.PodSpec{
				Image:             "test-image",
				ImagePullPolicy:   corev1.PullIfNotPresent,
				EnvVarsSecretName: "test-secret",
			},
			secrets:       "monitor-secret",
			crVersion:     "1.18.0",
			expectedName:  "pxc-monit",
			expectedImage: "test-image",
			expectedArgs: []string{
				"/opt/percona/peer-list",
				"-on-change=/opt/percona/haproxy_add_pxc_nodes.sh",
				"-service=$(PXC_SERVICE)",
				"-protocol=$(PEER_LIST_SRV_PROTOCOL)",
			},
			expectedEnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						Optional: pointerToTrue(),
					},
				},
			},
			expectError: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Spec: api.PerconaXtraDBClusterSpec{
					CRVersion: tt.crVersion,
					HAProxy: &api.HAProxySpec{
						PodSpec:        tt.spec,
						ExposeReplicas: &api.ReplicasServiceExpose{OnlyReaders: true},
					},
					PXC: &api.PXCSpec{
						PodSpec: &api.PodSpec{
							Configuration: "config",
						},
					},
				},
			}

			haproxy := &HAProxy{cr: cr}

			containers, err := haproxy.SidecarContainers(&tt.spec, tt.secrets, cr)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if len(containers) != 1 {
				t.Errorf("expected 1 container, got %d", len(containers))
			}

			c := containers[0]
			if c.Name != tt.expectedName {
				t.Errorf("expected container name %s, got %s", tt.expectedName, c.Name)
			}
			if c.Image != tt.expectedImage {
				t.Errorf("expected image %s, got %s", tt.expectedImage, c.Image)
			}
			if !reflect.DeepEqual(c.Args, tt.expectedArgs) {
				t.Errorf("expected args %s, got %s", tt.expectedArgs, c.Args)
			}
			if len(tt.expectedEnvFrom) != len(c.EnvFrom) {
				t.Errorf("expected length envFrom %d, got %d", len(tt.expectedEnvFrom), len(c.EnvFrom))
			}
			if !reflect.DeepEqual(c.EnvFrom, tt.expectedEnvFrom) {
				t.Errorf("expected EnvFrom %v, got %v", tt.expectedEnvFrom, c.EnvFrom)
			}
		})
	}
}

func pointerToTrue() *bool {
	b := true
	return &b
}
