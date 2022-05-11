package config

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewConfigMap(cr *api.PerconaXtraDBCluster, cmName, filename, content string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			filename: content,
		},
	}
}

func NewAutoTuneConfigMap(cr *api.PerconaXtraDBCluster, cmName string) (*corev1.ConfigMap, error) {
	var memory string

	if cr.Spec.PXC.Resources != nil {
		if cr.Spec.PXC.Resources.Requests != nil {
			if cr.Spec.PXC.Resources.Requests.Memory != nil {
				memory = cr.Spec.PXC.Resources.Requests.Memory.String()
			}
		}
		// Use limits memory in priority if it set
		if cr.Spec.PXC.Resources.Limits != nil {
			if cr.Spec.PXC.Resources.Limits.Memory != nil {
				memory = cr.Spec.PXC.Resources.Limits.Memory.String()
			}
		}
	}
	autotuneParams, err := getAutoTuneParams(memory)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"auto-config.cnf": "[mysqld]" + autotuneParams,
		},
	}, nil
}
