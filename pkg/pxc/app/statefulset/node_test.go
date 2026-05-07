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
					Value: "/usr/lib64/libjemalloc.so.2",
				})
				return c
			},
		},
		"container construction with jemalloc and PXC 8.4 image": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					MySQLAllocator: "jemalloc",
					PodSpec: &api.PodSpec{
						Image:             "percona/percona-xtradb-cluster:8.4.32",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						LivenessProbes:    corev1.Probe{TimeoutSeconds: 5},
						ReadinessProbes:   corev1.Probe{TimeoutSeconds: 15},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				c.Image = "percona/percona-xtradb-cluster:8.4.32"
				c.Env = append(c.Env, corev1.EnvVar{Name: "LD_PRELOAD", Value: "/usr/lib64/libjemalloc.so.2"})
				return c
			},
		},
		"container construction with jemalloc and PXC 8.0 image": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					MySQLAllocator: "jemalloc",
					PodSpec: &api.PodSpec{
						Image:             "percona/percona-xtradb-cluster:8.0.35",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						LivenessProbes:    corev1.Probe{TimeoutSeconds: 5},
						ReadinessProbes:   corev1.Probe{TimeoutSeconds: 15},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				c.Image = "percona/percona-xtradb-cluster:8.0.35"
				c.Env = append(c.Env, corev1.EnvVar{Name: "LD_PRELOAD", Value: "/usr/lib64/libjemalloc.so.1"})
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
		"container construction with extra pvcs": {
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
						ExtraPVCs: []api.ExtraPVC{
							{
								Name:      "extra-data-volume",
								ClaimName: "extra-storage-0",
								MountPath: "/var/lib/mysql-extra",
							},
							{
								Name:      "backup-volume",
								ClaimName: "backup-storage-0",
								MountPath: "/backups",
								SubPath:   "mysql",
							},
						},
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				c.VolumeMounts = append(c.VolumeMounts,
					corev1.VolumeMount{
						Name:      "extra-data-volume",
						MountPath: "/var/lib/mysql-extra",
					},
					corev1.VolumeMount{
						Name:      "backup-volume",
						MountPath: "/backups",
						SubPath:   "mysql",
					},
				)
				return c
			},
		},
		"container construction with sst retry count": {
			spec: api.PerconaXtraDBClusterSpec{
				CRVersion: version.Version(),
				PXC: &api.PXCSpec{
					SSTRetryCount: func() *int32 {
						v := int32(3)
						return &v
					}(),
					PodSpec: &api.PodSpec{
						Image:             "test-image",
						ImagePullPolicy:   corev1.PullIfNotPresent,
						LivenessProbes:    corev1.Probe{TimeoutSeconds: 5},
						ReadinessProbes:   corev1.Probe{TimeoutSeconds: 15},
						EnvVarsSecretName: "test-secret",
					},
				},
			},
			expectedContainer: func() corev1.Container {
				c := defaultExpectedContainer()
				idx := len(c.Env) - 3
				c.Env = append(c.Env[:idx], append([]corev1.EnvVar{{
					Name:  "PXC_SST_RETRY_COUNT",
					Value: "3",
				}}, c.Env[idx:]...)...)
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

func TestLogCollectorContainer(t *testing.T) {
	logPsecrets := "log-p-secret"
	logRsecrets := "log-r-secret"

	baseLogProcEnvs := func(includeNamespace bool) []corev1.EnvVar {
		envs := []corev1.EnvVar{
			{Name: "LOG_DATA_DIR", Value: "/var/lib/mysql"},
			{
				Name: "POD_NAMESPASE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
				},
			},
		}
		if includeNamespace {
			envs = append(envs, corev1.EnvVar{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
				},
			})
		}
		return envs
	}

	baseLogRotEnvs := func() []corev1.EnvVar {
		return []corev1.EnvVar{
			{Name: "SERVICE_TYPE", Value: "mysql"},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(logRsecrets, users.Monitor),
				},
			},
		}
	}

	fvar := true
	baseEnvFrom := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: logPsecrets},
				Optional:             &fvar,
			},
		},
	}

	datadirMount := corev1.VolumeMount{Name: app.DataVolumeName, MountPath: "/var/lib/mysql"}
	binMount := corev1.VolumeMount{Name: app.BinVolumeName, MountPath: app.BinVolumeMountPath}

	tests := map[string]struct {
		crVersion      string
		logCollector   *api.LogCollectorSpec
		expectedResult func() []corev1.Container
	}{
		"pre-1.20 basic": {
			crVersion: "1.19.0",
			logCollector: &api.LogCollectorSpec{
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(false),
						EnvFrom:         baseEnvFrom,
						VolumeMounts:    []corev1.VolumeMount{datadirMount},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogRotEnvs(),
						Args:            []string{"logrotate"},
						VolumeMounts:    []corev1.VolumeMount{datadirMount},
					},
				}
			},
		},
		"1.20+ basic": {
			crVersion: version.Version(),
			logCollector: &api.LogCollectorSpec{
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(true),
						EnvFrom:         baseEnvFrom,
						Command:         []string{"/opt/percona/logcollector/entrypoint.sh"},
						Args:            []string{"fluent-bit"},
						VolumeMounts:    []corev1.VolumeMount{datadirMount, binMount},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env: append(baseLogRotEnvs(), corev1.EnvVar{
							Name: "LOGROTATE_STATUS_FILE", Value: "/var/lib/mysql/logrotate.status",
						}),
						Args:         []string{"logrotate"},
						Command:      []string{"/opt/percona/logcollector/entrypoint.sh"},
						VolumeMounts: []corev1.VolumeMount{datadirMount, binMount},
					},
				}
			},
		},
		"pre-1.20 with configuration": {
			crVersion: "1.19.0",
			logCollector: &api.LogCollectorSpec{
				Configuration:   "some-fluentbit-config",
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullIfNotPresent,
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env:             baseLogProcEnvs(false),
						EnvFrom:         baseEnvFrom,
						VolumeMounts: []corev1.VolumeMount{
							datadirMount,
							{Name: "logcollector-config", MountPath: "/etc/fluentbit/custom"},
						},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env:             baseLogRotEnvs(),
						Args:            []string{"logrotate"},
						VolumeMounts:    []corev1.VolumeMount{datadirMount},
					},
				}
			},
		},
		"1.20+ with configuration": {
			crVersion: version.Version(),
			logCollector: &api.LogCollectorSpec{
				Configuration:   "some-fluentbit-config",
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(true),
						EnvFrom:         baseEnvFrom,
						Command:         []string{"/opt/percona/logcollector/entrypoint.sh"},
						Args:            []string{"fluent-bit"},
						VolumeMounts: []corev1.VolumeMount{
							datadirMount,
							{Name: "logcollector-config", MountPath: "/opt/percona/logcollector/fluentbit/custom"},
							binMount,
						},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env: append(baseLogRotEnvs(), corev1.EnvVar{
							Name: "LOGROTATE_STATUS_FILE", Value: "/var/lib/mysql/logrotate.status",
						}),
						Args:         []string{"logrotate"},
						Command:      []string{"/opt/percona/logcollector/entrypoint.sh"},
						VolumeMounts: []corev1.VolumeMount{datadirMount, binMount},
					},
				}
			},
		},
		"1.20+ with hookscript": {
			crVersion: version.Version(),
			logCollector: &api.LogCollectorSpec{
				HookScript:      "some-hook-script",
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(true),
						EnvFrom:         baseEnvFrom,
						Command:         []string{"/opt/percona/logcollector/entrypoint.sh"},
						Args:            []string{"fluent-bit"},
						VolumeMounts: []corev1.VolumeMount{
							datadirMount,
							{Name: "hookscript", MountPath: "/opt/percona/hookscript"},
							binMount,
						},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env: append(baseLogRotEnvs(), corev1.EnvVar{
							Name: "LOGROTATE_STATUS_FILE", Value: "/var/lib/mysql/logrotate.status",
						}),
						Args:         []string{"logrotate"},
						Command:      []string{"/opt/percona/logcollector/entrypoint.sh"},
						VolumeMounts: []corev1.VolumeMount{datadirMount, binMount},
					},
				}
			},
		},
		"1.20+ with logrotate configuration": {
			crVersion: version.Version(),
			logCollector: &api.LogCollectorSpec{
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
				LogRotate: &api.LogRotateSpec{
					Configuration: "custom-logrotate-config",
				},
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(true),
						EnvFrom:         baseEnvFrom,
						Command:         []string{"/opt/percona/logcollector/entrypoint.sh"},
						Args:            []string{"fluent-bit"},
						VolumeMounts:    []corev1.VolumeMount{datadirMount, binMount},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env: append(baseLogRotEnvs(), corev1.EnvVar{
							Name: "LOGROTATE_STATUS_FILE", Value: "/var/lib/mysql/logrotate.status",
						}),
						Args:    []string{"logrotate"},
						Command: []string{"/opt/percona/logcollector/entrypoint.sh"},
						VolumeMounts: []corev1.VolumeMount{
							datadirMount,
							{Name: LogRotateConfigVolumeName, MountPath: LogRotateConfigDir},
							binMount,
						},
					},
				}
			},
		},
		"1.20+ with logrotate extraconfig": {
			crVersion: version.Version(),
			logCollector: &api.LogCollectorSpec{
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
				LogRotate: &api.LogRotateSpec{
					ExtraConfig: corev1.LocalObjectReference{Name: "extra-logrotate-cm"},
				},
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(true),
						EnvFrom:         baseEnvFrom,
						Command:         []string{"/opt/percona/logcollector/entrypoint.sh"},
						Args:            []string{"fluent-bit"},
						VolumeMounts:    []corev1.VolumeMount{datadirMount, binMount},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env: append(baseLogRotEnvs(), corev1.EnvVar{
							Name: "LOGROTATE_STATUS_FILE", Value: "/var/lib/mysql/logrotate.status",
						}),
						Args:    []string{"logrotate"},
						Command: []string{"/opt/percona/logcollector/entrypoint.sh"},
						VolumeMounts: []corev1.VolumeMount{
							datadirMount,
							{Name: LogRotateConfigVolumeName, MountPath: LogRotateConfigDir},
							binMount,
						},
					},
				}
			},
		},
		"pre-1.20 with logrotate schedule": {
			crVersion: "1.19.0",
			logCollector: &api.LogCollectorSpec{
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
				LogRotate: &api.LogRotateSpec{
					Schedule: "*/5 * * * *",
				},
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(false),
						EnvFrom:         baseEnvFrom,
						VolumeMounts:    []corev1.VolumeMount{datadirMount},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env: append(baseLogRotEnvs(), corev1.EnvVar{
							Name: "LOGROTATE_SCHEDULE", Value: "*/5 * * * *",
						}),
						Args:         []string{"logrotate"},
						VolumeMounts: []corev1.VolumeMount{datadirMount},
					},
				}
			},
		},
		"1.20+ with all options": {
			crVersion: version.Version(),
			logCollector: &api.LogCollectorSpec{
				Configuration:   "custom-fluentbit-config",
				HookScript:      "custom-hook-script",
				Image:           "test-log-image",
				ImagePullPolicy: corev1.PullAlways,
				LogRotate: &api.LogRotateSpec{
					Configuration: "custom-logrotate-config",
					Schedule:      "0 */6 * * *",
				},
			},
			expectedResult: func() []corev1.Container {
				return []corev1.Container{
					{
						Name:            "logs",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env:             baseLogProcEnvs(true),
						EnvFrom:         baseEnvFrom,
						Command:         []string{"/opt/percona/logcollector/entrypoint.sh"},
						Args:            []string{"fluent-bit"},
						VolumeMounts: []corev1.VolumeMount{
							datadirMount,
							{Name: "logcollector-config", MountPath: "/opt/percona/logcollector/fluentbit/custom"},
							{Name: "hookscript", MountPath: "/opt/percona/hookscript"},
							binMount,
						},
					},
					{
						Name:            "logrotate",
						Image:           "test-log-image",
						ImagePullPolicy: corev1.PullAlways,
						Env: append(baseLogRotEnvs(),
							corev1.EnvVar{Name: "LOGROTATE_SCHEDULE", Value: "0 */6 * * *"},
							corev1.EnvVar{Name: "LOGROTATE_STATUS_FILE", Value: "/var/lib/mysql/logrotate.status"},
						),
						Args:    []string{"logrotate"},
						Command: []string{"/opt/percona/logcollector/entrypoint.sh"},
						VolumeMounts: []corev1.VolumeMount{
							datadirMount,
							{Name: LogRotateConfigVolumeName, MountPath: LogRotateConfigDir},
							binMount,
						},
					},
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: api.PerconaXtraDBClusterSpec{
					CRVersion:    tt.crVersion,
					LogCollector: tt.logCollector,
				},
			}

			pxcNode := Node{cr: cr}

			containers, err := pxcNode.LogCollectorContainer(cr, logPsecrets, logRsecrets)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult(), containers)
		})
	}
}

func TestJemallocPathForPXCImage(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"", "/usr/lib64/libjemalloc.so.2"},
		{"percona/percona-xtradb-cluster", "/usr/lib64/libjemalloc.so.2"},
		{"percona/percona-xtradb-cluster:8.0.35", "/usr/lib64/libjemalloc.so.1"},
		{"percona/percona-xtradb-cluster:8.0", "/usr/lib64/libjemalloc.so.1"},
		{"percona/percona-xtradb-cluster:8.4.32", "/usr/lib64/libjemalloc.so.2"},
		{"percona/percona-xtradb-cluster:8.4", "/usr/lib64/libjemalloc.so.2"},
		{"percona/percona-xtradb-cluster:8.4.32-31", "/usr/lib64/libjemalloc.so.2"},
		{"registry.example.com/pxc:9.0.0", "/usr/lib64/libjemalloc.so.2"},
		{"perconalab/percona-xtradb-cluster-operator:main-pxc8.4", "/usr/lib64/libjemalloc.so.2"},
		{"perconalab/percona-xtradb-cluster-operator:main-pxc8.0", "/usr/lib64/libjemalloc.so.1"},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := jemallocPathForPXCImage(tt.image)
			assert.Equal(t, tt.want, got)
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
