package pxcrestore

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/binlogcollector"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

// Add creates a new PerconaXtraDBClusterRestore Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	sv, err := version.Server()
	if err != nil {
		return nil, fmt.Errorf("get version: %v", err)
	}

	cli, err := clientcmd.NewClient()
	if err != nil {
		return nil, errors.Wrap(err, "create clientcmd")
	}

	return &ReconcilePerconaXtraDBClusterRestore{
		client:               mgr.GetClient(),
		clientcmd:            cli,
		scheme:               mgr.GetScheme(),
		serverVersion:        sv,
		newStorageClientFunc: storage.NewClient,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	return builder.ControllerManagedBy(mgr).
		Named("pxcrestore-controller").
		For(&api.PerconaXtraDBClusterRestore{}).
		Complete(r)
}

var _ reconcile.Reconciler = &ReconcilePerconaXtraDBClusterRestore{}

// ReconcilePerconaXtraDBClusterRestore reconciles a PerconaXtraDBClusterRestore object
type ReconcilePerconaXtraDBClusterRestore struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	clientcmd *clientcmd.Client
	scheme    *runtime.Scheme

	serverVersion *version.ServerVersion

	newStorageClientFunc storage.NewClientFunc
}

// Reconcile reads that state of the cluster for a PerconaXtraDBClusterRestore object and makes changes based on the state read
// and what is in the PerconaXtraDBClusterRestore.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBClusterRestore) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	rr := reconcile.Result{
		// TODO: do not depend on the RequeueAfter
		RequeueAfter: time.Second * 5,
	}

	cr := &api.PerconaXtraDBClusterRestore{}
	err := r.client.Get(ctx, request.NamespacedName, cr)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	switch cr.Status.State {
	case api.RestoreSucceeded, api.RestoreFailed:
		if err := r.runJobFinalizers(ctx, cr); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "run job finalizers")
		}
		return reconcile.Result{}, nil
	}

	cr.Status.Comments = ""

	defer func() {
		if err := setStatus(ctx, r.client, cr); err != nil {
			log.Error(err, "failed to set status")
		}
	}()

	if cr.Status.State == api.RestoreNew {
		cr.Status.State = api.RestoreStarting
	}

	otherRestore, err := isOtherRestoreInProgress(ctx, r.client, cr)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to check if other restore is in progress")
	}
	if otherRestore != nil {
		err = errors.Errorf("unable to continue, concurrent restore job %s running now", otherRestore.Name)
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return reconcile.Result{}, err
	}

	if err := cr.CheckNsetDefaults(); err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return reconcile.Result{}, err
	}

	cluster := new(api.PerconaXtraDBCluster)
	if err := r.client.Get(ctx, types.NamespacedName{Name: cr.Spec.PXCCluster, Namespace: cr.Namespace}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			cr.Status.State = api.RestoreFailed
			cr.Status.Comments = err.Error()
		}
		return reconcile.Result{}, errors.Wrapf(err, "get cluster %s", cr.Spec.PXCCluster)
	}

	if err := cluster.CheckNSetDefaults(r.serverVersion, log); err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return reconcile.Result{}, errors.Wrap(err, "wrong PXC options")
	}

	bcp, err := getBackup(ctx, r.client, cr)
	if err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return reconcile.Result{}, errors.Wrap(err, "get backup")
	}

	restorer, err := r.getRestorer(ctx, cr, bcp, cluster)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to get restorer")
	}

	switch cr.Status.State {
	case api.RestoreStarting:
		return r.reconcileStateNew(ctx, restorer, cr, cluster, bcp)
	case api.RestoreStopCluster:
		return r.reconcileStateStopCluster(ctx, restorer, cr, cluster)
	case api.RestoreRestore:
		return r.reconcileStateRestore(ctx, restorer, cr, cluster)
	case api.RestorePITR:
		return r.reconcileStatePITR(ctx, restorer, cr)
	case api.RestorePrepareCluster:
		return r.reconcileStatePrepareCluster(ctx, cr, bcp, cluster)
	case api.RestoreStartCluster:
		return r.reconcileStateStartCluster(ctx, restorer, cr, cluster)
	}

	return reconcile.Result{}, errors.Errorf("unknown state: %s", cr.Status.State)
}

func (r *ReconcilePerconaXtraDBClusterRestore) reconcileStateStartCluster(ctx context.Context, restorer Restorer, cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster) (reconcile.Result, error) {
	log := logf.FromContext(ctx)
	rr := reconcile.Result{
		// TODO: do not depend on the RequeueAfter
		RequeueAfter: time.Second * 5,
	}

	if cluster.Spec.Pause ||
		(cr.Status.PXCSize != 0 && cluster.Spec.PXC.Size != cr.Status.PXCSize) ||
		(cluster.Spec.HAProxy != nil && cr.Status.HAProxySize != 0 && cr.Status.HAProxySize != cluster.Spec.HAProxy.Size) ||
		(cluster.Spec.ProxySQL != nil && cr.Status.ProxySQLSize != 0 && cr.Status.ProxySQLSize != cluster.Spec.ProxySQL.Size) {
		if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			current := new(api.PerconaXtraDBCluster)
			err := r.client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, current)
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

			return r.client.Update(ctx, current)
		}); err != nil {
			return rr, errors.Wrap(err, "update cluster")
		}
		return rr, nil
	}

	if cluster.Status.ObservedGeneration == cluster.Generation && cluster.Status.PXC.Status == api.AppStateReady {
		if err := restorer.Finalize(ctx); err != nil {
			return rr, errors.Wrap(err, "failed to finalize restore")
		}

		cr.Status.State = api.RestoreSucceeded
		return rr, nil
	}

	log.Info("Waiting for cluster to start", "cluster", cluster.Name)
	return rr, nil
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

func (r *ReconcilePerconaXtraDBClusterRestore) reconcileStateNew(ctx context.Context, restorer Restorer, cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster, bcp *api.PerconaXtraDBClusterBackup) (reconcile.Result, error) {
	log := logf.FromContext(ctx)
	rr := reconcile.Result{
		// TODO: do not depend on the RequeueAfter
		RequeueAfter: time.Second * 5,
	}

	if cr.Spec.PITR != nil {
		if err := backup.CheckPITRErrors(ctx, r.client, r.clientcmd, cluster, r.newStorageClientFunc); err != nil {
			return reconcile.Result{}, err
		}

		annotations := cr.GetAnnotations()
		_, unsafePITR := annotations[api.AnnotationUnsafePITR]
		if !unsafePITR {
			ready, err := r.isPITRReady(ctx, cluster, bcp)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "is pitr ready")
			}
			if !ready {
				cr.Status.Comments = fmt.Sprintf("Backup doesn't guarantee consistent recovery with PITR. Annotate PerconaXtraDBClusterRestore with %s to force it.", api.AnnotationUnsafePITR)
				cr.Status.State = api.RestoreFailed
				return reconcile.Result{}, nil
			}
		}
	}

	if err := validate(ctx, restorer, cr); err != nil {
		if errors.Is(err, errWaitValidate) {
			return rr, nil
		}
		cr.Status.Comments = fmt.Sprintf("failed to validate restore job: %s", err.Error())
		cr.Status.State = api.RestoreFailed
		return rr, err
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
	return rr, nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) reconcileStateRestore(ctx context.Context, restorer Restorer, cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster) (reconcile.Result, error) {
	log := logf.FromContext(ctx)
	rr := reconcile.Result{
		// TODO: do not depend on the RequeueAfter
		RequeueAfter: time.Second * 5,
	}

	restorerJob, err := restorer.Job(ctx)
	if err != nil {
		return rr, errors.Wrap(err, "failed to create restore job")
	}
	job := new(batchv1.Job)
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      restorerJob.Name,
		Namespace: restorerJob.Namespace,
	}, job); err != nil {
		return rr, errors.Wrap(err, "failed to get restore job")
	}

	finished, err := isJobFinished(job)
	if err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return rr, err
	}
	if !finished {
		log.Info("Waiting for restore job to finish", "job", job.Name)
		return rr, nil
	}

	if cluster.Spec.Backup.PITR.Enabled {
		if err := binlogcollector.InvalidateCache(ctx, r.client, cluster); err != nil {
			log.Error(err, "failed to invalidate binlog collector cache")
		}
	}

	if cr.Spec.PITR != nil {
		if cluster.Spec.Pause {
			err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				current := new(api.PerconaXtraDBCluster)
				err := r.client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, current)
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

				return r.client.Update(ctx, current)
			})
			if err != nil {
				return rr, errors.Wrap(err, "update cluster")
			}
			return rr, nil
		} else {
			if cluster.Status.ObservedGeneration != cluster.Generation || cluster.Status.PXC.Status != api.AppStateReady || cluster.Status.ProxySQL.Size != 0 || cluster.Status.HAProxy.Size != 0 {
				log.Info("Waiting for cluster to start", "cluster", cluster.Name)
				return rr, nil
			}
		}

		log.Info("point-in-time recovering", "cluster", cr.Spec.PXCCluster)
		if err := createRestoreJob(ctx, r.client, restorer, true); err != nil {
			if errors.Is(err, errWaitInit) {
				return rr, nil
			}
			return rr, errors.Wrap(err, "run pitr")
		}
		cr.Status.State = api.RestorePITR
		return rr, nil
	}

	if cluster.CompareVersionWith("1.18.0") >= 0 {
		log.Info("preparing cluster", "cluster", cr.Spec.PXCCluster)
		cr.Status.State = api.RestorePrepareCluster
	} else {
		log.Info("starting cluster", "cluster", cr.Spec.PXCCluster)
		cr.Status.State = api.RestoreStartCluster
	}

	return rr, nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) reconcileStatePITR(ctx context.Context, restorer Restorer, cr *api.PerconaXtraDBClusterRestore) (reconcile.Result, error) {
	log := logf.FromContext(ctx)
	rr := reconcile.Result{
		// TODO: do not depend on the RequeueAfter
		RequeueAfter: time.Second * 5,
	}

	restorerJob, err := restorer.PITRJob(ctx)
	if err != nil {
		return rr, errors.Wrap(err, "failed to create restore job")
	}
	job := new(batchv1.Job)
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      restorerJob.Name,
		Namespace: restorerJob.Namespace,
	}, job); err != nil {
		return rr, errors.Wrap(err, "failed to get pitr job")
	}

	finished, err := isJobFinished(job)
	if err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return rr, err
	}
	if !finished {
		log.Info("Waiting for restore job to finish", "job", job.Name)
		return rr, nil
	}

	log.Info("starting cluster", "cluster", cr.Spec.PXCCluster)
	cr.Status.State = api.RestoreStartCluster
	return rr, nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) reconcileStateStopCluster(ctx context.Context, restorer Restorer, cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster) (reconcile.Result, error) {
	log := logf.FromContext(ctx)
	rr := reconcile.Result{
		// TODO: do not depend on the RequeueAfter
		RequeueAfter: time.Second * 5,
	}

	// TODO: we should use PauseCluster and delete PVCs
	err := k8s.PauseClusterWithWait(ctx, r.client, cluster, true)
	if err != nil {
		return rr, errors.Wrapf(err, "stop cluster %s", cluster.Name)
	}

	log.Info("starting restore", "cluster", cr.Spec.PXCCluster, "backup", cr.Spec.BackupName)
	if err := createRestoreJob(ctx, r.client, restorer, false); err != nil {
		if errors.Is(err, errWaitInit) {
			return rr, nil
		}
		cr.Status.Comments = fmt.Sprintf("failed to run restore: %s", err.Error())
		cr.Status.State = api.RestoreFailed
		return rr, err
	}
	cr.Status.State = api.RestoreRestore
	return rr, nil
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

func (r *ReconcilePerconaXtraDBClusterRestore) reconcileStatePrepareCluster(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterRestore,
	bcp *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	initImage, err := k8s.GetInitImage(ctx, cluster, r.client)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get init image")
	}

	job, err := backup.PrepareJob(cr, bcp, cluster, initImage, r.scheme)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "prepare job")
	}

	if err := r.client.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
		if k8serrors.IsNotFound(err) {
			if err := r.client.Create(ctx, job); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "create prepare job")
			}
		} else {
			return reconcile.Result{}, errors.Wrap(err, "get prepare job")
		}
	}

	finished, err := isJobFinished(job)
	if err != nil {
		cr.Status.State = api.RestoreFailed
		cr.Status.Comments = err.Error()
		return reconcile.Result{}, err
	}
	if !finished {
		log.Info("Waiting for prepare job to finish", "job", job.Name)
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	log.Info("starting cluster", "cluster", cr.Spec.PXCCluster)
	cr.Status.State = api.RestoreStartCluster
	return reconcile.Result{}, nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) runJobFinalizers(ctx context.Context, cr *api.PerconaXtraDBClusterRestore) error {
	for _, jobName := range []string{
		naming.RestoreJobName(cr, false),
		naming.RestoreJobName(cr, true),
		naming.PrepareJobName(cr),
	} {
		if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			job := new(batchv1.Job)
			if err := r.client.Get(ctx, types.NamespacedName{
				Name:      jobName,
				Namespace: cr.Namespace,
			}, job); err != nil {
				if k8serrors.IsNotFound(err) {
					return nil
				}
				return errors.Wrap(err, "failed to get job")
			}

			if removed := controllerutil.RemoveFinalizer(job, naming.FinalizerKeepJob); removed {
				return r.client.Update(ctx, job)
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to remove keep-job finalizer")
		}
	}
	return nil
}
