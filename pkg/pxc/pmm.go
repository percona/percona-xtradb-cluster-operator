package pxc

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func pmmNodeContainer(cr *api.PerconaXtraDBCluster) corev1.Container {
	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "PMM_SERVER",
			Value: cr.Spec.PMM.ServerHost,
		},
		{
			Name:  "DB_TYPE",
			Value: "mysql",
		},
		{
			Name:  "DB_USER",
			Value: "monitor",
		},
		{
			Name: "DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "monitor"),
			},
		},
	}

	if cr.Spec.PMM.ServerUser != "" {
		pmmEnvs = append(pmmEnvs, pmmEnvServerUser(cr)...)
	}

	return corev1.Container{
		Name:            "pmm-client",
		Image:           cr.Spec.PMM.Image,
		ImagePullPolicy: corev1.PullAlways,
		Env:             pmmEnvs,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "datadir",
				MountPath: "/var/lib/mysql",
			},
		},
	}
}

func pmmProxySQLContainer(cr *api.PerconaXtraDBCluster) corev1.Container {
	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "PMM_SERVER",
			Value: cr.Spec.PMM.ServerHost,
		},
		{
			Name:  "DB_TYPE",
			Value: "proxysql",
		},
		{
			Name:  "MONITOR_USER",
			Value: "monitor",
		},
		{
			Name: "MONITOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "monitor"),
			},
		},
		{
			Name:  "DB_ARGS",
			Value: "--dsn $(MONITOR_USER):$(MONITOR_PASSWORD)@tcp(localhost:6032)/",
		},
	}

	if cr.Spec.PMM.ServerUser != "" {
		pmmEnvs = append(pmmEnvs, pmmEnvServerUser(cr)...)
	}

	return corev1.Container{
		Name:            "pmm-client",
		Image:           cr.Spec.PMM.Image,
		ImagePullPolicy: corev1.PullAlways,
		Env:             pmmEnvs,
	}
}

func pmmEnvServerUser(cr *api.PerconaXtraDBCluster) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "PMM_USER",
			Value: cr.Spec.PMM.ServerUser,
		},
		{
			Name: "PMM_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "pmmserver"),
			},
		},
	}
}
