package config

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewConfigMap(cr *api.PerconaXtraDBCluster, cmName string) *corev1.ConfigMap {
	conf := cr.Spec.PXC.Configuration

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
			"init.cnf": conf,
		},
	}
}

func NewAutoTuneConfigMap(cr *api.PerconaXtraDBCluster, cmName string) (*corev1.ConfigMap, error) {
	var conf string
	if len(cr.Spec.PXC.Resources.Limits.Memory) > 0 || len(cr.Spec.PXC.Resources.Requests.Memory) > 0 {
		memory := cr.Spec.PXC.Resources.Requests.Memory
		if len(cr.Spec.PXC.Resources.Limits.Memory) > 0 {
			memory = cr.Spec.PXC.Resources.Limits.Memory
		}
		autotuneParams, err := getAutoTuneParams(memory)
		if err != nil {
			return nil, err
		}
		conf = "[mysqld]" + autotuneParams
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"init.cnf": conf,
		},
	}

	return cm, nil
}
