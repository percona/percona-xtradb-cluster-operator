package statefulset

import (
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	corev1 "k8s.io/api/core/v1"
)

func EntrypointInitContainer(initImageName string, resources corev1.ResourceRequirements, securityContext *corev1.SecurityContext, pullPolicy corev1.PullPolicy) corev1.Container {
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.DataVolumeName,
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
