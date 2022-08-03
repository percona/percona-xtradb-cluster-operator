package app

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func PMMClient(spec *api.PMMSpec, secret *corev1.Secret, v120OrGreater bool, v170OrGreater bool) corev1.Container {
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

	container := corev1.Container{
		Name:            "pmm-client",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Env:             pmmEnvs,
		SecurityContext: spec.ContainerSecurityContext,
	}

	if v120OrGreater {
		container.Env = append(container.Env, clientEnvs...)
		container.Ports = ports
	}

	if v170OrGreater {
		container.LivenessProbe = &corev1.Probe{
			InitialDelaySeconds: 60,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.FromInt(7777),
					Path: "/local/Status",
				},
			},
		}
		container.Env = append(container.Env, pmmAgentEnvs(spec.ServerHost, spec.ServerUser, secret.Name, spec.UseAPI(secret))...)
		container.Lifecycle = &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					// TODO https://jira.percona.com/browse/PMM-7010
					Command: []string{"bash", "-c", "pmm-admin inventory remove node --force $(pmm-admin status --json | python -c \"import sys, json; print(json.load(sys.stdin)['pmm_agent_status']['node_id'])\")"},
				},
			},
		}
	}

	return container
}

func pmmAgentEnvs(pmmServerHost, pmmServerUser, secrets string, useAPI bool) []corev1.EnvVar {
	var pmmServerPassKey string
	if useAPI {
		pmmServerUser = "api_key"
		pmmServerPassKey = "pmmserverkey"
	} else {
		pmmServerPassKey = "pmmserver"
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
			Name:  "PMM_AGENT_SETUP_NODE_NAME",
			Value: "$(POD_NAMESPASE)-$(POD_NAME)",
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

func PMMAgentScript(dbType string) []corev1.EnvVar {
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

func pmmEnvServerUser(user, secrets string, useAPI bool) []corev1.EnvVar {
	var passKey string
	if useAPI {
		user = "api_key"
		passKey = "pmmserverkey"
	} else {
		passKey = "pmmserver"
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
