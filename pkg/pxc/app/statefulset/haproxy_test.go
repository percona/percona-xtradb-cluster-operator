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

func TestHAProxyHealthCheckEnvVars(t *testing.T) {
	tests := map[string]struct {
		healthCheck     *api.HAProxyHealthCheckSpec
		expectedEnvVars map[string]string
	}{
		"default values": {
			healthCheck: nil,
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 10000 rise 1 fall 2 weight 1",
			},
		},
		"custom interval only": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Interval: func() *int32 { i := int32(3000); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 3000 rise 1 fall 2 weight 1",
			},
		},
		"custom fall only": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Fall: func() *int32 { i := int32(3); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 10000 rise 1 fall 3 weight 1",
			},
		},
		"custom rise only": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Rise: func() *int32 { i := int32(2); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 10000 rise 2 fall 2 weight 1",
			},
		},
		"all custom values": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Interval: func() *int32 { i := int32(3000); return &i }(),
				Fall:     func() *int32 { i := int32(2); return &i }(),
				Rise:     func() *int32 { i := int32(1); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 3000 rise 1 fall 2 weight 1",
			},
		},
		"shutdown on mark down enabled": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				ShutdownOnMarkDown: true,
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS":        "resolvers kubernetes check inter 10000 rise 1 fall 2 weight 1",
				"HA_SHUTDOWN_ON_MARK_DOWN": "yes",
			},
		},
		"all custom values with shutdown": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Interval:           func() *int32 { i := int32(3000); return &i }(),
				Fall:               func() *int32 { i := int32(2); return &i }(),
				Rise:               func() *int32 { i := int32(1); return &i }(),
				ShutdownOnMarkDown: true,
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS":        "resolvers kubernetes check inter 3000 rise 1 fall 2 weight 1",
				"HA_SHUTDOWN_ON_MARK_DOWN": "yes",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Spec: api.PerconaXtraDBClusterSpec{
					CRVersion: "1.18.0",
					HAProxy: &api.HAProxySpec{
						PodSpec: api.PodSpec{
							Image:             "test-image",
							ImagePullPolicy:   corev1.PullIfNotPresent,
							EnvVarsSecretName: "test-secret",
						},
						ExposeReplicas: &api.ReplicasServiceExpose{OnlyReaders: false},
						HealthCheck:    tt.healthCheck,
					},
					PXC: &api.PXCSpec{
						PodSpec: &api.PodSpec{
							Configuration: "config",
						},
					},
				},
			}

			haproxy := &HAProxy{cr: cr}

			containers, err := haproxy.SidecarContainers(&cr.Spec.HAProxy.PodSpec, "test-secret", cr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(containers) != 1 {
				t.Fatalf("expected 1 container, got %d", len(containers))
			}

			c := containers[0]

			// Check that all expected env vars are set correctly
			for expectedName, expectedValue := range tt.expectedEnvVars {
				found := false
				for _, env := range c.Env {
					if env.Name == expectedName {
						found = true
						if env.Value != expectedValue {
							t.Errorf("expected %s=%q, got %q", expectedName, expectedValue, env.Value)
						}
						break
					}
				}
				if !found {
					t.Errorf("%s env var not found in container", expectedName)
				}
			}

			// Verify HA_SHUTDOWN_ON_MARK_DOWN is not present when not expected
			if _, expected := tt.expectedEnvVars["HA_SHUTDOWN_ON_MARK_DOWN"]; !expected {
				for _, env := range c.Env {
					if env.Name == "HA_SHUTDOWN_ON_MARK_DOWN" {
						t.Errorf("unexpected HA_SHUTDOWN_ON_MARK_DOWN env var found")
					}
				}
			}
		})
	}
}
