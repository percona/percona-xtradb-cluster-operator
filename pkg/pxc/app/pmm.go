package app

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func PMMClient(spec *api.PMMSpec, secrets string, v120OrGreater bool) corev1.Container {
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
		pmmEnvs = append(pmmEnvs, pmmEnvServerUser(spec.ServerUser, secrets)...)
	}

	container := corev1.Container{
		Name:            "pmm-client",
		Image:           spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Env:             pmmEnvs,
	}

	if v120OrGreater {
		container.Env = append(container.Env, clientEnvs...)
		container.Ports = ports
	}

	return container
}

func pmmEnvServerUser(user, secrets string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "PMM_USER",
			Value: user,
		},
		{
			Name: "PMM_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: SecretKeySelector(secrets, "pmmserver"),
			},
		},
	}
}
