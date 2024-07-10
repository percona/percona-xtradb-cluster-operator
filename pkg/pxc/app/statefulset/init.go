package statefulset

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
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

func PitrInitContainer(cluster *api.PerconaXtraDBCluster, resources corev1.ResourceRequirements, initImageName string) corev1.Container {
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.BinVolumeName,
				MountPath: app.BinVolumeMountPath,
			},
		},
		Image:           initImageName,
		ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
		Name:            "pitr-init",
		Command:         []string{"/pitr-init-entrypoint.sh"},
		SecurityContext: cluster.Spec.PXC.ContainerSecurityContext,
		Resources:       resources,
	}
}

func BackupInitContainer(cluster *api.PerconaXtraDBCluster, resources corev1.ResourceRequirements, initImageName string, securityContext *corev1.SecurityContext) corev1.Container {
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.BinVolumeName,
				MountPath: app.BinVolumeMountPath,
			},
		},
		Image:           initImageName,
		ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
		Name:            "backup-init",
		Command:         []string{"/backup-init-entrypoint.sh"},
		SecurityContext: securityContext,
		Resources:       resources,
	}
}
