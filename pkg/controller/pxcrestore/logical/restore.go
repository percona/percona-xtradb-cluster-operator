// Package logical restores logical (mydumper) backups. Unlike a physical
// restore it never stops the cluster or changes its spec: a job downloads
// the dump and loads it with myloader into the running cluster.
package logical

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/common"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

// metadataObject is the file mydumper writes last. Its presence marks
// a logical backup as complete, and tells it apart from a physical one.
const metadataObject = "metadata"

type controller struct {
	common.Options
}

// NewController returns the controller that restores a logical backup.
func NewController(opts common.Options) *controller {
	return &controller{Options: opts}
}

// Restore moves the restore one step forward, according to its state. It
// reports progress by changing the status of the restore object.
func (c *controller) Restore(ctx context.Context) error {
	switch c.CR.Status.State {
	case api.RestoreStarting:
		return c.reconcileStateNew(ctx)
	case api.RestoreRestore:
		return c.reconcileStateRestore(ctx)
	}

	return errors.Errorf("unknown state of a logical restore: %s", c.CR.Status.State)
}

// IsLogicalBackup reports whether the backup in the storage is a logical
// one, by looking for the metadata object of a logical backup. It is for
// backups that don't state their type.
func IsLogicalBackup(ctx context.Context, opts common.Options) (bool, error) {
	objects, err := listBackupObjects(ctx, opts)
	if err != nil {
		return false, err
	}

	return hasMetadata(objects, opts.Backup), nil
}

// listBackupObjects lists the objects of the backup in its storage.
func listBackupObjects(ctx context.Context, opts common.Options) ([]storage.ObjectInfo, error) {
	storageOpts, err := storage.GetOptionsFromBackup(ctx, opts.Client, opts.Cluster, opts.Backup)
	if err != nil {
		return nil, errors.Wrap(err, "get storage options")
	}

	s, err := opts.NewStorageClient(ctx, storageOpts)
	if err != nil {
		return nil, errors.Wrap(err, "create storage client")
	}

	objects, err := s.ListObjectInfos(ctx, common.BackupListPath(s.GetPrefix(), opts.Backup.Status.Destination))
	if err != nil {
		return nil, errors.Wrap(err, "list backup objects")
	}

	return objects, nil
}

// hasMetadata reports whether the listed objects of the backup include the
// metadata object of a logical backup.
func hasMetadata(objects []storage.ObjectInfo, bcp *api.PerconaXtraDBClusterBackup) bool {
	metadata := bcp.Status.Destination.BackupName() + "/" + metadataObject
	for _, object := range objects {
		if strings.TrimPrefix(object.Name, "/") == metadata {
			return true
		}
	}
	return false
}

// validate checks what can be checked without looking at the storage.
func validate(
	cr *api.PerconaXtraDBClusterRestore,
	cluster *api.PerconaXtraDBCluster,
	bcp *api.PerconaXtraDBClusterBackup,
) error {
	if cr.Spec.PITR != nil {
		return errors.New("point-in-time recovery is not supported for logical backups")
	}

	// The restore job gets its script from the init image, and the init image
	// of a cluster on an older crVersion doesn't have it.
	if cluster.CompareVersionWith("1.21.0") < 0 {
		return errors.New("logical restores require crVersion 1.21.0 or newer")
	}

	if bcp.Status.Destination.StorageTypePrefix() != api.AwsBlobStoragePrefix {
		return errors.New("logical backups are supported only on s3 storage")
	}

	// The dump is loaded over SQL, so the cluster has to be up.
	if cluster.Status.Status != api.AppStateReady {
		return errors.Errorf("cluster must be ready for a logical restore, current status: %s", cluster.Status.Status)
	}

	return nil
}

// volumeSize returns the size of the volume the dump is downloaded to: the
// user's override, or a size derived from the backup size. The backup size
// is the one recorded when the backup was taken, or the sum of the listed
// objects when none was recorded.
func volumeSize(
	cr *api.PerconaXtraDBClusterRestore,
	bcp *api.PerconaXtraDBClusterBackup,
	objects []storage.ObjectInfo,
) resource.Quantity {
	var override *resource.Quantity
	if cr.Spec.Job != nil && cr.Spec.Job.Volume != nil {
		override = cr.Spec.Job.Volume.Size
	}

	dumpSize := bcp.Status.Size
	if dumpSize == 0 {
		dumpSize = storage.TotalSize(objects)
	}

	return backup.LogicalRestoreVolumeSize(dumpSize, override)
}

func (c *controller) reconcileStateNew(ctx context.Context) error {
	log := logf.FromContext(ctx)
	cr, cluster, bcp := c.CR, c.Cluster, c.Backup

	if err := validate(cr, cluster, bcp); err != nil {
		common.FailRestore(cr, err.Error())
		return nil
	}

	objects, err := listBackupObjects(ctx, c.Options)
	if err != nil {
		common.FailRestore(cr, fmt.Sprintf("failed to validate backup existence: %s", err.Error()))
		return nil
	}
	if !hasMetadata(objects, bcp) {
		common.FailRestore(cr, fmt.Sprintf("backup not found or incomplete: no %s object in %s", metadataObject, bcp.Status.Destination))
		return nil
	}

	initImage, err := k8s.GetInitImage(ctx, cluster, c.Client)
	if err != nil {
		return errors.Wrap(err, "get init image")
	}

	job, err := backup.LogicalRestoreJob(ctx, cr, bcp, cluster, initImage, c.Scheme, volumeSize(cr, bcp, objects))
	if err != nil {
		common.FailRestore(cr, fmt.Sprintf("failed to create restore job: %s", err.Error()))
		return nil
	}

	if err := c.validateJob(ctx, job); err != nil {
		common.FailRestore(cr, fmt.Sprintf("failed to validate job: %s", err.Error()))
		return nil
	}

	log.Info("starting logical restore", "cluster", cr.Spec.PXCCluster, "backup", bcp.Name, "job", job.Name)
	if err := c.Client.Create(ctx, job); client.IgnoreAlreadyExists(err) != nil {
		return errors.Wrap(err, "create restore job")
	}

	cr.Status.State = api.RestoreRestore
	return nil
}

// validateJob checks that the secrets the job refers to exist. As for
// a physical restore from s3, the check is skipped when the storage has no
// credentials secret, which is the case with IAM roles.
func (c *controller) validateJob(ctx context.Context, job *batchv1.Job) error {
	if c.Backup.Status.S3.CredentialsSecret == "" {
		return nil
	}

	return common.ValidateJobSecrets(ctx, c.Client, job)
}

func (c *controller) reconcileStateRestore(ctx context.Context) error {
	log := logf.FromContext(ctx)
	cr := c.CR

	job := new(batchv1.Job)
	if err := c.Client.Get(ctx, types.NamespacedName{
		Name:      naming.RestoreJobName(cr, false),
		Namespace: cr.Namespace,
	}, job); err != nil {
		return errors.Wrap(err, "failed to get restore job")
	}

	finished, err := common.IsJobFinished(job)
	if err != nil {
		common.FailRestore(cr, err.Error())
		return nil
	}
	if !finished {
		log.Info("Waiting for restore job to finish", "job", job.Name)
		return nil
	}

	log.Info("logical restore finished", "cluster", cr.Spec.PXCCluster, "job", job.Name)
	cr.Status.State = api.RestoreSucceeded
	return nil
}
