package statefulset

import (
	"reflect"
	"testing"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSidecarContainers_ProxySQL(t *testing.T) {
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
			expectedName:  "proxysql-monit",
			expectedImage: "test-image",
			expectedArgs: []string{
				"/opt/percona/peer-list",
				"-on-change=/opt/percona/proxysql_add_proxysql_nodes.sh",
				"-service=$(PROXYSQL_SERVICE)",
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
					ProxySQL: &api.ProxySQLSpec{
						PodSpec: tt.spec,
					},
					PXC: &api.PXCSpec{
						PodSpec: &api.PodSpec{
							Configuration: "config",
						},
					},
				},
			}

			proxySQL := &Proxy{cr: cr}

			containers, err := proxySQL.SidecarContainers(&tt.spec, tt.secrets, cr)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if len(containers) != 2 {
				t.Errorf("expected 2 container, got %d", len(containers))
			}

			for _, c := range containers {
				if c.Name == "proxysql-monit" {
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
						t.Errorf("expected length of envFrom %d, got %d", len(tt.expectedEnvFrom), len(c.EnvFrom))
					}
					if !reflect.DeepEqual(c.EnvFrom, tt.expectedEnvFrom) {
						t.Errorf("expected EnvFrom %v, got %v", tt.expectedEnvFrom, c.EnvFrom)
					}
				}
			}
		})
	}
}
