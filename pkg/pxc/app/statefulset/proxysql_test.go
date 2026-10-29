package statefulset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
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
				// probes & mounted volume were added in 1.21.0
				c.LivenessProbe = nil
				c.ReadinessProbe = nil
				c.VolumeMounts = c.VolumeMounts[:len(c.VolumeMounts)-1]
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
					{Name: "PERCONA_SCHEDULER_CFG", Value: "/tmp/scheduler-config.toml"},
					{Name: "SCHEDULER_ENABLED", Value: "true"},
					{Name: "PXC_READ_ONLY", Value: "false"},
					{Name: "LIVENESS_CHECK_TIMEOUT", Value: "0"},
					{Name: "READINESS_CHECK_TIMEOUT", Value: "0"},
				}...)
				return c
			},
		},
		"latest cr container construction - scheduler enabled, read only cluster": {
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
					ReplicationChannels: []api.ReplicationChannel{
						{
							Name:     "replica-channel",
							IsSource: false,
						},
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedProxySQLContainer()
				c.Env = append(c.Env[:5], []corev1.EnvVar{
					{Name: "SCHEDULER_CHECKTIMEOUT", Value: "2000"},
					{Name: "SCHEDULER_WRITERALSOREADER", Value: "1"},
					{Name: "SCHEDULER_RETRYUP", Value: "1"},
					{Name: "SCHEDULER_RETRYDOWN", Value: "3"},
					{Name: "SCHEDULER_PINGTIMEOUT", Value: "1000"},
					{Name: "SCHEDULER_NODECHECKINTERVAL", Value: "2000"},
					{Name: "SCHEDULER_MAXCONNECTIONS", Value: "1000"},
					{Name: "PERCONA_SCHEDULER_CFG", Value: "/tmp/scheduler-config.toml"},
					{Name: "SCHEDULER_ENABLED", Value: "true"},
					{Name: "PXC_READ_ONLY", Value: "true"},
					{Name: "LIVENESS_CHECK_TIMEOUT", Value: "0"},
					{Name: "READINESS_CHECK_TIMEOUT", Value: "0"},
				}...)
				return c
			},
		},
		"container construction with extra pvcs": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				ProxySQL: &api.ProxySQLSpec{
					PodSpec: api.PodSpec{
						Image:             "test-image",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						EnvVarsSecretName: "test-secret",
						ExtraPVCs: []api.ExtraPVC{
							{
								Name:      "extra-data-volume",
								ClaimName: "extra-storage-0",
								MountPath: "/var/lib/proxysql-extra",
							},
							{
								Name:      "backup-volume",
								ClaimName: "backup-storage-0",
								MountPath: "/backups",
								SubPath:   "proxysql",
							},
						},
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedProxySQLContainer()
				// extra PVC mounts are appended before the
				// mysql-users-secret-file mount in AppContainer
				last := c.VolumeMounts[len(c.VolumeMounts)-1]
				c.VolumeMounts = append(c.VolumeMounts[:len(c.VolumeMounts)-1],
					corev1.VolumeMount{
						Name:      "extra-data-volume",
						MountPath: "/var/lib/proxysql-extra",
					},
					corev1.VolumeMount{
						Name:      "backup-volume",
						MountPath: "/backups",
						SubPath:   "proxysql",
					},
					last,
				)
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
			{Name: naming.BinVolumeName, MountPath: naming.BinVolumeMountPath},
			{Name: naming.MySQLUsersSecretVolumeName, MountPath: naming.MySQLUsersSecretMountPath},
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
			{Name: "PERCONA_SCHEDULER_CFG", Value: "/tmp/scheduler-config.toml"},
			{Name: "PXC_READ_ONLY", Value: "false"},
			{Name: "LIVENESS_CHECK_TIMEOUT", Value: "0"},
			{Name: "READINESS_CHECK_TIMEOUT", Value: "0"},
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
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/opt/percona/proxysql_liveness_check.sh"},
				},
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/opt/percona/proxysql_readiness_check.sh"},
				},
			},
		},
	}
}

func TestSidecarContainers_ProxySQL(t *testing.T) {
	secretName := "monitor-secret"

	tests := map[string]struct {
		spec               api.PerconaXtraDBClusterSpec
		expectedContainers func() []corev1.Container
	}{
		"success - default container construction": {
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
					PodSpec: &api.PodSpec{
						Configuration: "config",
					},
				},
			},
			expectedContainers: func() []corev1.Container {
				return defaultExpectedProxySQLSidecarContainers()
			},
		},
		"scheduler enabled - only pxc-monit container": {
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
					PodSpec: &api.PodSpec{
						Configuration: "config",
					},
				},
			},
			expectedContainers: func() []corev1.Container {
				c := defaultExpectedProxySQLSidecarContainers()
				pxcMonit := c[0]
				pxcMonit.Env = append(pxcMonit.Env[:5], []corev1.EnvVar{
					{Name: "SCHEDULER_CHECKTIMEOUT", Value: "2000"},
					{Name: "SCHEDULER_WRITERALSOREADER", Value: "1"},
					{Name: "SCHEDULER_RETRYUP", Value: "1"},
					{Name: "SCHEDULER_RETRYDOWN", Value: "3"},
					{Name: "SCHEDULER_PINGTIMEOUT", Value: "1000"},
					{Name: "SCHEDULER_NODECHECKINTERVAL", Value: "2000"},
					{Name: "SCHEDULER_MAXCONNECTIONS", Value: "1000"},
					{Name: "PERCONA_SCHEDULER_CFG", Value: "/tmp/scheduler-config.toml"},
					{Name: "SCHEDULER_ENABLED", Value: "true"},
					{Name: "PXC_READ_ONLY", Value: "false"},
				}...)
				return []corev1.Container{pxcMonit}
			},
		},
		"scheduler enabled - read only cluster": {
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
					PodSpec: &api.PodSpec{
						Configuration: "config",
					},
					ReplicationChannels: []api.ReplicationChannel{
						{
							Name:     "replica-channel",
							IsSource: false,
						},
					},
				},
			},
			expectedContainers: func() []corev1.Container {
				c := defaultExpectedProxySQLSidecarContainers()
				pxcMonit := c[0]
				pxcMonit.Env = append(pxcMonit.Env[:5], []corev1.EnvVar{
					{Name: "SCHEDULER_CHECKTIMEOUT", Value: "2000"},
					{Name: "SCHEDULER_WRITERALSOREADER", Value: "1"},
					{Name: "SCHEDULER_RETRYUP", Value: "1"},
					{Name: "SCHEDULER_RETRYDOWN", Value: "3"},
					{Name: "SCHEDULER_PINGTIMEOUT", Value: "1000"},
					{Name: "SCHEDULER_NODECHECKINTERVAL", Value: "2000"},
					{Name: "SCHEDULER_MAXCONNECTIONS", Value: "1000"},
					{Name: "PERCONA_SCHEDULER_CFG", Value: "/tmp/scheduler-config.toml"},
					{Name: "SCHEDULER_ENABLED", Value: "true"},
					{Name: "PXC_READ_ONLY", Value: "true"},
				}...)
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
				Spec: tt.spec,
			}

			client := test.BuildFakeClient()
			proxySQL := &Proxy{cr: cr}

			containers, err := proxySQL.SidecarContainers(t.Context(), client, &tt.spec.ProxySQL.PodSpec, secretName, cr)
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
				{Name: "SCHEDULER_CHECKTIMEOUT", Value: "0"},
				{Name: "SCHEDULER_WRITERALSOREADER", Value: "0"},
				{Name: "SCHEDULER_RETRYUP", Value: "0"},
				{Name: "SCHEDULER_RETRYDOWN", Value: "0"},
				{Name: "SCHEDULER_PINGTIMEOUT", Value: "0"},
				{Name: "SCHEDULER_NODECHECKINTERVAL", Value: "0"},
				{Name: "SCHEDULER_MAXCONNECTIONS", Value: "0"},
				{Name: "PERCONA_SCHEDULER_CFG", Value: "/tmp/scheduler-config.toml"},
				{Name: "PXC_READ_ONLY", Value: "false"},
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

func TestVolumes_ProxySQL(t *testing.T) {
	noCustomVolume := func(ctx context.Context, nsName, cvName, cmName string, useDefaultVolume bool) (corev1.Volume, error) {
		return corev1.Volume{}, api.NoCustomVolumeErr
	}

	tests := map[string]struct {
		crVersion              string
		expectMysqlUsersSecret bool
	}{
		"cr 1.18 doesn't mount mysql-users-secret-file": {
			crVersion:              "1.18.0",
			expectMysqlUsersSecret: false,
		},
		"cr 1.21.0 mounts mysql-users-secret-file": {
			crVersion:              "1.21.0",
			expectMysqlUsersSecret: true,
		},
		"latest cr mounts mysql-users-secret-file": {
			crVersion:              version.Version(),
			expectMysqlUsersSecret: true,
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
						PodSpec: api.PodSpec{},
					},
				},
			}
			proxySQL := &Proxy{cr: cr}

			vol, err := proxySQL.Volumes(t.Context(), &cr.Spec.ProxySQL.PodSpec, cr, noCustomVolume)
			assert.NoError(t, err)

			var found *corev1.Volume
			for i := range vol.Volumes {
				if vol.Volumes[i].Name == naming.MySQLUsersSecretVolumeName {
					found = &vol.Volumes[i]
					break
				}
			}

			if !tt.expectMysqlUsersSecret {
				assert.Nil(t, found)
				return
			}

			if assert.NotNil(t, found) {
				assert.Equal(t, naming.MySQLUsersSecretVolumeName, found.Name)
				assert.NotNil(t, found.Secret)
				assert.Equal(t, "internal-test-cluster", found.Secret.SecretName)
				assert.False(t, *found.Secret.Optional)
			}
		})
	}
}
