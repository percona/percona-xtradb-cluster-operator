package pxc

import api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"

func (r *ReconcilePerconaXtraDBCluster) ensureVersion(cr *api.PerconaXtraDBCluster, vs VersionService) error {
	return nil
}

type VersionService interface {
}
