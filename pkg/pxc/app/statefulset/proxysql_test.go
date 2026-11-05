package statefulset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
				c.VolumeMounts = append(c.VolumeMounts,
					corev1.VolumeMount{
						Name:      "extra-data-volume",
						MountPath: "/var/lib/proxysql-extra",
					},
					corev1.VolumeMount{
						Name:      "backup-volume",
						MountPath: "/backups",
						SubPath:   "proxysql",
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

func TestPMMContainer_ProxySQL(t *testing.T) {
	const (
		namespace            = "test-ns"
		proxySQLEnvSecret    = "proxysql-env-vars"
		pxcEnvSecret         = "pxc-env-vars"
		pmmSecretName        = "pmm-secret"
		proxySQLParams       = "--custom-proxysql-param"
		nodeNameWithPrefix   = "$(PMM_PREFIX)$(POD_NAMESPACE)-$(POD_NAME)"
		nodeNameNoPrefix     = "$(POD_NAMESPACE)-$(POD_NAME)"
		pmm2NodeNameWithPref = "$(PMM_PREFIX)$(POD_NAMESPASE)-$(POD_NAME)"
	)

	newCR := func(crVersion string, pmm *api.PMMSpec) *api.PerconaXtraDBCluster {
		return &api.PerconaXtraDBCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: namespace},
			Spec: api.PerconaXtraDBClusterSpec{
				CRVersion: crVersion,
				PMM:       pmm,
				ProxySQL: &api.ProxySQLSpec{
					PodSpec: api.PodSpec{
						Image:             "test-image",
						EnvVarsSecretName: proxySQLEnvSecret,
					},
				},
				PXC: &api.PXCSpec{
					PodSpec: &api.PodSpec{EnvVarsSecretName: pxcEnvSecret},
				},
			},
		}
	}

	enabledPMM := func() *api.PMMSpec {
		return &api.PMMSpec{
			Enabled:        true,
			Image:          "pmm-image",
			ServerHost:     "pmm-server",
			ServerUser:     users.PMMServer,
			ProxysqlParams: proxySQLParams,
		}
	}

	newSecret := func(name string, data map[string][]byte) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Data:       data,
		}
	}

	pmm3Secret := newSecret(pmmSecretName, map[string][]byte{users.PMMServerToken: []byte("token")})
	pmm2Secret := newSecret(pmmSecretName, map[string][]byte{users.PMMServerKey: []byte("key")})
	emptySecret := newSecret(pmmSecretName, nil)

	prefixedEnvSecrets := func(t *testing.T) []client.Object {
		t.Helper()
		return []client.Object{
			newSecret(proxySQLEnvSecret, map[string][]byte{"PMM_PREFIX": []byte("pxc-prefix-")}),
			newSecret(pxcEnvSecret, nil),
		}
	}
	prefixOnPXCEnvSecrets := func(t *testing.T) []client.Object {
		t.Helper()
		return []client.Object{
			newSecret(proxySQLEnvSecret, nil),
			newSecret(pxcEnvSecret, map[string][]byte{"PMM_PREFIX": []byte("pxc-prefix-")}),
		}
	}

	tests := map[string]struct {
		cr          *api.PerconaXtraDBCluster
		pmmSecret   *corev1.Secret
		setup       func(t *testing.T) []client.Object
		wantNil     bool
		wantErrMsg  string
		wantEnv     map[string]string
		wantNoEnv   []string
		wantEnvFrom []corev1.EnvFromSource
	}{
		"pmm is disabled": {
			cr:        newCR(version.Version(), &api.PMMSpec{Enabled: false}),
			pmmSecret: pmm3Secret,
			setup:     prefixedEnvSecrets,
			wantNil:   true,
		},
		"pmm is not configured": {
			cr:        newCR(version.Version(), nil),
			pmmSecret: pmm3Secret,
			setup:     prefixedEnvSecrets,
			wantNil:   true,
		},
		"pmm3 monitors proxysql as the proxystats user": {
			cr:        newCR(version.Version(), enabledPMM()),
			pmmSecret: pmm3Secret,
			setup:     prefixedEnvSecrets,
			wantEnv: map[string]string{
				"DB_TYPE":                   "proxysql",
				"DB_USER":                   users.ProxyStats,
				"DB_HOST":                   "localhost",
				"DB_PORT":                   "6032",
				"DB_CLUSTER":                naming.ComponentPXC,
				"PMM_ADMIN_CUSTOM_PARAMS":   proxySQLParams,
				"PMM_AGENT_SETUP_NODE_NAME": nodeNameWithPrefix,
			},
			wantEnvFrom: expectedProxySQLEnvFrom(proxySQLEnvSecret),
		},
		"pmm3 ignores PMM_PREFIX from the pxc env vars secret": {
			cr:        newCR(version.Version(), enabledPMM()),
			pmmSecret: pmm3Secret,
			setup:     prefixOnPXCEnvSecrets,
			wantEnv: map[string]string{
				"PMM_AGENT_SETUP_NODE_NAME": nodeNameNoPrefix,
			},
			wantEnvFrom: expectedProxySQLEnvFrom(proxySQLEnvSecret),
		},
		"pmm2 monitors proxysql as the proxystats user": {
			cr:        newCR(version.Version(), enabledPMM()),
			pmmSecret: pmm2Secret,
			setup:     prefixedEnvSecrets,
			wantEnv: map[string]string{
				"DB_TYPE":                   "proxysql",
				"DB_USER":                   users.ProxyStats,
				"DB_HOST":                   "localhost",
				"DB_PORT":                   "6032",
				"DB_CLUSTER":                naming.ComponentPXC,
				"MONITOR_USER":              users.Monitor,
				"PMM_ADMIN_CUSTOM_PARAMS":   proxySQLParams,
				"PMM_AGENT_SETUP_NODE_NAME": pmm2NodeNameWithPref,
			},
			wantEnvFrom: expectedProxySQLEnvFrom(proxySQLEnvSecret),
		},
		"pmm2 without a server key or password": {
			cr:         newCR(version.Version(), enabledPMM()),
			pmmSecret:  emptySecret,
			setup:      prefixedEnvSecrets,
			wantErrMsg: "can't enable PMM2: either pmmserverkey key doesn't exist in the secrets, or secrets and internal secrets are out of sync",
		},
		"pmm3 on cr 1.20.0 keeps monitoring proxysql as the monitor user": {
			cr:        newCR("1.20.0", enabledPMM()),
			pmmSecret: pmm3Secret,
			setup:     prefixedEnvSecrets,
			wantEnv: map[string]string{
				"DB_USER": users.Monitor,
			},
			wantEnvFrom: expectedProxySQLEnvFrom(proxySQLEnvSecret),
		},
		"pmm2 on cr 1.20.0 keeps monitoring proxysql as the monitor user": {
			cr:        newCR("1.20.0", enabledPMM()),
			pmmSecret: pmm2Secret,
			setup:     prefixedEnvSecrets,
			wantEnv: map[string]string{
				"DB_USER": users.Monitor,
			},
			wantEnvFrom: expectedProxySQLEnvFrom(proxySQLEnvSecret),
		},
		"cr before 1.2.0 configures the exporter through DB_ARGS": {
			cr:        newCR("1.1.0", enabledPMM()),
			pmmSecret: pmm2Secret,
			setup:     prefixedEnvSecrets,
			wantEnv: map[string]string{
				"DB_ARGS": "--dsn $(MONITOR_USER):$(MONITOR_PASSWORD)@tcp(localhost:6032)/",
			},
			wantNoEnv: []string{"DB_USER", "DB_HOST", "DB_PORT"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			objects := append(tt.setup(t), tt.pmmSecret)
			cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()

			proxy := &Proxy{cr: tt.cr}
			c, err := proxy.PMMContainer(t.Context(), cl, tt.cr.Spec.PMM, tt.pmmSecret, tt.cr)

			if tt.wantErrMsg != "" {
				require.EqualError(t, err, tt.wantErrMsg)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, c)
				return
			}
			require.NotNil(t, c)

			assert.Equal(t, tt.wantEnvFrom, c.EnvFrom)

			env := make(map[string]corev1.EnvVar, len(c.Env))
			for _, e := range c.Env {
				env[e.Name] = e
			}

			for envName, want := range tt.wantEnv {
				assert.Equal(t, want, env[envName].Value, envName)
			}
			for _, envName := range tt.wantNoEnv {
				assert.NotContains(t, env, envName)
			}
		})
	}
}

func expectedProxySQLEnvFrom(secretName string) []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Optional:             pointerToTrue(),
			},
		},
	}
}
