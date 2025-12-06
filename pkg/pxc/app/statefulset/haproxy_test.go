package statefulset

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/test"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAppContainer_HAProxy(t *testing.T) {
	secretName := "my-secret"

	tests := map[string]struct {
		spec              api.PerconaXtraDBClusterSpec
		expectedContainer func() corev1.Container
	}{
		"latest cr container construction": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				HAProxy: &api.HAProxySpec{
					PodSpec: api.PodSpec{
						Image:             "test-image",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						EnvVarsSecretName: "test-secret",
						LivenessProbes: corev1.Probe{
							TimeoutSeconds: 5,
						},
						ReadinessProbes: corev1.Probe{
							TimeoutSeconds: 15,
						},
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{},
				},
			},
			expectedContainer: func() corev1.Container {
				return defaultExpectedHAProxyContainer()
			},
		},
		"container construction with extra pvcs": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				HAProxy: &api.HAProxySpec{
					PodSpec: api.PodSpec{
						Image:             "test-image",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						EnvVarsSecretName: "test-secret",
						LivenessProbes: corev1.Probe{
							TimeoutSeconds: 5,
						},
						ReadinessProbes: corev1.Probe{
							TimeoutSeconds: 15,
						},
						ExtraPVCs: []api.ExtraPVC{
							{
								Name:      "extra-data-volume",
								ClaimName: "extra-storage-0",
								MountPath: "/var/lib/haproxy-extra",
							},
							{
								Name:      "backup-volume",
								ClaimName: "backup-storage-0",
								MountPath: "/backups",
								SubPath:   "haproxy",
							},
						},
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedHAProxyContainer()
				c.VolumeMounts = append(c.VolumeMounts,
					corev1.VolumeMount{
						Name:      "extra-data-volume",
						MountPath: "/var/lib/haproxy-extra",
					},
					corev1.VolumeMount{
						Name:      "backup-volume",
						MountPath: "/backups",
						SubPath:   "haproxy",
					},
				)
				return c
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					UID:       "test-uid",
				},
				Spec: tt.spec,
			}

			client := test.BuildFakeClient()
			haproxy := &HAProxy{cr: cr}

			c, err := haproxy.AppContainer(t.Context(), client, &tt.spec.HAProxy.PodSpec, secretName, cr, nil)
			assert.Equal(t, tt.expectedContainer(), c)
			assert.NoError(t, err)
		})
	}
}

func defaultExpectedHAProxyContainer() corev1.Container {
	fvar := true
	return corev1.Container{
		Name:            "haproxy",
		Image:           "test-image",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/opt/percona/haproxy-entrypoint.sh"},
		Args:            []string{"haproxy"},
		Ports: []corev1.ContainerPort{
			{ContainerPort: 3306, Name: "mysql"},
			{ContainerPort: 3307, Name: "mysql-replicas"},
			{ContainerPort: 3309, Name: "proxy-protocol"},
			{ContainerPort: 33062, Name: "mysql-admin"},
			{ContainerPort: 33060, Name: "mysqlx"},
			{ContainerPort: 8404, Name: "stats"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "haproxy-custom", MountPath: "/etc/haproxy-custom/"},
			{Name: "haproxy-auto", MountPath: "/etc/haproxy/pxc"},
			{Name: app.BinVolumeName, MountPath: app.BinVolumeMountPath},
			{Name: "mysql-users-secret-file", MountPath: "/etc/mysql/mysql-users-secret"},
			{Name: "test-secret", MountPath: "/etc/mysql/haproxy-env-secret"},
		},
		Env: []corev1.EnvVar{
			{Name: "PXC_SERVICE", Value: "test-cluster-pxc"},
			{Name: "LIVENESS_CHECK_TIMEOUT", Value: "5"},
			{Name: "READINESS_CHECK_TIMEOUT", Value: "15"},
		},
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "test-secret",
					},
					Optional: &fvar,
				},
			},
		},
		ReadinessProbe: &corev1.Probe{
			TimeoutSeconds: 15,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/opt/percona/haproxy_readiness_check.sh"},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			TimeoutSeconds: 5,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/opt/percona/haproxy_liveness_check.sh"},
				},
			},
		},
	}
}

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
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 10000 rise 1 fall 2 weight 1 on-marked-down shutdown-sessions",
			},
		},
		"custom interval only": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Interval: func() *int32 { i := int32(3000); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 3000 rise 1 fall 2 weight 1 on-marked-down shutdown-sessions",
			},
		},
		"custom fall only": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Fall: func() *int32 { i := int32(3); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 10000 rise 1 fall 3 weight 1 on-marked-down shutdown-sessions",
			},
		},
		"custom rise only": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Rise: func() *int32 { i := int32(2); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 10000 rise 2 fall 2 weight 1 on-marked-down shutdown-sessions",
			},
		},
		"all custom values": {
			healthCheck: &api.HAProxyHealthCheckSpec{
				Interval: func() *int32 { i := int32(3000); return &i }(),
				Fall:     func() *int32 { i := int32(2); return &i }(),
				Rise:     func() *int32 { i := int32(1); return &i }(),
			},
			expectedEnvVars: map[string]string{
				"HA_SERVER_OPTIONS": "resolvers kubernetes check inter 3000 rise 1 fall 2 weight 1 on-marked-down shutdown-sessions",
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
					CRVersion: version.Version(),
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
		})
	}
}
