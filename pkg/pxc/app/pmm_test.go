package app

import (
	"testing"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestPMM3Client(t *testing.T) {
	tests := map[string]struct {
		secret         *corev1.Secret
		envVarsSecret  *corev1.Secret
		livenessProbe  *corev1.Probe
		readinessProbe *corev1.Probe
		expectedErrMsg string
	}{
		"secret is nil": {
			envVarsSecret:  &corev1.Secret{},
			expectedErrMsg: "secret is nil",
		},
		"envVarsSecret is nil": {
			secret: &corev1.Secret{
				Data: map[string][]byte{
					"pmmservertoken": []byte("some-token"),
				},
			},
			expectedErrMsg: "envVarsSecret is nil",
		},
		"success": {
			secret: &corev1.Secret{
				Data: map[string][]byte{
					"pmmservertoken": []byte("some-token"),
					"monitor":        []byte("monitor-password"),
				},
			},
			envVarsSecret: &corev1.Secret{
				Data: map[string][]byte{
					"PMM_PREFIX": []byte("prefix-"),
				},
			},
		},
		"success with custom probes": {
			secret: &corev1.Secret{
				Data: map[string][]byte{
					"pmmserver": []byte("some-token"),
					"monitor":   []byte("monitor-password"),
				},
			},
			envVarsSecret: &corev1.Secret{
				Data: map[string][]byte{
					"PMM_PREFIX": []byte("prefix-"),
				},
			},
			livenessProbe: &corev1.Probe{
				InitialDelaySeconds: 12,
				TimeoutSeconds:      11,
				PeriodSeconds:       10,
			},
			readinessProbe: &corev1.Probe{
				InitialDelaySeconds: 14,
				TimeoutSeconds:      15,
				PeriodSeconds:       16,
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
					PMM: &api.PMMSpec{
						ServerHost:               "pmm-server",
						Image:                    "percona/pmm-3",
						ImagePullPolicy:          corev1.PullIfNotPresent,
						ContainerSecurityContext: &corev1.SecurityContext{},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("50M"),
								corev1.ResourceCPU:    resource.MustParse("50m"),
							},
						},
						LivenessProbes:    tt.livenessProbe,
						ReadinessProbes:   tt.readinessProbe,
						CustomClusterName: "foo-cluster",
					},
				},
			}

			container, err := PMM3Client(cr, tt.secret, tt.envVarsSecret)

			if tt.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				assert.Empty(t, container)
				return
			}

			assert.NoError(t, err)
			assert.NotEmpty(t, container)

			assert.Equal(t, "pmm-client", container.Name)
			assert.Equal(t, cr.Spec.PMM.Image, container.Image)
			assert.Equal(t, cr.Spec.PMM.ImagePullPolicy, container.ImagePullPolicy)
			assert.Equal(t, cr.Spec.PMM.ContainerSecurityContext, container.SecurityContext)
			assert.Equal(t, cr.Spec.PMM.Resources, container.Resources)

			expectedPorts := []int32{7777, 30100, 30101, 30102, 30103, 30104, 30105}
			var containerPorts []int32
			for _, p := range container.Ports {
				containerPorts = append(containerPorts, p.ContainerPort)
			}
			assert.ElementsMatch(t, expectedPorts, containerPorts)

			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}

			assert.Equal(t, "pmm-server", envMap["PMM_AGENT_SERVER_ADDRESS"])
			assert.Equal(t, "service_token", envMap["PMM_AGENT_SERVER_USERNAME"])
			assert.Equal(t, "7777", envMap["PMM_AGENT_LISTEN_PORT"])
			assert.Equal(t, "30100", envMap["PMM_AGENT_PORTS_MIN"])
			assert.Equal(t, "30105", envMap["PMM_AGENT_PORTS_MAX"])
			assert.Equal(t, "1", envMap["PMM_AGENT_SERVER_INSECURE_TLS"])
			assert.Equal(t, "0.0.0.0", envMap["PMM_AGENT_LISTEN_ADDRESS"])
			assert.Equal(t, "push", envMap["PMM_AGENT_SETUP_METRICS_MODE"])
			assert.Equal(t, "1", envMap["PMM_AGENT_SETUP"])
			assert.Equal(t, "1", envMap["PMM_AGENT_SETUP_FORCE"])
			assert.Equal(t, "container", envMap["PMM_AGENT_SETUP_NODE_TYPE"])
			assert.Equal(t, "true", envMap["PMM_AGENT_SIDECAR"])
			assert.Equal(t, "5", envMap["PMM_AGENT_SIDECAR_SLEEP"])
			assert.Equal(t, "/tmp/pmm", envMap["PMM_AGENT_PATHS_TEMPDIR"])
			assert.Equal(t, "/var/lib/mysql/pmm-prerun.sh", envMap["PMM_AGENT_PRERUN_SCRIPT"])
			assert.Equal(t, "foo-cluster", envMap["CLUSTER_NAME"])
			assert.Equal(t, "$(PMM_PREFIX)$(POD_NAMESPACE)-$(POD_NAME)", envMap["PMM_AGENT_SETUP_NODE_NAME"])

			assert.NotNil(t, container.Lifecycle)
			assert.NotNil(t, container.Lifecycle.PreStop)
			assert.NotNil(t, container.Lifecycle.PreStop.Exec)
			assert.Contains(t, container.Lifecycle.PreStop.Exec.Command, "pmm-admin unregister --force")

			assert.Len(t, container.VolumeMounts, 1)
			assert.Equal(t, "/var/lib/mysql", container.VolumeMounts[0].MountPath)

			if tt.livenessProbe != nil {
				assert.NotNil(t, container.LivenessProbe)
				assert.Equal(t, tt.livenessProbe.InitialDelaySeconds, container.LivenessProbe.InitialDelaySeconds)
				assert.Equal(t, tt.livenessProbe.TimeoutSeconds, container.LivenessProbe.TimeoutSeconds)
				assert.Equal(t, tt.livenessProbe.PeriodSeconds, container.LivenessProbe.PeriodSeconds)
				assert.NotNil(t, container.LivenessProbe.HTTPGet)
				assert.Equal(t, "/local/Status", container.LivenessProbe.HTTPGet.Path)
				assert.Equal(t, intstr.FromInt32(7777), container.LivenessProbe.HTTPGet.Port)
			} else {
				assert.NotNil(t, container.LivenessProbe)
				assert.Equal(t, int32(60), container.LivenessProbe.InitialDelaySeconds)
				assert.Equal(t, int32(5), container.LivenessProbe.TimeoutSeconds)
				assert.Equal(t, int32(10), container.LivenessProbe.PeriodSeconds)
				assert.NotNil(t, container.LivenessProbe.HTTPGet)
				assert.Equal(t, intstr.FromInt32(7777), container.LivenessProbe.HTTPGet.Port)
				assert.Equal(t, "/local/Status", container.LivenessProbe.HTTPGet.Path)
			}

			if tt.readinessProbe != nil {
				assert.NotNil(t, container.LivenessProbe)
				assert.Equal(t, tt.readinessProbe.InitialDelaySeconds, container.ReadinessProbe.InitialDelaySeconds)
				assert.Equal(t, tt.readinessProbe.TimeoutSeconds, container.ReadinessProbe.TimeoutSeconds)
				assert.Equal(t, tt.readinessProbe.PeriodSeconds, container.ReadinessProbe.PeriodSeconds)
				assert.NotNil(t, container.ReadinessProbe.HTTPGet)
				assert.Equal(t, "/local/Status", container.ReadinessProbe.HTTPGet.Path)
				assert.Equal(t, intstr.FromInt32(7777), container.ReadinessProbe.HTTPGet.Port)
			}
		})
	}
}
