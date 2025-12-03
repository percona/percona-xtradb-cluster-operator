package statefulset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/test"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestAppContainer_ProxySQL(t *testing.T) {
	secretName := "my-secret"

	tests := map[string]struct {
		spec              api.PerconaXtraDBClusterSpec
		expectedContainer func() corev1.Container
	}{
		"cr 1.18 container construction": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: "1.18.0",
				ProxySQL: &api.ProxySQLSpec{
					PodSpec: api.PodSpec{
						Image:             "test-image",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						EnvVarsSecretName: "test-secret",
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedProxySQLContainer()
				// 1.18 doesn't have scheduler env vars
				c.Env = c.Env[:5]
				return c
			},
		},
		"latest cr container construction - scheduler disabled": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				ProxySQL: &api.ProxySQLSpec{
					PodSpec: api.PodSpec{
						Image:             "test-image",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						EnvVarsSecretName: "test-secret",
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{},
				},
			},
			expectedContainer: func() corev1.Container {
				return defaultExpectedProxySQLContainer()
			},
		},
		"latest cr container construction - scheduler enabled": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				ProxySQL: &api.ProxySQLSpec{
					PodSpec: api.PodSpec{
						Image:             "test-image",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						EnvVarsSecretName: "test-secret",
					},
					Scheduler: api.ProxySQLSchedulerSpec{
						Enabled:                       true,
						WriterIsAlsoReader:            true,
						SuccessThreshold:              1,
						FailureThreshold:              3,
						MaxConnections:                1000,
						PingTimeoutMilliseconds:       1000,
						CheckTimeoutMilliseconds:      2000,
						NodeCheckIntervalMilliseconds: 2000,
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedProxySQLContainer()
				// 1.18 doesn't have scheduler env vars
				c.Env = append(c.Env[:5], []corev1.EnvVar{
					{Name: "SCHEDULER_CHECKTIMEOUT", Value: "2000"},
					{Name: "SCHEDULER_WRITERALSOREADER", Value: "1"},
					{Name: "SCHEDULER_RETRYUP", Value: "1"},
					{Name: "SCHEDULER_RETRYDOWN", Value: "3"},
					{Name: "SCHEDULER_PINGTIMEOUT", Value: "1000"},
					{Name: "SCHEDULER_NODECHECKINTERVAL", Value: "2000"},
					{Name: "SCHEDULER_MAXCONNECTIONS", Value: "1000"},
					{Name: "SCHEDULER_ENABLED", Value: "true"},
				}...)
				return c
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
					UID:  "test-uid",
				},
				Spec: tt.spec,
			}

			client := test.BuildFakeClient()
			proxySQL := &Proxy{cr: cr}

			c, err := proxySQL.AppContainer(t.Context(), client, &tt.spec.ProxySQL.PodSpec, secretName, cr, nil)
			assert.Equal(t, tt.expectedContainer(), c)
			assert.NoError(t, err)
		})
	}
}

func defaultExpectedProxySQLContainer() corev1.Container {
	return corev1.Container{
		Name:            "proxysql",
		Image:           "test-image",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/opt/percona/proxysql-entrypoint.sh"},
		Args:            []string{"proxysql", "-f", "-c", "/etc/proxysql/proxysql.cnf", "--reload"},
		Ports: []corev1.ContainerPort{
			{ContainerPort: 3306, Name: "mysql"},
			{ContainerPort: 6032, Name: "proxyadm"},
			{ContainerPort: 6070, Name: "stats"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: proxyDataVolumeName, MountPath: "/var/lib/proxysql"},
			{Name: "ssl", MountPath: "/etc/proxysql/ssl"},
			{Name: "ssl-internal", MountPath: "/etc/proxysql/ssl-internal"},
			{Name: app.BinVolumeName, MountPath: app.BinVolumeMountPath},
		},
		Env: []corev1.EnvVar{
			{Name: "PXC_SERVICE", Value: "test-cluster-pxc"},
			{Name: "OPERATOR_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector("my-secret", users.Operator),
			}},
			{Name: "PROXY_ADMIN_USER", Value: "proxyadmin"},
			{Name: "PROXY_ADMIN_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector("my-secret", users.ProxyAdmin),
			}},
			{Name: "MONITOR_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector("my-secret", users.Monitor),
			}},
			{Name: "SCHEDULER_CHECKTIMEOUT", Value: "0"},
			{Name: "SCHEDULER_WRITERALSOREADER", Value: "0"},
			{Name: "SCHEDULER_RETRYUP", Value: "0"},
			{Name: "SCHEDULER_RETRYDOWN", Value: "0"},
			{Name: "SCHEDULER_PINGTIMEOUT", Value: "0"},
			{Name: "SCHEDULER_NODECHECKINTERVAL", Value: "0"},
			{Name: "SCHEDULER_MAXCONNECTIONS", Value: "0"},
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
	}
}

func TestSidecarContainers_ProxySQL(t *testing.T) {
	tests := map[string]struct {
		spec               api.PodSpec
		secrets            string
		crVersion          string
		scheduler          api.ProxySQLSchedulerSpec
		expectedContainers func() []corev1.Container
	}{
		"success - default container construction": {
			spec: api.PodSpec{
				Image:             "test-image",
				ImagePullPolicy:   corev1.PullIfNotPresent,
				EnvVarsSecretName: "test-secret",
			},
			secrets:   "monitor-secret",
			crVersion: version.Version(),
			expectedContainers: func() []corev1.Container {
				return defaultExpectedProxySQLSidecarContainers()
			},
		},
		"scheduler enabled - only pxc-monit container": {
			spec: api.PodSpec{
				Image:             "test-image",
				ImagePullPolicy:   corev1.PullIfNotPresent,
				EnvVarsSecretName: "test-secret",
			},
			secrets:   "monitor-secret",
			crVersion: version.Version(),
			scheduler: api.ProxySQLSchedulerSpec{
				Enabled: true,
			},
			expectedContainers: func() []corev1.Container {
				c := defaultExpectedProxySQLSidecarContainers()
				pxcMonit := c[0]
				pxcMonit.Env = append(pxcMonit.Env, corev1.EnvVar{
					Name:  "SCHEDULER_ENABLED",
					Value: "true",
				})
				return []corev1.Container{pxcMonit}
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
					CRVersion: tt.crVersion,
					ProxySQL: &api.ProxySQLSpec{
						PodSpec:   tt.spec,
						Scheduler: tt.scheduler,
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
			assert.NoError(t, err)

			expected := tt.expectedContainers()
			assert.Len(t, containers, len(expected))
			for i, c := range containers {
				assert.Equal(t, expected[i], c)
			}
		})
	}
}

func defaultExpectedProxySQLSidecarContainers() []corev1.Container {
	return []corev1.Container{
		{
			Name:            "pxc-monit",
			Image:           "test-image",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/opt/percona/proxysql-entrypoint.sh"},
			Args: []string{
				"/opt/percona/peer-list",
				"-on-change=/opt/percona/proxysql_add_pxc_nodes.sh",
				"-service=$(PXC_SERVICE)",
				"-protocol=$(PEER_LIST_SRV_PROTOCOL)",
			},
			Env: []corev1.EnvVar{
				{Name: "PXC_SERVICE", Value: "test-cluster-pxc"},
				{Name: "OPERATOR_PASSWORD", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector("monitor-secret", users.Operator),
				}},
				{Name: "PROXY_ADMIN_USER", Value: "proxyadmin"},
				{Name: "PROXY_ADMIN_PASSWORD", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector("monitor-secret", users.ProxyAdmin),
				}},
				{Name: "MONITOR_PASSWORD", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector("monitor-secret", users.Monitor),
				}},
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
			VolumeMounts: []corev1.VolumeMount{
				{Name: "bin", MountPath: "/opt/percona"},
				{Name: "ssl", MountPath: "/etc/proxysql/ssl"},
				{Name: "ssl-internal", MountPath: "/etc/proxysql/ssl-internal"},
			},
		},
		{
			Name:            "proxysql-monit",
			Image:           "test-image",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/opt/percona/proxysql-entrypoint.sh"},
			Args: []string{
				"/opt/percona/peer-list",
				"-on-change=/opt/percona/proxysql_add_proxysql_nodes.sh",
				"-service=$(PROXYSQL_SERVICE)",
				"-protocol=$(PEER_LIST_SRV_PROTOCOL)",
			},
			Env: []corev1.EnvVar{
				{Name: "PROXYSQL_SERVICE", Value: "test-cluster-proxysql-unready"},
				{Name: "OPERATOR_PASSWORD", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector("monitor-secret", users.Operator),
				}},
				{Name: "PROXY_ADMIN_USER", Value: "proxyadmin"},
				{Name: "PROXY_ADMIN_PASSWORD", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector("monitor-secret", users.ProxyAdmin),
				}},
				{Name: "MONITOR_PASSWORD", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector("monitor-secret", users.Monitor),
				}},
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
			VolumeMounts: []corev1.VolumeMount{
				{Name: "bin", MountPath: "/opt/percona"},
			},
		},
	}
}
