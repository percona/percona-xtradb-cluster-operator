package pxcbackup

import (
	"context"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

// isLogicalBackup reports whether the backup was requested as a logical one.
func isLogicalBackup(cr *api.PerconaXtraDBClusterBackup) bool {
	return cr.Spec.Type == api.BackupTypeLogical
}

// validateLogicalBackup checks that the cluster and the storage can be used
// for a logical backup.
func validateLogicalBackup(cluster *api.PerconaXtraDBCluster, storageName string, storageSpec *api.BackupStorageSpec) error {
	if cluster.CompareVersionWith("1.21.0") < 0 {
		return errors.Errorf("logical backups require crVersion 1.21.0 or newer")
	}

	if storageSpec.Type != api.BackupStorageS3 {
		return errors.Errorf("logical backups are supported only on s3 storage, storage %s has type %s", storageName, storageSpec.Type)
	}

	return nil
}

// logicalBackupSize returns the total size in bytes of the objects of
// a logical backup.
func logicalBackupSize(ctx context.Context, s storage.Storage, destination api.PXCBackupDestination) (int64, error) {
	objects, err := s.ListObjectInfos(ctx, destination.BackupName()+"/")
	if err != nil {
		return 0, errors.Wrap(err, "list backup objects")
	}

	return storage.TotalSize(objects), nil
}

// getLogicalBackupSize returns the size of a finished logical backup, or 0
// when it can't be determined. The size is only used to size the volume of
// a restore, which can compute or override it, so a failure here must not
// fail a backup that has succeeded.
func (r *ReconcilePerconaXtraDBClusterBackup) getLogicalBackupSize(
	ctx context.Context,
	cluster *api.PerconaXtraDBCluster,
	cr *api.PerconaXtraDBClusterBackup,
) int64 {
	log := logf.FromContext(ctx).WithValues("backup", cr.Name, "namespace", cr.Namespace)

	opts, err := storage.GetOptionsFromBackup(ctx, r.client, cluster, cr)
	if err != nil {
		log.Error(err, "failed to get storage options, backup size is unknown")
		return 0
	}

	s, err := r.newStorageClientFunc(ctx, opts)
	if err != nil {
		log.Error(err, "failed to create storage client, backup size is unknown")
		return 0
	}

	size, err := logicalBackupSize(ctx, s, cr.Status.Destination)
	if err != nil {
		log.Error(err, "failed to get backup size, backup size is unknown")
		return 0
	}

	return size
}
