package app

import (
	corev1 "k8s.io/api/core/v1"
)

// Probe is a k8s helper to create Probe object
func Probe(pb *corev1.Probe, cmd ...string) *corev1.Probe {
	pb.Exec = &corev1.ExecAction{
		Command: cmd,
	}
	return pb
}

// SecretKeySelector is a k8s helper to create SecretKeySelector object
func SecretKeySelector(name, key string) *corev1.SecretKeySelector {
	evs := &corev1.SecretKeySelector{}
	evs.Name = name
	evs.Key = key

	return evs
}

// SecretKeySelectorWithOptional is a k8s helper to create SecretKeySelector object with optional flag
func SecretKeySelectorWithOptional(name, key string, optional bool) *corev1.SecretKeySelector {
	evs := &corev1.SecretKeySelector{}
	evs.Name = name
	evs.Key = key
	evs.Optional = &optional

	return evs
}
