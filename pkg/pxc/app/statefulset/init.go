package statefulset

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func EntrypointInitContainer(initImageName string, securityContext *corev1.SecurityContext) corev1.Container {
	c := corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
		},
		Image:           initImageName,
		ImagePullPolicy: corev1.PullAlways,
		Name:            "pxc-init",
		Command:         []string{"/pxc-init-entrypoint.sh"},
		SecurityContext: securityContext,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("600m"),
				corev1.ResourceMemory: resource.MustParse("1G"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1G"),
			},
		},
	}

	return c
}
