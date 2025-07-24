package app

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

// PMMClient constructs a PMM2 container. This function is going to be deprecated soon.
func PMMClient(cr *api.PerconaXtraDBCluster, spec *api.PMMSpec, secret *corev1.Secret, envVarsSecret *corev1.Secret) corev1.Container {
	ports := []corev1.ContainerPort{{ContainerPort: 7777}}

	for i := 30100; i <= 30105; i++ {
		ports = append(ports, corev1.ContainerPort{ContainerPort: int32(i)})
	}

	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "PMM_SERVER",
			Value: spec.ServerHost,
		},
	}

	clientEnvs := []corev1.EnvVar{
		{
			Name:  "CLIENT_PORT_LISTEN",
			Value: "7777",
		},
		{
			Name:  "CLIENT_PORT_MIN",
			Value: "30100",
		},
		{
			Name:  "CLIENT_PORT_MAX",
			Value: "30105",
		},
	}

	if spec.ServerUser != "" {
		pmmEnvs = append(pmmEnvs, pmmEnvServerUser(spec.ServerUser, secret.Name, spec.UseAPI(secret))...)
	}
	pmmEnvs = append(pmmEnvs, clientEnvs...)

	pmmAgentEnvs := pmmAgentEnvs(spec.ServerHost, spec.ServerUser, secret.Name, spec.UseAPI(secret))
	if cr.CompareVersionWith("1.14.0") >= 0 {
		val := "$(POD_NAMESPASE)-$(POD_NAME)"
		if len(envVarsSecret.Data["PMM_PREFIX"]) > 0 {
			val = "$(PMM_PREFIX)$(POD_NAMESPASE)-$(POD_NAME)"
		}
		pmmAgentEnvs = append(pmmAgentEnvs, corev1.EnvVar{
			Name:  "PMM_AGENT_SETUP_NODE_NAME",
			Value: val,
		})
	} else {
		pmmAgentEnvs = append(pmmAgentEnvs, corev1.EnvVar{
			Name:  "PMM_AGENT_SETUP_NODE_NAME",
			Value: "$(POD_NAMESPASE)-$(POD_NAME)",
		})
	}

	pmmEnvs = append(pmmEnvs, pmmAgentEnvs...)

	container := corev1.Container{
		Name:            "pmm-client",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Env:             pmmEnvs,
		SecurityContext: spec.ContainerSecurityContext,
		Ports:           ports,
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"-c",
						"pmm-admin unregister --force",
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 60,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(7777),
					Path: "/local/Status",
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      BinVolumeName,
				MountPath: "/var/lib/mysql",
			},
		},
	}

	if cr.CompareVersionWith("1.17.0") >= 0 {
		if spec.LivenessProbes != nil {
			container.LivenessProbe = spec.LivenessProbes

			if reflect.DeepEqual(container.LivenessProbe.ProbeHandler, corev1.ProbeHandler{}) {
				container.LivenessProbe.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{
					Port: intstr.FromInt(7777),
					Path: "/local/Status",
				}
			}
		}
		if spec.ReadinessProbes != nil {
			container.ReadinessProbe = spec.ReadinessProbes

			if reflect.DeepEqual(container.ReadinessProbe.ProbeHandler, corev1.ProbeHandler{}) {
				container.ReadinessProbe.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{
					Port: intstr.FromInt(7777),
					Path: "/local/Status",
				}
			}
		}
	}

	return container
}

func pmmAgentEnvs(pmmServerHost, pmmServerUser, secrets string, useAPI bool) []corev1.EnvVar {
	var pmmServerPassKey string
	if useAPI {
		pmmServerUser = "api_key"
		pmmServerPassKey = users.PMMServerKey
	} else {
		pmmServerPassKey = users.PMMServer
	}
	return []corev1.EnvVar{
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "POD_NAMESPASE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name:  "PMM_AGENT_SERVER_ADDRESS",
			Value: pmmServerHost,
		},
		{
			Name:  "PMM_AGENT_SERVER_USERNAME",
			Value: pmmServerUser,
		},
		{
			Name: "PMM_AGENT_SERVER_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: SecretKeySelector(secrets, pmmServerPassKey),
			},
		},
		{
			Name:  "PMM_AGENT_LISTEN_PORT",
			Value: "7777",
		},
		{
			Name:  "PMM_AGENT_PORTS_MIN",
			Value: "30100",
		},
		{
			Name:  "PMM_AGENT_PORTS_MAX",
			Value: "30105",
		},
		{
			Name:  "PMM_AGENT_CONFIG_FILE",
			Value: "/usr/local/percona/pmm2/config/pmm-agent.yaml",
		},
		{
			Name:  "PMM_AGENT_SERVER_INSECURE_TLS",
			Value: "1",
		},
		{
			Name:  "PMM_AGENT_LISTEN_ADDRESS",
			Value: "0.0.0.0",
		},
		{
			Name:  "PMM_AGENT_SETUP_METRICS_MODE",
			Value: "push",
		},
		{
			Name:  "PMM_AGENT_SETUP",
			Value: "1",
		},
		{
			Name:  "PMM_AGENT_SETUP_FORCE",
			Value: "1",
		},
		{
			Name:  "PMM_AGENT_SETUP_NODE_TYPE",
			Value: "container",
		},
	}
}

func PMMAgentScript(cr *api.PerconaXtraDBCluster, dbType string) []corev1.EnvVar {
	if cr.CompareVersionWith("1.13.0") < 0 {
		pmmServerArgs := " $(PMM_ADMIN_CUSTOM_PARAMS) --skip-connection-check --metrics-mode=push"
		pmmServerArgs += " --username=$(DB_USER) --password=$(DB_PASSWORD) --cluster=$(CLUSTER_NAME)"
		if dbType != "haproxy" {
			pmmServerArgs += " --service-name=$(PMM_AGENT_SETUP_NODE_NAME) --host=$(POD_NAME) --port=$(DB_PORT)"
		}

		if dbType == "mysql" {
			pmmServerArgs += " $(DB_ARGS)"
		}

		if dbType == "haproxy" {
			pmmServerArgs += " $(PMM_AGENT_SETUP_NODE_NAME)"
		}
		return []corev1.EnvVar{
			{
				Name:  "PMM_AGENT_PRERUN_SCRIPT",
				Value: "pmm-admin status --wait=10s;\npmm-admin add $(DB_TYPE)" + pmmServerArgs + ";\npmm-admin annotate --service-name=$(PMM_AGENT_SETUP_NODE_NAME) 'Service restarted'",
			},
		}
	}

	return []corev1.EnvVar{
		{
			Name:  "PMM_AGENT_PRERUN_SCRIPT",
			Value: "/var/lib/mysql/pmm-prerun.sh",
		},
	}
}

func pmmEnvServerUser(user, secrets string, useAPI bool) []corev1.EnvVar {
	var passKey string
	if useAPI {
		user = "api_key"
		passKey = users.PMMServerKey
	} else {
		passKey = users.PMMServer
	}
	return []corev1.EnvVar{
		{
			Name:  "PMM_USER",
			Value: user,
		},
		{
			Name: "PMM_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: SecretKeySelector(secrets, passKey),
			},
		},
	}
}

func PMM3Client(cr *api.PerconaXtraDBCluster, secret *corev1.Secret, envVarsSecret *corev1.Secret) (corev1.Container, error) {
	if secret == nil {
		return corev1.Container{}, fmt.Errorf("secret is nil")
	}
	if envVarsSecret == nil {
		return corev1.Container{}, fmt.Errorf("envVarsSecret is nil")
	}

	pmmSpec := cr.Spec.PMM

	ports := []corev1.ContainerPort{{ContainerPort: 7777}}
	for i := 30100; i <= 30105; i++ {
		ports = append(ports, corev1.ContainerPort{ContainerPort: int32(i)})
	}

	clusterName := cr.Name
	if cr.Spec.PMM.CustomClusterName != "" {
		clusterName = cr.Spec.PMM.CustomClusterName
	}

	envs := []corev1.EnvVar{
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name:  "PMM_AGENT_SERVER_ADDRESS",
			Value: pmmSpec.ServerHost,
		},
		{
			Name:  "PMM_AGENT_SERVER_USERNAME",
			Value: "service_token",
		},
		{
			Name: "PMM_AGENT_SERVER_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: SecretKeySelector(secret.Name, users.PMMServerToken),
			},
		},
		{
			Name:  "PMM_AGENT_LISTEN_PORT",
			Value: "7777",
		},
		{
			Name:  "PMM_AGENT_PORTS_MIN",
			Value: "30100",
		},
		{
			Name:  "PMM_AGENT_PORTS_MAX",
			Value: "30105",
		},
		{
			Name:  "PMM_AGENT_CONFIG_FILE",
			Value: "/usr/local/percona/pmm/config/pmm-agent.yaml",
		},
		{
			Name:  "PMM_AGENT_SERVER_INSECURE_TLS",
			Value: "1",
		},
		{
			Name:  "PMM_AGENT_LISTEN_ADDRESS",
			Value: "0.0.0.0",
		},
		{
			Name:  "PMM_AGENT_SETUP_METRICS_MODE",
			Value: "push",
		},
		{
			Name:  "PMM_AGENT_SETUP",
			Value: "1",
		},
		{
			Name:  "PMM_AGENT_SETUP_FORCE",
			Value: "1",
		},
		{
			Name:  "PMM_AGENT_SETUP_NODE_TYPE",
			Value: "container",
		},
		{
			Name:  "PMM_AGENT_SIDECAR",
			Value: "true",
		},
		{
			Name:  "PMM_AGENT_SIDECAR_SLEEP",
			Value: "5",
		},
		{
			Name:  "PMM_AGENT_PATHS_TEMPDIR",
			Value: "/tmp/pmm",
		},
		{
			Name:  "PMM_AGENT_PRERUN_SCRIPT",
			Value: "/var/lib/mysql/pmm-prerun.sh",
		},
		{
			Name:  "DB_CLUSTER",
			Value: Name,
		},
		{
			Name:  "DB_USER",
			Value: users.Monitor,
		},
		{
			Name: "DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: SecretKeySelector(secret.Name, users.Monitor),
			},
		},
		{
			Name:  "DB_HOST",
			Value: "localhost",
		},
		{
			Name:  "CLUSTER_NAME",
			Value: clusterName,
		},
	}

	pmmAgentSetupNodeName := "$(POD_NAMESPACE)-$(POD_NAME)"
	if len(envVarsSecret.Data["PMM_PREFIX"]) > 0 {
		pmmAgentSetupNodeName = "$(PMM_PREFIX)$(POD_NAMESPACE)-$(POD_NAME)"
	}
	envs = append(envs, corev1.EnvVar{
		Name:  "PMM_AGENT_SETUP_NODE_NAME",
		Value: pmmAgentSetupNodeName,
	})

	container := corev1.Container{
		Name:            "pmm-client",
		Image:           pmmSpec.Image,
		ImagePullPolicy: pmmSpec.ImagePullPolicy,
		Env:             envs,
		SecurityContext: pmmSpec.ContainerSecurityContext,
		Ports:           ports,
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"-c",
						"pmm-admin unregister --force",
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 60,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt32(7777),
					Path: "/local/Status",
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      BinVolumeName,
				MountPath: "/var/lib/mysql",
			},
		},
		Resources: pmmSpec.Resources,
	}

	if pmmSpec.LivenessProbes != nil {
		container.LivenessProbe = pmmSpec.LivenessProbes
		if reflect.DeepEqual(container.LivenessProbe.ProbeHandler, corev1.ProbeHandler{}) {
			container.LivenessProbe.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{
				Port: intstr.FromInt32(7777),
				Path: "/local/Status",
			}
		}
	}

	if pmmSpec.ReadinessProbes != nil {
		container.ReadinessProbe = pmmSpec.ReadinessProbes
		if reflect.DeepEqual(container.ReadinessProbe.ProbeHandler, corev1.ProbeHandler{}) {
			container.ReadinessProbe.ProbeHandler.HTTPGet = &corev1.HTTPGetAction{
				Port: intstr.FromInt32(7777),
				Path: "/local/Status",
			}
		}
	}

	return container, nil
}
