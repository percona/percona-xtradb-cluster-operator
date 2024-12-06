package naming

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

const ConditionTLS api.AppState = "tls"

type ConditionTLSState string

const (
	ConditionTLSStateEnabled  ConditionTLSState = "enabled"
	ConditionTLSStateDisabled ConditionTLSState = "disabled"
)

func GetConditionTLSState(cr *api.PerconaXtraDBCluster) ConditionTLSState {
	if *cr.Spec.TLS.Enabled {
		return ConditionTLSStateEnabled
	}
	return ConditionTLSStateDisabled
}
