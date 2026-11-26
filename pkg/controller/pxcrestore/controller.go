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
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/common"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/logical"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/physical"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

// restoreController runs the restore of one kind of backup. Restore moves the
// restore one step forward and reports progress through the status of the
// restore object.
type restoreController interface {
	Restore(context.Context) error
}

// getRestoreController returns the controller for the given type of backup.
func getRestoreController(t api.PXCBackupType, opts common.Options) restoreController {
	if t == api.BackupTypeLogical {
		return logical.NewController(opts)
	}
	return physical.NewController(opts)
}

// Add creates a new PerconaXtraDBClusterRestore Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, mgr manager.Manager) error {
	r, err := newReconciler(ctx, mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx context.Context, mgr manager.Manager) (reconcile.Reconciler, error) {
	sv, err := version.Server(ctx)
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
	clientcmd clientcmd.Client
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

	opts := common.Options{
		Client:           r.client,
		ClientCmd:        r.clientcmd,
		Scheme:           r.scheme,
		NewStorageClient: r.newStorageClientFunc,
		CR:               cr,
		Cluster:          cluster,
		Backup:           bcp,
	}

	backupType, err := resolveBackupType(ctx, opts)
	if err != nil {
		log.Info("Waiting to find out the type of the backup", "reason", err.Error())
		return rr, nil
	}
	cr.Status.BackupType = backupType

	if err := getRestoreController(backupType, opts).Restore(ctx); err != nil {
		return reconcile.Result{}, err
	}

	return rr, nil
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

// resolveBackupType returns the type of the backup the restore is for.
func resolveBackupType(ctx context.Context, opts common.Options) (api.PXCBackupType, error) {
	cr, bcp := opts.CR, opts.Backup

	if cr.Status.BackupType != "" {
		return cr.Status.BackupType, nil
	}

	// A restore that is past its first state with no recorded type was
	// started before the type existed, so it is a physical one.
	if cr.Status.State != api.RestoreStarting {
		return api.BackupTypeFull, nil
	}

	// A backup object records its type. One taken before types existed has
	// none, and is a physical backup.
	source := cr.Spec.BackupSource
	if source == nil {
		if bcp.Status.Type == "" {
			return api.BackupTypeFull, nil
		}
		return bcp.Status.Type, nil
	}

	// A restore from a backupSource can state the type, and then it is
	// trusted and the bucket is not looked at.
	if source.Type != "" {
		return source.Type, nil
	}

	// Without a stated type, only an s3 destination can hold a logical backup.
	if source.Destination.StorageTypePrefix() != api.AwsBlobStoragePrefix {
		return api.BackupTypeFull, nil
	}

	isLogical, err := logical.IsLogicalBackup(ctx, opts)
	if err != nil {
		return "", err
	}
	if isLogical {
		return api.BackupTypeLogical, nil
	}

	return api.BackupTypeFull, nil
}
