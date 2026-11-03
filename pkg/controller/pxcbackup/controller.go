package pxcbackup

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	k8sretry "k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/features"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/binlogcollector"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup"
)

// Add creates a new PerconaXtraDBClusterBackup Controller and adds it to the Manager. The Manager will set fields on the Controller
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
		return nil, errors.Wrap(err, "get version")
	}

	limit := 10

	envLimStr := os.Getenv("S3_WORKERS_LIMIT")
	if envLimStr != "" {
		envLim, err := strconv.Atoi(envLimStr)
		if err != nil || envLim <= 0 {
			return nil, errors.Wrapf(err, "invalid S3_WORKERS_LIMIT value (%s), should be positive int", envLimStr)
		}

		limit = envLim
	}

	cli, err := clientcmd.NewClient()
	if err != nil {
		return nil, errors.Wrap(err, "create clientcmd")
	}

	return &ReconcilePerconaXtraDBClusterBackup{
		client:              mgr.GetClient(),
		scheme:              mgr.GetScheme(),
		serverVersion:       sv,
		clientcmd:           cli,
		chLimit:             make(chan struct{}, limit),
		bcpDeleteInProgress: new(sync.Map),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	return builder.ControllerManagedBy(mgr).
		Named("pxcbackup-controller").
		For(&api.PerconaXtraDBClusterBackup{}).
		Complete(r)
}

var _ reconcile.Reconciler = &ReconcilePerconaXtraDBClusterBackup{}

// ReconcilePerconaXtraDBClusterBackup reconciles a PerconaXtraDBClusterBackup object
type ReconcilePerconaXtraDBClusterBackup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	serverVersion       *version.ServerVersion
	clientcmd           clientcmd.Client
	chLimit             chan struct{}
	bcpDeleteInProgress *sync.Map
}

// Reconcile reads that state of the cluster for a PerconaXtraDBClusterBackup object and makes changes based on the state read
// and what is in the PerconaXtraDBClusterBackup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBClusterBackup) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	rr := reconcile.Result{
		RequeueAfter: time.Second * 5,
	}

	// Fetch the PerconaXtraDBClusterBackup instance
	cr := &api.PerconaXtraDBClusterBackup{}
	err := r.client.Get(ctx, request.NamespacedName, cr)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	err = r.tryRunBackupFinalizers(ctx, cr)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "run finalizers")
	}

	if cr.Status.State == api.BackupSucceeded || cr.Status.State == api.BackupFailed {
		if err := r.runJobFinalizers(ctx, cr); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "run finalizers")
		}

		if len(cr.GetFinalizers()) > 0 {
			return rr, nil
		}

		return reconcile.Result{}, nil
	}

	if cr.DeletionTimestamp != nil {
		return rr, nil
	}

	cluster, err := r.getCluster(ctx, cr)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get cluster")
	}

	log = log.WithValues("cluster", cluster.Name)

	storage, ok := cluster.Spec.Backup.Storages[cr.Spec.StorageName]
	if !ok {
		err := errors.Errorf("storage %s doesn't exist", cr.Spec.StorageName)

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
	}

	// TODO: implement support
	if storage.Type == api.BackupStorageFilesystem && features.Enabled(ctx, features.XtrabackupSidecar) {
		err := fmt.Errorf("pvc backups are not supported when '%s' feature flag is enabled", features.XtrabackupSidecar)

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}
		return reconcile.Result{}, err
	}

	err = cluster.CheckNSetDefaults(r.serverVersion, log)
	if err != nil {
		err := errors.Wrap(err, "wrong PXC options")

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
	}

	if cluster.Spec.Backup == nil {
		err := errors.New("a backup image should be set in the PXC config")

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
	}

	if cr.Status.Type == "" {
		cr.Status.Type = api.BackupTypeFull
		if cr.Spec.Type != "" {
			cr.Status.Type = cr.Spec.Type
		}
	}

	if err := validateIncrementalBackup(ctx, cr, cluster, storage); err != nil {
		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
	}

	err = r.ensureFinalizers(ctx, cluster, cr)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "ensure finalizers")
	}

	// we need to defer this before checking deadlines
	// to properly release the lock if backup fails due to a deadline
	defer func() {
		if cluster.Spec.Backup.GetAllowParallel() {
			return
		}

		if !cr.Status.State.IsTerminal() && cr.DeletionTimestamp.IsZero() {
			return
		}

		log.Info("Releasing backup lock", "lease", naming.BackupLeaseName(cluster.Name), "namespace", cr.Namespace, "name", cr.Name)

		err := k8s.ReleaseLease(ctx, r.client, naming.BackupLeaseName(cluster.Name), cr.Namespace, naming.BackupHolderId(cr))
		if err != nil && !k8sErrors.IsNotFound(err) && !errors.Is(err, k8s.ErrNotTheHolder) {
			log.Error(err, "failed to release the lock")
		}
	}()

	if err := r.checkDeadlines(ctx, cluster, cr); err != nil {
		// There was an error with checking the deadline itself, we must retry.
		if !errors.Is(err, errDeadlineExceeded) {
			return reconcile.Result{}, errors.Wrap(err, "check deadlines")
		}
		// If a deadline was exceeded, we must set the status to failed so the backup can stop.
		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "update status")
		}

		if errors.Is(err, errSuspendedDeadlineExceeded) {
			log.Info("cleaning up suspended backup job")
			if err := r.cleanUpJob(ctx, cluster, cr); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "clean up suspended job")
			}
		}

		if errors.Is(err, errRunningDeadlineExceeded) {
			log.Info("running deadline exceeded, deleting the job and its pods")
			if err := r.cleanUpJob(ctx, cluster, cr); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "clean up running deadline exceeded job")
			}
		}
		return reconcile.Result{}, nil
	}

	if err := r.reconcileBackupJob(ctx, cr, cluster); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "reconcile backup job")
	}

	if err := cluster.CanBackup(); err != nil {
		log.Info("Cluster is not ready for backup", "reason", err.Error())

		return rr, nil
	}

	log = log.WithValues("storage", cr.Spec.StorageName)

	log.V(1).Info("Check if parallel backups are allowed", "allowed", cluster.Spec.Backup.GetAllowParallel())
	if cr.Status.State == api.BackupNew && !cluster.Spec.Backup.GetAllowParallel() {
		lease, err := k8s.AcquireLease(ctx, r.client, naming.BackupLeaseName(cluster.Name), cr.Namespace, naming.BackupHolderId(cr))
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "acquire backup lock")
		}

		if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != naming.BackupHolderId(cr) {
			log.Info("Another backup is holding the lock", "holder", *lease.Spec.HolderIdentity)

			return rr, nil
		}
	}

	if cr.Status.S3 == nil || cr.Status.Azure == nil {
		cr.Status.S3 = storage.S3
		cr.Status.Azure = storage.Azure
		cr.Status.StorageType = storage.Type
		cr.Status.Image = cluster.Spec.Backup.Image
		cr.Status.SSLSecretName = cluster.Spec.PXC.SSLSecretName
		cr.Status.SSLInternalSecretName = cluster.Spec.PXC.SSLInternalSecretName
		cr.Status.VaultSecretName = cluster.Spec.PXC.VaultSecretName
		cr.Status.VerifyTLS = storage.VerifyTLS
	}

	if err := r.setIncrementalBaseAnnotation(ctx, cr, cluster, storage); err != nil {
		err = errors.Wrap(err, "set incremental base backup annotation")

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
	}

	var job *batchv1.Job
	job, err = r.createBackupJob(ctx, cr, cluster, storage)
	if err != nil {
		err = errors.Wrap(err, "create backup job")

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
	}

	if err := r.updateJobStatus(ctx, cr, job, cr.Spec.StorageName, storage, cluster); err != nil {
		return reconcile.Result{}, err
	}

	return rr, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) createBackupJob(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
	storage *api.BackupStorageSpec,
) (*batchv1.Job, error) {
	log := logf.FromContext(ctx)

	bcp := backup.New(cluster)
	job := bcp.Job(cr, cluster)
	initImage, err := k8s.GetInitImage(ctx, cluster, r.client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initImage")
	}

	var pvc *corev1.PersistentVolumeClaim
	if storage.Type == api.BackupStorageFilesystem {
		pvc = backup.NewPVC(cr, cluster)
		pvc.Spec = *storage.Volume.PersistentVolumeClaim
	}

	if cr.Status.Destination == "" {
		if err := setBackupDestination(cr, storage, pvc); err != nil {
			return nil, errors.Wrap(err, "set backup destination")
		}
	}

	incrementalBaseDest, err := r.incrementalPredecessorDestination(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "resolve the predecessor of the incremental backup")
	}

	xtrabackupEnabled := features.Enabled(ctx, features.XtrabackupSidecar)
	getJobSpec := func() (batchv1.JobSpec, error) {
		if xtrabackupEnabled {
			srcPod, host, err := backup.GetHostForSidecarBackup(ctx, r.client, cluster, cr)
			if err != nil {
				return batchv1.JobSpec{}, errors.Wrap(err, "failed to get primary pod dns name")
			}

			if cr.Status.SourcePod == "" {
				cr.Status.SourcePod = srcPod
			}

			return xtrabackup.JobSpec(cr, cluster, job, initImage, host, incrementalBaseDest)
		}
		return bcp.JobSpec(cr.Spec, cluster, job, initImage)
	}

	job.Spec, err = getJobSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to get job spec: %w (xtrabackup enabled: %t)", err, xtrabackupEnabled)
	}

	switch storage.Type {
	case api.BackupStorageFilesystem:
		// Check if this PVC already exists
		err = r.client.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, pvc)
		if err != nil && k8sErrors.IsNotFound(err) {
			log.Info("Creating a new volume for backup", "Namespace", pvc.Namespace, "Name", pvc.Name)
			err = r.client.Create(ctx, pvc)
			if err != nil {
				return nil, errors.Wrap(err, "create backup pvc")
			}
		} else if err != nil {
			return nil, errors.Wrap(err, "get backup pvc")
		}

		err := backup.SetStoragePVC(ctx, &job.Spec, cr, pvc.Name)
		if err != nil {
			return nil, errors.Wrap(err, "set storage FS")
		}

		cr.Status.SetFsPvcFromPVC(pvc)

	case api.BackupStorageS3:
		if storage.S3 == nil {
			return nil, errors.New("s3 storage is not specified")
		}

		if err := backup.SetStorageS3(ctx, &job.Spec, cr); err != nil {
			return nil, errors.Wrap(err, "set storage FS")
		}
	case api.BackupStorageAzure:
		if storage.Azure == nil {
			return nil, errors.New("azure storage is not specified")
		}

		err := backup.SetStorageAzure(ctx, &job.Spec, cr)
		if err != nil {
			return nil, errors.Wrap(err, "set storage FS for Azure")
		}
	}

	if xtrabackupEnabled {
		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{
			Name:  "BACKUP_DEST",
			Value: cr.Status.Destination.PathWithoutBucket(),
		})
	}

	// Set PerconaXtraDBClusterBackup instance as the owner and controller
	if err := k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return nil, errors.Wrap(err, "job/setControllerReference")
	}

	if err := r.client.Create(ctx, job); err != nil {
		if k8sErrors.IsAlreadyExists(err) {
			if err := r.client.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
				return nil, errors.Wrap(err, "get backup job")
			}
			return job, nil
		}
		return nil, errors.Wrap(err, "create backup job")
	}
	log.Info("Created a new backup job", "namespace", job.Namespace, "name", job.Name)

	return job, nil
}

func validateIncrementalBackup(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
	storage *api.BackupStorageSpec,
) error {
	if cr.GetType() != api.BackupTypeIncremental {
		return nil
	}

	if cluster.CompareVersionWith("1.21.0") < 0 {
		return errors.Errorf("incremental backups require crVersion 1.21.0 or newer, this cluster is on %s", cluster.Spec.CRVersion)
	}

	if !features.Enabled(ctx, features.XtrabackupSidecar) {
		return errors.Errorf("incremental backups require the '%s' feature gate to be enabled", features.XtrabackupSidecar)
	}

	switch storage.Type {
	case api.BackupStorageS3, api.BackupStorageAzure:
	default:
		return errors.Errorf("incremental backups are not supported on storage %q of type %q, only s3 and azure storages can hold a backup chain",
			cr.Spec.StorageName, storage.Type)
	}

	if cluster.Spec.Backup.GetAllowParallel() {
		return errors.New("incremental backups require spec.backup.allowParallel to be false: " +
			"allowParallel skips the per-cluster backup lease, so concurrent backups all start from the same LSN and produce a chain that cannot be prepared")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) setIncrementalBaseAnnotation(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
	storage *api.BackupStorageSpec,
) error {
	if cr.GetType() != api.BackupTypeIncremental {
		return nil
	}

	if _, ok := cr.GetAnnotations()[naming.AnnotationBaseBackupName]; ok {
		return nil
	}

	base, err := backup.ResolveIncrementalBase(ctx, r.client, cr, cluster, storage)
	if err != nil {
		return errors.Wrap(err, "resolve incremental base backup")
	}

	if cr.Spec.SourcePod == "" && base.Status.SourcePod == "" {
		return errors.Errorf("base backup %s/%s source cannot be known, specify spec.sourcePod",
			base.GetNamespace(), base.GetName())
	}

	// Incrementals need to be pinned to the same source
	if base.Status.SourcePod != "" {
		cr.Spec.SourcePod = base.Status.SourcePod
	}

	baseName := base.Status.Destination.BackupName()
	if baseName == "" {
		return errors.Errorf("base backup %s/%s has no destination recorded", base.GetNamespace(), base.GetName())
	}

	if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		updated := new(api.PerconaXtraDBClusterBackup)
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(cr), updated); err != nil {
			return errors.Wrap(err, "get backup")
		}

		annotations := updated.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}

		if _, ok := annotations[naming.AnnotationBaseBackupName]; !ok {
			annotations[naming.AnnotationBaseBackupName] = baseName
			updated.SetAnnotations(annotations)

			if err := r.client.Update(ctx, updated); err != nil {
				return err
			}
		}

		cr.ObjectMeta = *updated.ObjectMeta.DeepCopy()
		return nil
	}); err != nil {
		return errors.Wrap(err, "update backup annotations")
	}

	logf.FromContext(ctx).Info("Resolved the base of the incremental backup",
		"namespace", cr.Namespace, "name", cr.Name, "baseBackup", base.GetName(), "baseDestination", baseName)

	return nil
}

// incrementalPredecessorDestination returns the bucket-relative destination of
// the backup cr is a delta against, or "" when cr is a full backup.
func (r *ReconcilePerconaXtraDBClusterBackup) incrementalPredecessorDestination(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
) (string, error) {
	if cr.GetType() != api.BackupTypeIncremental {
		return "", nil
	}

	if cr.Status.Destination == "" {
		return "", errors.Errorf("incremental backup %s/%s has no destination recorded", cr.GetNamespace(), cr.GetName())
	}

	baseDest := cr.Status.Destination.IncrementalBaseDestination()
	predecessor := baseDest.PathWithoutBucket()

	last, err := backup.GetLatestIncrementalBackupInChain(ctx, r.client, cr, true)
	switch {
	case err == nil:
		if last.Status.Destination == "" {
			return "", errors.Errorf("backup %s/%s succeeded but has no destination recorded", last.GetNamespace(), last.GetName())
		}
		predecessor = last.Status.Destination.PathWithoutBucket()
	case errors.Is(err, backup.ErrNoIncrementalBackupFound):
		// First incremental of the chain: it chains onto the base itself.
	default:
		return "", errors.Wrap(err, "get latest incremental backup in chain")
	}

	return predecessor, nil
}

func setBackupDestination(cr *api.PerconaXtraDBClusterBackup, storage *api.BackupStorageSpec, pvc *corev1.PersistentVolumeClaim) error {
	switch storage.Type {
	case api.BackupStorageFilesystem:
		if pvc == nil {
			return errors.New("no pvc given for a filesystem backup")
		}
		cr.Status.Destination.SetPVCDestination(pvc.Name)
	case api.BackupStorageS3:
		if storage.S3 == nil {
			return errors.New("s3 storage is not specified")
		}
		bucket, prefix, err := storage.S3.BucketAndPrefix()
		if err != nil {
			return errors.Wrap(err, "failed to get bucket and prefix")
		}
		leaf, err := backupLeafName(cr)
		if err != nil {
			return err
		}
		cr.Status.Destination.SetS3Destination(path.Join(bucket, prefix), leaf)
	case api.BackupStorageAzure:
		if storage.Azure == nil {
			return errors.New("azure storage is not specified")
		}
		container, prefix := storage.Azure.ContainerAndPrefix()
		leaf, err := backupLeafName(cr)
		if err != nil {
			return err
		}
		cr.Status.Destination.SetAzureDestination(path.Join(container, prefix), leaf)
	}

	return nil
}

// backupLeafName returns the object-storage path of a backup relative to the
// bucket and prefix of its storage.
//
// A full backup keeps the layout it has always had, "<cluster>-<ts>-full"
// directly under the storage prefix. An incremental backup nests in a sibling
// directory of its base, "<base-leaf>.incr/<cluster>-<ts>-incr", so that one
// chain is one directory and any member's path names its base.
func backupLeafName(cr *api.PerconaXtraDBClusterBackup) (string, error) {
	timestamp := cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05")

	if cr.GetType() != api.BackupTypeIncremental {
		return cr.Spec.PXCCluster + "-" + timestamp + "-full", nil
	}

	base := cr.GetAnnotations()[naming.AnnotationBaseBackupName]
	if base == "" {
		// A programming error: the annotation is persisted before the
		// destination is computed, in the same reconcile.
		return "", errors.Errorf("incremental backup %s/%s has no %s annotation, its base must be resolved before its destination is computed",
			cr.GetNamespace(), cr.GetName(), naming.AnnotationBaseBackupName)
	}

	return path.Join(base+".incr", cr.Spec.PXCCluster+"-"+timestamp+"-incr"), nil
}

// canDeleteIncrementalBackup reports whether cr may have its objects removed
// from storage now. Full backups always may; an incremental may only when it is
// the tail of its chain.
func (r *ReconcilePerconaXtraDBClusterBackup) canDeleteIncrementalBackup(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
) (bool, error) {
	if cr.GetType() != api.BackupTypeIncremental {
		return true, nil
	}

	tail, err := backup.GetLatestIncrementalBackupInChain(ctx, r.client, cr, false)
	if err != nil {
		if errors.Is(err, backup.ErrNoIncrementalBackupFound) {
			// Nothing is chained here at all, so nothing can depend on it.
			return true, nil
		}
		return false, errors.Wrap(err, "get latest incremental backup in chain")
	}

	return tail.GetName() == cr.GetName(), nil
}

// deleteDependentIncrementalBackups cascades the deletion of a full backup to
// the incremental backups chained to it.
func (r *ReconcilePerconaXtraDBClusterBackup) deleteDependentIncrementalBackups(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
) (bool, error) {
	if cr.GetType() != api.BackupTypeFull {
		return true, nil
	}

	dependents, err := backup.ListDependentIncrementalBackups(ctx, r.client, cr)
	if err != nil {
		return false, errors.Wrap(err, "list dependent incremental backups")
	}
	if len(dependents) == 0 {
		return true, nil
	}

	newest := dependents[len(dependents)-1]
	if newest.GetDeletionTimestamp() != nil {
		// Already on its way out; wait for it instead of issuing a second delete.
		return false, nil
	}

	logf.FromContext(ctx).Info("Deleting an incremental backup that depends on this backup",
		"namespace", newest.GetNamespace(), "name", newest.GetName(), "base", cr.GetName())

	if err := r.client.Delete(ctx, &newest); client.IgnoreNotFound(err) != nil {
		return false, errors.Wrapf(err, "delete dependent incremental backup %s/%s", newest.GetNamespace(), newest.GetName())
	}

	return false, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) ensureFinalizers(ctx context.Context, cluster *api.PerconaXtraDBCluster, cr *api.PerconaXtraDBClusterBackup) error {
	if cluster.Spec.Backup.GetAllowParallel() {
		return nil
	}

	if slices.Contains(cr.GetFinalizers(), naming.FinalizerReleaseLock) {
		return nil
	}

	orig := cr.DeepCopy()
	cr.SetFinalizers(append(cr.GetFinalizers(), naming.FinalizerReleaseLock))
	if err := r.client.Patch(ctx, cr.DeepCopy(), client.MergeFrom(orig)); err != nil {
		return errors.Wrap(err, "patch finalizers")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) tryRunBackupFinalizers(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	if cr.ObjectMeta.DeletionTimestamp == nil {
		return nil
	}

	select {
	case r.chLimit <- struct{}{}:
		_, ok := r.bcpDeleteInProgress.LoadOrStore(cr.Name, struct{}{})
		if ok {
			<-r.chLimit
			return nil
		}

		go r.runBackupFinalizers(ctx, cr)
	default:
		if _, ok := r.bcpDeleteInProgress.Load(cr.Name); !ok {
			inprog := []string{}
			r.bcpDeleteInProgress.Range(func(key, value any) bool {
				inprog = append(inprog, key.(string))
				return true
			})

			logf.FromContext(ctx).Info("all workers are busy - skip backup deletion for now",
				"backup", cr.Name, "in progress", strings.Join(inprog, ", "))
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runBackupFinalizers(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) {
	log := logf.FromContext(ctx)

	defer func() {
		r.bcpDeleteInProgress.Delete(cr.Name)
		<-r.chLimit
	}()

	var finalizers []string
	for _, f := range cr.GetFinalizers() {
		var err error
		switch f {
		case naming.FinalizerDeleteBackup:
			if (cr.Status.S3 == nil && cr.Status.Azure == nil && cr.Status.PVC == nil) || cr.Status.Destination == "" {
				continue
			}

			if can, err := r.canDeleteIncrementalBackup(ctx, cr); err != nil {
				log.Error(err, "failed to check the position of the backup in its chain", "namespace", cr.Namespace, "name", cr.Name)
				finalizers = append(finalizers, f)
				continue
			} else if !can {
				log.Info("Cannot delete incremental backup which is in middle of the chain",
					"namespace", cr.Namespace, "name", cr.Name)
				finalizers = append(finalizers, f)
				continue
			}

			if done, err := r.deleteDependentIncrementalBackups(ctx, cr); err != nil {
				log.Error(err, "failed to delete dependent incremental backups", "namespace", cr.Namespace, "name", cr.Name)
				finalizers = append(finalizers, f)
				continue
			} else if !done {
				log.Info("Awaiting deletion of dependent incremental backups",
					"namespace", cr.Namespace, "name", cr.Name)
				finalizers = append(finalizers, f)
				continue
			}

			switch cr.Status.GetStorageType(nil) {
			case api.BackupStorageS3:
				if cr.Status.Destination.StorageTypePrefix() != api.AwsBlobStoragePrefix {
					continue
				}
				err = r.runS3BackupFinalizer(ctx, cr)
			case api.BackupStorageAzure:
				err = r.runAzureBackupFinalizer(ctx, cr)
			case api.BackupStorageFilesystem:
				err = r.runFilesystemBackupFinalizer(ctx, cr)
			default:
				continue
			}

			if err != nil {
				log.Info("failed to delete backup", "backup path", cr.Status.Destination, "error", err.Error())
				finalizers = append(finalizers, f)
				continue
			}

			log.Info("backup was removed", "name", cr.Name)
		case naming.FinalizerReleaseLock:
			err = r.runReleaseLockFinalizer(ctx, cr)
			if err != nil {
				log.Error(err, "failed to release backup lock")
				finalizers = append(finalizers, f)
			}
		default:
			finalizers = append(finalizers, f)
		}

	}
	cr.SetFinalizers(finalizers)

	err := r.client.Update(ctx, cr)
	if err != nil {
		log.Error(err, "failed to update finalizers for backup", "backup", cr.Name)
	}
}

func (r *ReconcilePerconaXtraDBClusterBackup) runS3BackupFinalizer(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	log := logf.FromContext(ctx)

	if cr.Status.S3 == nil {
		return errors.New("s3 storage is not specified")
	}

	sec := corev1.Secret{}
	err := r.client.Get(ctx,
		types.NamespacedName{Name: cr.Status.S3.CredentialsSecret, Namespace: cr.Namespace}, &sec)
	if err != nil {
		return errors.Wrap(err, "failed to get secret")
	}

	opts, err := storage.GetOptionsFromBackup(ctx, r.client, nil, cr)
	if err != nil {
		return errors.Wrap(err, "get storage options")
	}
	storage, err := storage.NewClient(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "new s3 storage")
	}

	backupName := backupObjectPrefix(&cr.Status.Destination, storage.GetPrefix())
	log.Info("deleting backup from s3", "name", cr.Name, "bucket", cr.Status.S3.Bucket, "backupName", backupName)
	err = retry.OnError(retry.DefaultBackoff, func(e error) bool { return true }, removeBackupObjects(ctx, storage, backupName))
	if err != nil {
		return errors.Wrapf(err, "failed to delete backup %s", cr.Name)
	}
	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runAzureBackupFinalizer(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	log := logf.FromContext(ctx)

	if cr.Status.Azure == nil {
		return errors.New("azure storage is not specified")
	}

	opts, err := storage.GetOptionsFromBackup(ctx, r.client, nil, cr)
	if err != nil {
		return errors.Wrap(err, "get storage options")
	}
	azureStorage, err := storage.NewClient(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "new azure storage")
	}

	backupName := backupObjectPrefix(&cr.Status.Destination, azureStorage.GetPrefix())
	log.Info("Deleting backup from azure", "name", cr.Name, "backupName", backupName)
	err = retry.OnError(retry.DefaultBackoff,
		func(e error) bool {
			return true
		},
		removeBackupObjects(ctx, azureStorage, backupName))
	if err != nil {
		return errors.Wrapf(err, "failed to delete backup %s", cr.Name)
	}
	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runFilesystemBackupFinalizer(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	log := logf.FromContext(ctx)

	backupName := cr.Status.Destination.BackupName()
	log.Info("Deleting backup from fs-pvc", "name", cr.Name, "backupName", backupName)
	err := r.deleteBackupPVC(ctx, cr.Namespace, backupName)
	if err != nil {
		return errors.Wrapf(err, "failed to delete backup %s", cr.Name)
	}
	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runReleaseLockFinalizer(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	err := k8s.ReleaseLease(ctx, r.client, naming.BackupLeaseName(cr.Spec.PXCCluster), cr.Namespace, naming.BackupHolderId(cr))
	if k8sErrors.IsNotFound(err) || errors.Is(err, k8s.ErrNotTheHolder) {
		return nil
	}
	return errors.Wrap(err, "release backup lock")
}

func removeBackupObjects(ctx context.Context, s storage.Storage, destination string) func() error {
	return func() error {
		destination = strings.TrimSuffix(destination, "/")

		if err := deleteObjectsWithPrefix(ctx, s, destination+"/"); err != nil {
			return errors.Wrap(err, "delete backup blobs")
		}

		siblings := []string{
			destination + ".md5",
			destination + ".meta.json",
			destination + "." + naming.PITRNotReady,
		}
		for _, object := range siblings {
			if err := s.DeleteObject(ctx, object); err != nil && err != storage.ErrObjectNotFound {
				return errors.Wrapf(err, "delete object %s", object)
			}
		}

		sstInfo := destination + ".sst_info"
		if err := deleteObjectsWithPrefix(ctx, s, sstInfo+"/"); err != nil {
			return errors.Wrap(err, "delete backup objects")
		}
		if err := s.DeleteObject(ctx, sstInfo+".md5"); err != nil && err != storage.ErrObjectNotFound {
			return errors.Wrapf(err, "delete object %s", sstInfo+".md5")
		}

		return nil
	}
}

// deleteObjectsWithPrefix removes every object the storage lists under prefix.
func deleteObjectsWithPrefix(ctx context.Context, s storage.Storage, prefix string) error {
	blobs, err := s.ListObjects(ctx, prefix)
	if err != nil {
		return errors.Wrapf(err, "list objects with prefix %s", prefix)
	}

	for _, blob := range blobs {
		if err := s.DeleteObject(ctx, blob); err != nil {
			return errors.Wrapf(err, "delete object %s", blob)
		}
	}

	return nil
}

// backupObjectPrefix returns where a backup's objects live relative to the
// storage client's own prefix, which is what ListObjects and DeleteObject take.
func backupObjectPrefix(dest *api.PXCBackupDestination, clientPrefix string) string {
	if !dest.IsIncremental() {
		return dest.BackupName()
	}

	return strings.TrimPrefix(strings.TrimPrefix(dest.PathWithoutBucket(), clientPrefix), "/")
}

func (r *ReconcilePerconaXtraDBClusterBackup) getCluster(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) (*api.PerconaXtraDBCluster, error) {
	cluster := api.PerconaXtraDBCluster{}
	err := r.client.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cr.Spec.PXCCluster}, &cluster)
	if err != nil {
		return nil, errors.Wrap(err, "get PXC cluster")
	}

	return &cluster, nil
}

func getPXCBackupStateFromJob(job *batchv1.Job) api.PXCBackupState {
	for _, cond := range job.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}
		switch cond.Type {
		case batchv1.JobFailed:
			return api.BackupFailed
		case batchv1.JobComplete:
			return api.BackupSucceeded
		}
	}
	readyCount := ptr.Deref(job.Status.Ready, 0)
	failedCount := job.Status.Failed
	if readyCount > 0 || failedCount > 0 {
		return api.BackupRunning
	}
	return api.BackupStarting
}

func (r *ReconcilePerconaXtraDBClusterBackup) checkJobPodsRunning(
	ctx context.Context,
	job *batchv1.Job,
) (bool, error) {
	pods := corev1.PodList{}
	err := r.client.List(ctx, &pods, client.InNamespace(job.GetNamespace()), client.MatchingLabels(job.GetLabels()))
	if err != nil {
		return false, errors.Wrap(err, "list job pods")
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return true, nil
		}
	}
	return false, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) updateJobStatus(
	ctx context.Context,
	bcp *api.PerconaXtraDBClusterBackup,
	job *batchv1.Job,
	storageName string,
	storage *api.BackupStorageSpec,
	cluster *api.PerconaXtraDBCluster,
) error {
	log := logf.FromContext(ctx).WithValues("job", job.Name)

	err := r.client.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, job)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}

		return errors.Wrap(err, "get backup status")
	}

	status := api.PXCBackupStatus{
		Type:                  bcp.Status.Type,
		State:                 getPXCBackupStateFromJob(job),
		Destination:           bcp.Status.Destination,
		StorageName:           storageName,
		S3:                    storage.S3,
		Azure:                 storage.Azure,
		PVC:                   bcp.Status.PVC,
		StorageType:           storage.Type,
		Image:                 bcp.Status.Image,
		SSLSecretName:         bcp.Status.SSLSecretName,
		SSLInternalSecretName: bcp.Status.SSLInternalSecretName,
		VaultSecretName:       bcp.Status.VaultSecretName,
		VerifyTLS:             storage.VerifyTLS,
		SourcePod:             bcp.Status.SourcePod,
	}

	if status.State == api.BackupSucceeded {
		status.CompletedAt = job.Status.CompletionTime
	}

	// There is a bug in k8s 1.33.2 (possibly some earlier versions as well), where a suspended job after being un-suspended
	// contains stale status. I.e, the job status still reflects the suspended state even though the job pods are running.
	// As a consequence, the backup state may transition from Starting -> Succeeded without going through the Running state.
	//
	// As a workaround, when the computed state is `Starting`, we shall double check if there are any running pods.
	// See: https://perconadev.atlassian.net/browse/K8SPXC-1772
	//
	// This was fixed in k8s 1.33.6. See: https://github.com/kubernetes/kubernetes/pull/135129
	if status.State == api.BackupStarting {
		running, err := r.checkJobPodsRunning(ctx, job)
		if err != nil {
			return errors.Wrap(err, "check job pods running")
		}
		if running {
			status.State = api.BackupRunning
		}
	}

	// don't update the status if there aren't any changes.
	if reflect.DeepEqual(bcp.Status, status) {
		return nil
	}

	bcp.Status = status

	switch status.State {
	case api.BackupSucceeded:
		log.Info("Backup succeeded")

		if cluster.PITREnabled() {
			collectorPod, err := binlogcollector.GetPod(ctx, r.client, cluster)
			if err != nil {
				return errors.Wrap(err, "get binlog collector pod")
			}

			log.V(1).Info("Removing binlog gap file from binlog collector", "pod", collectorPod.Name)
			if err := binlogcollector.RemoveGapFile(r.clientcmd, collectorPod); err != nil {
				if !errors.Is(err, binlogcollector.GapFileNotFound) {
					return errors.Wrap(err, "remove gap file")
				}
			}

			log.V(1).Info("Removing binlog timeline file from binlog collector", "pod", collectorPod.Name)
			if err := binlogcollector.RemoveTimelineFile(r.clientcmd, collectorPod); err != nil {
				return errors.Wrap(err, "remove timeline file")
			}
		}

		initSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name + "-mysql-init",
				Namespace: cluster.Namespace,
			},
		}
		log.V(1).Info("Removing mysql-init secret", "secret", initSecret.Name)
		if err := r.client.Delete(ctx, &initSecret); client.IgnoreNotFound(err) != nil {
			return errors.Wrap(err, "delete mysql-init secret")
		}
	case api.BackupFailed:
		log.Info("Backup failed")
	}

	if err := r.updateStatus(ctx, bcp); err != nil {
		return errors.Wrap(err, "update status")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) updateStatus(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		localCr := new(api.PerconaXtraDBClusterBackup)
		err := r.client.Get(ctx, client.ObjectKeyFromObject(cr), localCr)
		if err != nil {
			return err
		}

		localCr.Status = cr.Status

		return r.client.Status().Update(ctx, localCr)
	})
}

func (r *ReconcilePerconaXtraDBClusterBackup) setFailedStatus(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
	err error,
) error {
	cr.SetFailedStatusWithError(err)
	return r.updateStatus(ctx, cr)
}

func (r *ReconcilePerconaXtraDBClusterBackup) suspendJobIfNeeded(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
) error {
	if cluster.Spec.Unsafe.BackupIfUnhealthy {
		return nil
	}

	if cluster.Status.Status == api.AppStateReady {
		return nil
	}

	if cluster.Status.PXC.Ready == cluster.Status.PXC.Size {
		return nil
	}

	log := logf.FromContext(ctx)

	job, err := r.getBackupJob(ctx, cr)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	for _, cond := range job.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}

		switch cond.Type {
		case batchv1.JobSuspended, batchv1.JobComplete:
			return nil
		}
	}

	if suspendedSpec := job.Spec.Suspend; suspendedSpec != nil && *suspendedSpec {
		return nil // already suspended
	}

	log.Info("Suspending backup job",
		"job", job.Name,
		"clusterStatus", cluster.Status.Status,
		"readyPXC", cluster.Status.PXC.Ready)

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest job to avoid conflicting updates
		job, err := r.getBackupJob(ctx, cr)
		if err != nil {
			return err
		}

		job.Spec.Suspend = new(true)
		err = r.client.Update(ctx, job)
		if err != nil {
			return err
		}

		cr.Status.State = api.BackupSuspended
		return r.updateStatus(ctx, cr)
	})

	return err
}

func (r *ReconcilePerconaXtraDBClusterBackup) resumeJobIfNeeded(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
) error {
	if cluster.Status.Status != api.AppStateReady {
		return nil
	}

	if cluster.Status.PXC.Ready != cluster.Status.PXC.Size {
		return nil
	}

	log := logf.FromContext(ctx)

	job, err := r.getBackupJob(ctx, cr)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	suspended := false
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobSuspended && cond.Status == corev1.ConditionTrue {
			suspended = true
		}
	}

	if suspendedSpec := job.Spec.Suspend; suspendedSpec != nil && !*suspendedSpec {
		return nil // already resumed
	}

	if !suspended {
		return nil
	}

	log.Info("Resuming backup job",
		"job", job.Name,
		"clusterStatus", cluster.Status.Status,
		"readyPXC", cluster.Status.PXC.Ready)

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest job to avoid conflicting updates
		job, err := r.getBackupJob(ctx, cr)
		if err != nil {
			return err
		}

		job.Spec.Suspend = new(false)
		err = r.client.Update(ctx, job)
		if err != nil {
			return err
		}

		cr.Status.State = api.BackupStarting
		return r.updateStatus(ctx, cr)
	})

	return err
}

func (r *ReconcilePerconaXtraDBClusterBackup) reconcileBackupJob(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
) error {
	if err := r.suspendJobIfNeeded(ctx, cr, cluster); err != nil {
		return errors.Wrap(err, "suspend job if needed")
	}

	if err := r.resumeJobIfNeeded(ctx, cr, cluster); err != nil {
		return errors.Wrap(err, "resume job if needed")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) getBackupJob(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterBackup,
) (*batchv1.Job, error) {
	jobName := naming.BackupJobName(cr.Name)

	job := new(batchv1.Job)

	if err := r.client.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: jobName}, job); err != nil {
		return nil, err
	}

	return job, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) cleanUpJob(
	ctx context.Context,
	cluster *api.PerconaXtraDBCluster,
	cr *api.PerconaXtraDBClusterBackup,
) error {
	job, err := r.getBackupJob(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "get job")
	}

	if err := r.runJobFinalizers(ctx, cr); err != nil {
		return errors.Wrap(err, "run job finalizers")
	}

	// Set propagationPolicy=background because default deletion preserves pods.
	if err := r.client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		return errors.Wrap(err, "delete job")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runJobFinalizers(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		job, err := r.getBackupJob(ctx, cr)
		if k8sErrors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return errors.Wrap(err, "failed to get job")
		}

		if removed := controllerutil.RemoveFinalizer(job, naming.FinalizerKeepJob); removed {
			return r.client.Update(ctx, job)
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to remove keep-job finalizer")
	}
	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) deleteBackupPVC(ctx context.Context, backupNamespace, backupPVCName string) error {
	log := logf.FromContext(ctx)
	log.Info("Deleting backup PVC", "namespace", backupNamespace, "pvc", backupPVCName)

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.client.Get(ctx, types.NamespacedName{Name: backupPVCName, Namespace: backupNamespace}, pvc)
	if err != nil {
		return errors.Wrap(err, "get backup PVC by name")
	}

	err = r.client.Delete(ctx, pvc, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pvc.UID}})
	if err != nil {
		return errors.Wrapf(err, "delete backup PVC %s", pvc.Name)
	}

	log.Info("Deleted backup PVC", "namespace", backupNamespace, "pvc", backupPVCName)
	return nil
}
