package pxcbackup

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

const (
	fullBackupDestinationSuffix    = "-full"
	logicalBackupDestinationSuffix = "-logical"
)

// backupDestinationName returns the name of the backup in the storage:
// <cluster>-<creation time>-<full|logical>.
func backupDestinationName(cr *api.PerconaXtraDBClusterBackup) string {
	suffix := fullBackupDestinationSuffix
	if isLogicalBackup(cr) {
		suffix = logicalBackupDestinationSuffix
	}

	return cr.Spec.PXCCluster + "-" + cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05") + suffix
}
