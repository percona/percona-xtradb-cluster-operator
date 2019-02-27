package app

import (
	corev1 "k8s.io/api/core/v1"
)

func GetConfigVolumes(cvName string) corev1.Volume {
	vol1 := corev1.Volume{
		Name: "config-volume",
	}

	vol1.ConfigMap = &corev1.ConfigMapVolumeSource{}
	vol1.ConfigMap.Name = cvName
	t := true
	vol1.ConfigMap.Optional = &t
	return vol1
}
