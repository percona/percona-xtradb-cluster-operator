package statefulset

import corev1 "k8s.io/api/core/v1"

func EntrypointInitContainer(initImageName string) corev1.Container {
	c := corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
		},
		Image:   initImageName,
		Name:    "pxc-init-entrypoint",
		Command: []string{"/pxc-init-entrypoint.sh"},
	}

	return c
}
