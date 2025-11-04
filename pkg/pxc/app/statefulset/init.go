package statefulset

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

func EntrypointInitContainer(cr *api.PerconaXtraDBCluster, initImageName string, volumeName string) corev1.Container {
	initResources := cr.Spec.PXC.Resources
	if cr.Spec.InitContainer.Resources != nil {
		initResources = *cr.Spec.InitContainer.Resources
	}
	securityContext := cr.Spec.PXC.ContainerSecurityContext
	if cr.CompareVersionWith("1.16.0") >= 0 {
		if cr.Spec.InitContainer.ContainerSecurityContext != nil {
			securityContext = cr.Spec.InitContainer.ContainerSecurityContext
		}
	}
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: "/var/lib/mysql",
			},
		},
		Image:           initImageName,
		ImagePullPolicy: cr.Spec.PXC.ImagePullPolicy,
		Name:            "pxc-init",
		Command:         []string{"/pxc-init-entrypoint.sh"},
		SecurityContext: securityContext,
		Resources:       initResources,
	}
}

func PitrInitContainer(cluster *api.PerconaXtraDBCluster, initImageName string) corev1.Container {
	securityContext := cluster.Spec.PXC.ContainerSecurityContext
	if cluster.CompareVersionWith("1.16.0") >= 0 {
		if cluster.Spec.InitContainer.ContainerSecurityContext != nil {
			securityContext = cluster.Spec.InitContainer.ContainerSecurityContext
		}
	}

	resources := corev1.ResourceRequirements{}
	if cluster.Spec.InitContainer.Resources != nil {
		resources = *cluster.Spec.InitContainer.Resources
	}

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
		SecurityContext: securityContext,
		Resources:       resources,
	}
}

func BackupInitContainer(cluster *api.PerconaXtraDBCluster, initImageName string, securityContext *corev1.SecurityContext) corev1.Container {
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
		Resources:       *cluster.Spec.InitContainer.Resources,
	}
}

func HaproxyEntrypointInitContainer(cluster *api.PerconaXtraDBCluster, initImageName string) corev1.Container {
	securityContext := cluster.Spec.HAProxy.ContainerSecurityContext
	if cluster.CompareVersionWith("1.16.0") >= 0 {
		if cluster.Spec.InitContainer.ContainerSecurityContext != nil {
			securityContext = cluster.Spec.InitContainer.ContainerSecurityContext
		}
	}
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.BinVolumeName,
				MountPath: app.BinVolumeMountPath,
			},
		},
		Image:           initImageName,
		ImagePullPolicy: cluster.Spec.HAProxy.ImagePullPolicy,
		Name:            "haproxy-init",
		Command:         []string{"/haproxy-init-entrypoint.sh"},
		SecurityContext: securityContext,
		Resources:       *cluster.Spec.InitContainer.Resources,
	}
}

func ProxySQLEntrypointInitContainer(cluster *api.PerconaXtraDBCluster, initImageName string) corev1.Container {
	securityContext := cluster.Spec.ProxySQL.ContainerSecurityContext
	if cluster.CompareVersionWith("1.16.0") >= 0 {
		if cluster.Spec.InitContainer.ContainerSecurityContext != nil {
			securityContext = cluster.Spec.InitContainer.ContainerSecurityContext
		}
	}
	return corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.BinVolumeName,
				MountPath: app.BinVolumeMountPath,
			},
		},
		Image:           initImageName,
		ImagePullPolicy: cluster.Spec.ProxySQL.ImagePullPolicy,
		Name:            "proxysql-init",
		Command:         []string{"/proxysql-init-entrypoint.sh"},
		SecurityContext: securityContext,
		Resources:       *cluster.Spec.InitContainer.Resources,
	}
}
