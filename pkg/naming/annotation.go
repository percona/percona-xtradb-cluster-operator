package naming

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

const AnnotationTLS = "percona.com/tls"

type AnnotationTLSState string

const (
	AnnotationTLSStateEnabled  AnnotationTLSState = "enabled"
	AnnotationTLSStateDisabled AnnotationTLSState = "disabled"
)

func GetAnnotationTLSState(cr *api.PerconaXtraDBCluster) AnnotationTLSState {
	if *cr.Spec.TLS.Enabled {
		return AnnotationTLSStateEnabled
	}
	return AnnotationTLSStateDisabled
}
