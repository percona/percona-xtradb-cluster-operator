package naming

import (
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func PrepareJobName(restore *pxcv1.PerconaXtraDBClusterRestore) string {
	return "prepare-job-" + restore.Name + "-" + restore.Spec.PXCCluster
}

func RestoreJobName(cr *pxcv1.PerconaXtraDBClusterRestore, pitr bool) string {
	prefix := "restore-job-"
	if pitr {
		prefix = "pitr-job-"
	}
	return prefix + cr.Name + "-" + cr.Spec.PXCCluster
}
