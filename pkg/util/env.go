package util

import (
	corev1 "k8s.io/api/core/v1"
)

func MergeEnvLists(envLists ...[]corev1.EnvVar) []corev1.EnvVar {
	resultList := make([]corev1.EnvVar, 0)
	for _, list := range envLists {
		for _, env := range list {
			idx := FindEnvIndex(resultList, env.Name)
			if idx == -1 {
				resultList = append(resultList, env)
				continue
			}
			resultList[idx] = env
		}
	}
	return resultList
}

func FindEnvIndex(envs []corev1.EnvVar, name string) int {
	for i, env := range envs {
		if env.Name == name {
			return i
		}
	}
	return -1
}
