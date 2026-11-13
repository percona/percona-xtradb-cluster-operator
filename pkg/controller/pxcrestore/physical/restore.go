// Package physical restores physical (xtrabackup) backups. The cluster is
// stopped, a job replaces the data directory of the first PXC pod, and the
// cluster is started again, with an optional point-in-time recovery in between.
package physical

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/common"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/binlogcollector"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

type controller struct {
	common.Options
}

// NewController returns the controller that restores a physical backup.
func NewController(opts common.Options) *controller {
	return &controller{Options: opts}
}

// Restore moves the restore one step forward, according to its state. It
// reports progress by changing the status of the restore object.
func (c *controller) Restore(ctx context.Context) error {
	cr := c.CR

	if cr.Spec.Job != nil && cr.Spec.Job.Volume != nil {
		common.FailRestore(cr, "job.volume is only supported for logical restores")
		return nil
	}

	restorer, err := getRestorer(ctx, c.Options)
	if err != nil {
		common.FailRestore(cr, err.Error())
		return errors.Wrap(err, "failed to get restorer")
	}

	switch cr.Status.State {
	case api.RestoreStarting:
		return c.reconcileStateNew(ctx, restorer)
	case api.RestoreStopCluster:
		return c.reconcileStateStopCluster(ctx, restorer)
	case api.RestoreRestore:
		return c.reconcileStateRestore(ctx, restorer)
	case api.RestorePITR:
		return c.reconcileStatePITR(ctx, restorer)
	case api.RestorePrepareCluster:
		return c.reconcileStatePrepareCluster(ctx, restorer)
	case api.RestoreStartCluster:
		return c.reconcileStateStartCluster(ctx, restorer)
	}

	return errors.Errorf("unknown state: %s", cr.Status.State)
}

func (c *controller) reconcileStateNew(ctx context.Context, restorer Restorer) error {
	log := logf.FromContext(ctx)
	cr, cluster := c.CR, c.Cluster

	if cr.Spec.PITR != nil {
		if err := backup.CheckPITRErrors(ctx, c.Client, c.ClientCmd, cluster, c.NewStorageClient); err != nil {
			return err
		}

		annotations := cr.GetAnnotations()
		_, unsafePITR := annotations[api.AnnotationUnsafePITR]
		if !unsafePITR {
			ready, err := c.isPITRReady(ctx)
			if err != nil {
				return errors.Wrap(err, "is pitr ready")
			}
			if !ready {
				cr.Status.Comments = fmt.Sprintf("Backup doesn't guarantee consistent recovery with PITR. Annotate PerconaXtraDBClusterRestore with %s to force it.", api.AnnotationUnsafePITR)
				cr.Status.State = api.RestoreFailed
				return nil
			}
		}
	}

	if err := validate(ctx, restorer, cr); err != nil {
		if errors.Is(err, errWaitValidate) {
			return nil
		}
		cr.Status.Comments = fmt.Sprintf("failed to validate restore job: %s", err.Error())
		cr.Status.State = api.RestoreFailed
		return err
	}
	cr.Status.PXCSize = cluster.Spec.PXC.Size
	if cluster.Spec.ProxySQL != nil {
		cr.Status.ProxySQLSize = cluster.Spec.ProxySQL.Size
	}
	if cluster.Spec.HAProxy != nil {
		cr.Status.HAProxySize = cluster.Spec.HAProxy.Size
	}
	cr.Status.Unsafe = cluster.Spec.Unsafe

	log.Info("stopping cluster", "cluster", cr.Spec.PXCCluster)
	cr.Status.State = api.RestoreStopCluster
	return nil
}

func validate(ctx context.Context, restorer Restorer, cr *api.PerconaXtraDBClusterRestore) error {
	job, err := restorer.Job(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create restore job")
	}
	if err := restorer.ValidateJob(ctx, job); err != nil {
		return errors.Wrap(err, "failed to validate job")
	}

	if cr.Spec.PITR != nil {
		job, err := restorer.PITRJob(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create pitr restore job")
		}
		if err := restorer.ValidateJob(ctx, job); err != nil {
			return errors.Wrap(err, "failed to validate pitr job")
		}
	}
	if err := restorer.Validate(ctx); err != nil {
		return errors.Wrap(err, "failed to validate backup existence")
	}
	return nil
}

func (c *controller) reconcileStateStopCluster(ctx context.Context, restorer Restorer) error {
	log := logf.FromContext(ctx)
	cr, cluster := c.CR, c.Cluster

	// TODO: we should use PauseCluster and delete PVCs
	err := k8s.PauseClusterWithWait(ctx, c.Client, cluster, true)
	if err != nil {
		return errors.Wrapf(err, "stop cluster %s", cluster.Name)
	}

	log.Info("starting restore", "cluster", cr.Spec.PXCCluster, "backup", cr.Spec.BackupName)
	if err := createRestoreJob(ctx, c.Client, restorer, false); err != nil {
		if errors.Is(err, errWaitInit) {
			return nil
		}
		cr.Status.Comments = fmt.Sprintf("failed to run restore: %s", err.Error())
		cr.Status.State = api.RestoreFailed
		return err
	}
	cr.Status.State = api.RestoreRestore
	return nil
}

func createRestoreJob(ctx context.Context, cl client.Client, restorer Restorer, pitr bool) error {
	if err := restorer.Init(ctx); err != nil {
		return errors.Wrap(err, "failed to init restore")
	}

	job, err := restorer.Job(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get restore job")
	}
	if pitr {
		job, err = restorer.PITRJob(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create pitr restore job")
		}
	}

	if err := cl.Create(ctx, job); err != nil && !k8serrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "create job")
	}

	return nil
}

func (c *controller) reconcileStateRestore(ctx context.Context, restorer Restorer) error {
	log := logf.FromContext(ctx)
	cr, cluster := c.CR, c.Cluster

	restorerJob, err := restorer.Job(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create restore job")
	}
	job := new(batchv1.Job)
	if err := c.Client.Get(ctx, types.NamespacedName{
		Name:      restorerJob.Name,
		Namespace: restorerJob.Namespace,
	}, job); err != nil {
		return errors.Wrap(err, "failed to get restore job")
	}

	finished, err := common.IsJobFinished(job)
	if err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return err
	}
	if !finished {
		log.Info("Waiting for restore job to finish", "job", job.Name)
		return nil
	}

	if cluster.Spec.Backup.PITR.Enabled {
		if err := binlogcollector.InvalidateCache(ctx, c.Client, cluster); err != nil {
			log.Error(err, "failed to invalidate binlog collector cache")
		}
	}

	if cluster.CompareVersionWith("1.18.0") >= 0 {
		log.Info("preparing cluster", "cluster", cr.Spec.PXCCluster)
		cr.Status.State = api.RestorePrepareCluster
	} else {
		log.Info("starting cluster", "cluster", cr.Spec.PXCCluster)
		cr.Status.State = api.RestoreStartCluster
	}

	return nil
}

func (c *controller) reconcileStatePrepareCluster(ctx context.Context, restorer Restorer) error {
	log := logf.FromContext(ctx)
	cr, cluster, bcp := c.CR, c.Cluster, c.Backup

	initImage, err := k8s.GetInitImage(ctx, cluster, c.Client)
	if err != nil {
		return errors.Wrap(err, "get init image")
	}

	job, err := backup.PrepareJob(cr, bcp, cluster, initImage, c.Scheme)
	if err != nil {
		return errors.Wrap(err, "prepare job")
	}

	if err := c.Client.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
		if k8serrors.IsNotFound(err) {
			if err := c.Client.Create(ctx, job); err != nil {
				return errors.Wrap(err, "create prepare job")
			}
		} else {
			return errors.Wrap(err, "get prepare job")
		}
	}

	finished, err := common.IsJobFinished(job)
	if err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return err
	}
	if !finished {
		log.Info("Waiting for prepare job to finish", "job", job.Name)
		return nil
	}

	if cr.Spec.PITR != nil {
		if cluster.Spec.Pause {
			err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				current := new(api.PerconaXtraDBCluster)
				err := c.Client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, current)
				if err != nil {
					return errors.Wrap(err, "get cluster")
				}
				current.Spec.Pause = false
				current.Spec.PXC.Size = 1
				current.Spec.Unsafe.PXCSize = true
				current.Spec.Unsafe.ProxySize = true

				if current.Spec.ProxySQL != nil {
					current.Spec.ProxySQL.Size = 0
				}

				if current.Spec.HAProxy != nil {
					current.Spec.HAProxy.Size = 0
				}

				return c.Client.Update(ctx, current)
			})
			if err != nil {
				return errors.Wrap(err, "update cluster")
			}
			return nil
		} else {
			if cluster.Status.ObservedGeneration != cluster.Generation || cluster.Status.PXC.Status != api.AppStateReady || cluster.Status.ProxySQL.Size != 0 || cluster.Status.HAProxy.Size != 0 {
				log.Info("Waiting for cluster to start", "cluster", cluster.Name)
				return nil
			}
		}

		log.Info("point-in-time recovering", "cluster", cr.Spec.PXCCluster)
		if err := createRestoreJob(ctx, c.Client, restorer, true); err != nil {
			if errors.Is(err, errWaitInit) {
				return nil
			}
			return errors.Wrap(err, "run pitr")
		}
		cr.Status.State = api.RestorePITR
		return nil
	}

	log.Info("starting cluster", "cluster", cr.Spec.PXCCluster)
	cr.Status.State = api.RestoreStartCluster
	return nil
}

func (c *controller) reconcileStatePITR(ctx context.Context, restorer Restorer) error {
	log := logf.FromContext(ctx)
	cr := c.CR

	restorerJob, err := restorer.PITRJob(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create restore job")
	}
	job := new(batchv1.Job)
	if err := c.Client.Get(ctx, types.NamespacedName{
		Name:      restorerJob.Name,
		Namespace: restorerJob.Namespace,
	}, job); err != nil {
		return errors.Wrap(err, "failed to get pitr job")
	}

	finished, err := common.IsJobFinished(job)
	if err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return err
	}
	if !finished {
		log.Info("Waiting for restore job to finish", "job", job.Name)
		return nil
	}

	log.Info("starting cluster", "cluster", cr.Spec.PXCCluster)
	cr.Status.State = api.RestoreStartCluster
	return nil
}

func (c *controller) reconcileStateStartCluster(ctx context.Context, restorer Restorer) error {
	log := logf.FromContext(ctx)
	cr, cluster := c.CR, c.Cluster

	if cluster.Spec.Pause ||
		(cr.Status.PXCSize != 0 && cluster.Spec.PXC.Size != cr.Status.PXCSize) ||
		(cluster.Spec.HAProxy != nil && cr.Status.HAProxySize != 0 && cr.Status.HAProxySize != cluster.Spec.HAProxy.Size) ||
		(cluster.Spec.ProxySQL != nil && cr.Status.ProxySQLSize != 0 && cr.Status.ProxySQLSize != cluster.Spec.ProxySQL.Size) {
		if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			current := new(api.PerconaXtraDBCluster)
			err := c.Client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, current)
			if err != nil {
				return errors.Wrap(err, "get cluster")
			}
			current.Spec.Pause = false
			current.Spec.PXC.Size = cr.Status.PXCSize
			current.Spec.Unsafe = cr.Status.Unsafe

			if current.Spec.ProxySQL != nil {
				current.Spec.ProxySQL.Size = cr.Status.ProxySQLSize
			}

			if current.Spec.HAProxy != nil {
				current.Spec.HAProxy.Size = cr.Status.HAProxySize
			}

			return c.Client.Update(ctx, current)
		}); err != nil {
			return errors.Wrap(err, "update cluster")
		}
		return nil
	}

	if cluster.Status.ObservedGeneration == cluster.Generation && cluster.Status.PXC.Status == api.AppStateReady {
		if err := restorer.Finalize(ctx); err != nil {
			return errors.Wrap(err, "failed to finalize restore")
		}

		cr.Status.State = api.RestoreSucceeded
		return nil
	}

	log.Info("Waiting for cluster to start", "cluster", cluster.Name)
	return nil
}

// isPITRReady reports whether the backup can be used as the base of
// a point-in-time recovery.
func (c *controller) isPITRReady(ctx context.Context) (bool, error) {
	bcp := c.Backup

	cond := meta.FindStatusCondition(bcp.Status.Conditions, api.BackupConditionPITRReady)
	if cond != nil && cond.Status == metav1.ConditionFalse {
		return false, nil
	}

	opts, err := storage.GetOptionsFromBackup(ctx, c.Client, c.Cluster, bcp)
	if err != nil {
		return false, errors.Wrap(err, "failed to get storage options")
	}

	stg, err := c.NewStorageClient(ctx, opts)
	if err != nil {
		return false, errors.Wrap(err, "new storage")
	}

	filepath := bcp.Status.Destination.BackupName() + "." + naming.PITRNotReady
	objReader, err := stg.GetObject(ctx, filepath)
	if err == nil {
		objReader.Close()
		return false, nil
	}
	if errors.Is(err, storage.ErrObjectNotFound) {
		return true, nil
	}

	return false, errors.Wrap(err, "get pitr-not-ready file from storage")
}
