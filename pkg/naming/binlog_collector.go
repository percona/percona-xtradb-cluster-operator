package naming

import api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"

func BinlogCollectorDeploymentName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + "-pitr"
}

func BinlogCollectorServiceName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + "-pitr"
}
