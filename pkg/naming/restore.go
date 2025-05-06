package naming

import (
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func PrepareJobName(restore *pxcv1.PerconaXtraDBClusterRestore) string {
	return "prepare-job-" + restore.Name + "-" + restore.Spec.PXCCluster
}
