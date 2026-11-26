package config

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

func NewConfigMap(cr *api.PerconaXtraDBCluster, cmName, filename, content string) *corev1.ConfigMap {
	var ls map[string]string
	if cr.CompareVersionWith("1.16.0") >= 0 {
		ls = naming.LabelsCluster(cr)
	}
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
			Labels:    ls,
		},
		Data: map[string]string{
			filename: content,
		},
	}
}

const AutoConfigKey = "auto-config.cnf"

func NewAutoConfigMap(cr *api.PerconaXtraDBCluster, cmName, params string) *corev1.ConfigMap {
	return NewConfigMap(cr, cmName, AutoConfigKey, "[mysqld]"+params)
}

func AutoTuneConfigMapName(clusterName, component string) string {
	return fmt.Sprintf("auto-%s-%s", clusterName, component)
}

func HookScriptConfigMapName(clusterName, component string) string {
	return fmt.Sprintf("%s-%s-hookscript", clusterName, component)
}

func CustomConfigMapName(clusterName, component string) string {
	return fmt.Sprintf("%s-%s", clusterName, component)
}

func AuthPolicyConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-auth-policy", clusterName)
}
