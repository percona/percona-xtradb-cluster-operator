package statefulset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/test"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestAppContainer(t *testing.T) {

	secretName := "my-secret"

	tests := map[string]struct {
		spec              api.PerconaXtraDBClusterSpec
		crUID             types.UID
		expectedContainer func() corev1.Container
		envFromSecret     *corev1.Secret
	}{
		"latest cr container construction ": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{
						Image:           "test-image",
						ImagePullPolicy: corev1.PullIfNotPresent,
						LivenessProbes: corev1.Probe{
							TimeoutSeconds: 5,
						},
						ReadinessProbes: corev1.Probe{
							TimeoutSeconds: 15,
						},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			expectedContainer: func() corev1.Container {
				return defaultExpectedContainer()
			},
		},
		"container construction with jemalloc": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					MySQLAllocator: "jemalloc",
					PodSpec: &api.PodSpec{
						Image:           "test-image",
						ImagePullPolicy: corev1.PullIfNotPresent,
						LivenessProbes: corev1.Probe{
							TimeoutSeconds: 5,
						},
						ReadinessProbes: corev1.Probe{
							TimeoutSeconds: 15,
						},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				c.Env = append(c.Env, corev1.EnvVar{
					Name:  "LD_PRELOAD",
					Value: "/usr/lib64/libjemalloc.so.1",
				})
				return c
			},
		},
		"container construction with tcmalloc": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					MySQLAllocator: "tcmalloc",
					PodSpec: &api.PodSpec{
						Image:           "test-image",
						ImagePullPolicy: corev1.PullIfNotPresent,
						LivenessProbes: corev1.Probe{
							TimeoutSeconds: 5,
						},
						ReadinessProbes: corev1.Probe{
							TimeoutSeconds: 15,
						},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				c.Env = append(c.Env, corev1.EnvVar{
					Name:  "LD_PRELOAD",
					Value: "/usr/lib64/libtcmalloc.so",
				})
				return c
			},
		},
		"container construction with cr configured with jemalloc but priority goes to tcmalloc from envFromSecret": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					MySQLAllocator: "jemalloc",
					PodSpec: &api.PodSpec{
						Image:           "test-image",
						ImagePullPolicy: corev1.PullIfNotPresent,
						LivenessProbes: corev1.Probe{
							TimeoutSeconds: 5,
						},
						ReadinessProbes: corev1.Probe{
							TimeoutSeconds: 15,
						},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			envFromSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"LD_PRELOAD": []byte("/usr/lib64/libtcmalloc.so"),
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				c.Env = append(c.Env, corev1.EnvVar{
					Name:  "LD_PRELOAD",
					Value: "/usr/lib64/libtcmalloc.so",
				})
				return c
			},
		},
		"cr <1.19 with proxysql enabled": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: "1.18.0",
				ProxySQL: &api.ProxySQLSpec{
					PodSpec: api.PodSpec{
						Enabled: true,
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{
						Image:           "test-image",
						ImagePullPolicy: corev1.PullIfNotPresent,
						LivenessProbes: corev1.Probe{
							TimeoutSeconds: 5,
						},
						ReadinessProbes: corev1.Probe{
							TimeoutSeconds: 15,
						},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				c.Env[9].Value = "mysql_native_password"
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

			pxcNode := Node{cr: cr}

			objs := []runtime.Object{cr}
			if tt.envFromSecret != nil {
				objs = append(objs, tt.envFromSecret)
			}

			client := test.BuildFakeClient(objs...)

			c, err := pxcNode.AppContainer(t.Context(), client, tt.spec.PXC.PodSpec, secretName, cr, nil)
			assert.Equal(t, tt.expectedContainer(), c)
			assert.NoError(t, err)
		})
	}
}

func defaultExpectedContainer() corev1.Container {
	return corev1.Container{
		Name:            "pxc",
		Image:           "test-image",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/var/lib/mysql/pxc-entrypoint.sh"},
		Args:            []string{"mysqld"},
		Ports: []corev1.ContainerPort{
			{ContainerPort: 3306, Name: "mysql"},
			{ContainerPort: 4444, Name: "sst"},
			{ContainerPort: 4567, Name: "write-set"},
			{ContainerPort: 4568, Name: "ist"},
			{ContainerPort: 33062, Name: "mysql-admin"},
			{ContainerPort: 33060, Name: "mysqlx"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: app.DataVolumeName, MountPath: "/var/lib/mysql"},
			{Name: "config", MountPath: "/etc/percona-xtradb-cluster.conf.d"},
			{Name: "tmp", MountPath: "/tmp"},
			{Name: "ssl", MountPath: "/etc/mysql/ssl"},
			{Name: "ssl-internal", MountPath: "/etc/mysql/ssl-internal"},
			{Name: "mysql-users-secret-file", MountPath: "/etc/mysql/mysql-users-secret"},
			{Name: "auto-config", MountPath: "/etc/my.cnf.d"},
			{Name: VaultSecretVolumeName, MountPath: "/etc/mysql/vault-keyring-secret"},
			{Name: "mysql-init-file", MountPath: "/etc/mysql/init-file"},
		},
		Env: []corev1.EnvVar{
			{Name: "PXC_SERVICE", Value: "test-cluster-pxc-unready"},
			{Name: "MONITOR_HOST", Value: "%"},
			{Name: "MYSQL_ROOT_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector("my-secret", users.Root),
			}},
			{Name: "XTRABACKUP_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector("my-secret", users.Xtrabackup),
			}},
			{Name: "MONITOR_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector("my-secret", users.Monitor),
			}},
			{Name: "CLUSTER_HASH", Value: "2584669"},
			{Name: "OPERATOR_ADMIN_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector("my-secret", users.Operator),
			}},
			{Name: "LIVENESS_CHECK_TIMEOUT", Value: "5"},
			{Name: "READINESS_CHECK_TIMEOUT", Value: "15"},
			{Name: "DEFAULT_AUTHENTICATION_PLUGIN", Value: "caching_sha2_password"},
			{Name: "MYSQL_NOTIFY_SOCKET", Value: "/var/lib/mysql/notify.sock"},
			{Name: "MYSQL_STATE_FILE", Value: "/var/lib/mysql/mysql.state"},
		},
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "test-secret",
					},
					Optional: pointerToTrue(),
				},
			},
		},
		ReadinessProbe: app.Probe(&corev1.Probe{
			TimeoutSeconds: 15,
		}, "/var/lib/mysql/readiness-check.sh"),
		LivenessProbe: app.Probe(&corev1.Probe{
			TimeoutSeconds: 5,
		}, "/var/lib/mysql/liveness-check.sh"),
	}
}
