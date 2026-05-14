package naming

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetConditionTLSState(cr *api.PerconaXtraDBCluster) metav1.ConditionStatus {
	if *cr.Spec.TLS.Enabled {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}
