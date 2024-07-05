package statefulset

import (
	corev1 "k8s.io/api/core/v1"
)

func EntrypointInitContainer(initImageName string, volumeName string, resources corev1.ResourceRequirements, securityContext *corev1.SecurityContext, pullPolicy corev1.PullPolicy) corev1.Container {
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: "/var/lib/mysql",
			},
		},
		Image:           initImageName,
		ImagePullPolicy: pullPolicy,
		Name:            "pxc-init",
		Command:         []string{"/pxc-init-entrypoint.sh"},
		SecurityContext: securityContext,
		Resources:       resources,
	}
}

func HaproxyEntrypointInitContainer(initImageName string, resources corev1.ResourceRequirements, securityContext *corev1.SecurityContext, pullPolicy corev1.PullPolicy) corev1.Container {
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "local-bin",
				MountPath: "/usr/local/bin",
			},
			{
				Name:      "bin",
				MountPath: "/usr/bin",
			},
			{
				Name:      "etc",
				MountPath: "/etc",
			},
		},
		Image:           initImageName,
		ImagePullPolicy: pullPolicy,
		Name:            "haproxy-init",
		Command:         []string{"/haproxy-init-entrypoint.sh"},
		SecurityContext: securityContext,
		Resources:       resources,
	}
}
