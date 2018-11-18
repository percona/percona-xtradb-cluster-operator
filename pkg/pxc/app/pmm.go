package app

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func PMMClient(spec *api.PMMSpec, secrets string) corev1.Container {
	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "PMM_SERVER",
			Value: spec.ServerHost,
		},
	}

	if spec.ServerUser != "" {
		pmmEnvs = append(pmmEnvs, pmmEnvServerUser(spec.ServerUser, secrets)...)
	}

	return corev1.Container{
		Name:            "pmm-client",
		Image:           spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Env:             pmmEnvs,
	}
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
