package app

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func GetConfigVolumes(cvName, cmName string) corev1.Volume {
	vol1 := corev1.Volume{
		Name: cvName,
	}

	vol1.ConfigMap = &corev1.ConfigMapVolumeSource{}
	vol1.ConfigMap.Name = cmName
	t := true
	vol1.ConfigMap.Optional = &t
	return vol1
}

func GetSecretVolumes(cvName, cmName string, optional bool) corev1.Volume {
	vol1 := corev1.Volume{
		Name: cvName,
	}

	vol1.Secret = &corev1.SecretVolumeSource{}
	vol1.Secret.SecretName = cmName
	vol1.Secret.Optional = &optional
	return vol1
}

func GetTmpVolume(cvName string) corev1.Volume {
	return corev1.Volume{
		Name: cvName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func Volumes(podSpec *api.PodSpec, dataVolumeName string) *api.Volume {
	var volume api.Volume

	if podSpec.VolumeSpec != nil && podSpec.VolumeSpec.PersistentVolumeClaim != nil {
		pvcs := PVCs(dataVolumeName, podSpec.VolumeSpec)
		volume.PVCs = pvcs
		return &volume
	}

	if podSpec.VolumeSpec == nil {
		volume.Volumes = []corev1.Volume{}
		return &volume
	}

	volume.Volumes = append(volume.Volumes, corev1.Volume{
		VolumeSource: corev1.VolumeSource{
			HostPath: podSpec.VolumeSpec.HostPath,
			EmptyDir: podSpec.VolumeSpec.EmptyDir,
		},
		Name: dataVolumeName,
	})

	return &volume
}
