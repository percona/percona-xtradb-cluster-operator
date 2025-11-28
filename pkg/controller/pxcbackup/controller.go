package pxcbackup

import (
	"context"
	"os"
	"reflect"
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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// Add creates a new PerconaXtraDBClusterBackup Controller and adds it to the Manager. The Manager will set fields on the Controller
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
	clientcmd           *clientcmd.Client
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

		switch cr.Status.State {
		case api.BackupSucceeded, api.BackupFailed:
			log.Info("Releasing backup lock", "lease", naming.BackupLeaseName(cluster.Name))

			err := k8s.ReleaseLease(ctx, r.client, naming.BackupLeaseName(cluster.Name), cr.Namespace, naming.BackupHolderId(cr))
			if err != nil {
				log.Error(err, "failed to release the lock")
			}
		}
	}()

	if err := r.checkDeadlines(ctx, cluster, cr); err != nil {
		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}

		if errors.Is(err, errSuspendedDeadlineExceeded) {
			log.Info("cleaning up suspended backup job")
			if err := r.cleanUpSuspendedJob(ctx, cluster, cr); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "clean up suspended job")
			}
		}

		return reconcile.Result{}, nil
	}

	if err := r.reconcileBackupJob(ctx, cr, cluster); err != nil {
		return rr, errors.Wrap(err, "reconcile backup job")
	}

	if err := cluster.CanBackup(); err != nil {
		log.Info("Cluster is not ready for backup", "reason", err.Error())

		return rr, nil
	}

	storage, ok := cluster.Spec.Backup.Storages[cr.Spec.StorageName]
	if !ok {
		err := errors.Errorf("storage %s doesn't exist", cr.Spec.StorageName)

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
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

	job, err := r.createBackupJob(ctx, cr, cluster, storage)
	if err != nil {
		err = errors.Wrap(err, "create backup job")

		if err := r.setFailedStatus(ctx, cr, err); err != nil {
			return rr, errors.Wrap(err, "update status")
		}

		return reconcile.Result{}, err
	}

	err = r.updateJobStatus(ctx, cr, job, cr.Spec.StorageName, storage, cluster)

	return rr, err
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
	job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, initImage)
	if err != nil {
		return nil, errors.Wrap(err, "can't create job spec")
	}

	switch storage.Type {
	case api.BackupStorageFilesystem:
		pvc := backup.NewPVC(cr, cluster)
		pvc.Spec = *storage.Volume.PersistentVolumeClaim

		cr.Status.Destination.SetPVCDestination(pvc.Name)

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

		err := backup.SetStoragePVC(&job.Spec, cr, pvc.Name)
		if err != nil {
			return nil, errors.Wrap(err, "set storage FS")
		}

		cr.Status.SetFsPvcFromPVC(pvc)

	case api.BackupStorageS3:
		if storage.S3 == nil {
			return nil, errors.New("s3 storage is not specified")
		}
		cr.Status.Destination.SetS3Destination(storage.S3.Bucket, cr.Spec.PXCCluster+"-"+cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05")+"-full")

		err := backup.SetStorageS3(&job.Spec, cr)
		if err != nil {
			return nil, errors.Wrap(err, "set storage FS")
		}
	case api.BackupStorageAzure:
		if storage.Azure == nil {
			return nil, errors.New("azure storage is not specified")
		}
		cr.Status.Destination.SetAzureDestination(storage.Azure.ContainerPath, cr.Spec.PXCCluster+"-"+cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05")+"-full")

		err := backup.SetStorageAzure(&job.Spec, cr)
		if err != nil {
			return nil, errors.Wrap(err, "set storage FS for Azure")
		}
	}

	// Set PerconaXtraDBClusterBackup instance as the owner and controller
	if err := k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return nil, errors.Wrap(err, "job/setControllerReference")
	}

	err = r.client.Create(ctx, job)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return nil, errors.Wrap(err, "create backup job")
	} else if err == nil {
		log.Info("Created a new backup job", "namespace", job.Namespace, "name", job.Name)
	}

	return job, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) ensureFinalizers(ctx context.Context, cluster *api.PerconaXtraDBCluster, cr *api.PerconaXtraDBClusterBackup) error {
	if cluster.Spec.Backup.GetAllowParallel() {
		return nil
	}

	for _, f := range cr.GetFinalizers() {
		if f == naming.FinalizerReleaseLock {
			return nil
		}
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
			r.bcpDeleteInProgress.Range(func(key, value interface{}) bool {
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

	backupName := cr.Status.Destination.BackupName()
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

	backupName := cr.Status.Destination.BackupName()
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
		blobs, err := s.ListObjects(ctx, destination)
		if err != nil {
			return errors.Wrap(err, "list backup blobs")
		}
		for _, blob := range blobs {
			if err := s.DeleteObject(ctx, blob); err != nil {
				return errors.Wrapf(err, "delete object %s", blob)
			}
		}
		if err := s.DeleteObject(ctx, strings.TrimSuffix(destination, "/")+".md5"); err != nil && err != storage.ErrObjectNotFound {
			return errors.Wrapf(err, "delete object %s", strings.TrimSuffix(destination, "/")+".md5")
		}
		destination = strings.TrimSuffix(destination, "/") + ".sst_info/"
		blobs, err = s.ListObjects(ctx, destination)
		if err != nil {
			return errors.Wrap(err, "list backup objects")
		}
		for _, blob := range blobs {
			if err := s.DeleteObject(ctx, blob); err != nil {
				return errors.Wrapf(err, "delete object %s", blob)
			}
		}
		if err := s.DeleteObject(ctx, strings.TrimSuffix(destination, "/")+".md5"); err != nil && err != storage.ErrObjectNotFound {
			return errors.Wrapf(err, "delete object %s", strings.TrimSuffix(destination, "/")+".md5")
		}
		return nil
	}
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
	if ptr.Deref(job.Status.Ready, 0) == 1 {
		return api.BackupRunning
	}
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
	return api.BackupStarting
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
	}

	if status.State == api.BackupSucceeded {
		status.CompletedAt = job.Status.CompletionTime
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

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		job, err := r.getBackupJob(ctx, cluster, cr)
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

		log.Info("Suspending backup job",
			"job", job.Name,
			"clusterStatus", cluster.Status.Status,
			"readyPXC", cluster.Status.PXC.Ready)

		job.Spec.Suspend = ptr.To(true)

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

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		job, err := r.getBackupJob(ctx, cluster, cr)
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

		if !suspended {
			return nil
		}

		log.Info("Resuming backup job",
			"job", job.Name,
			"clusterStatus", cluster.Status.Status,
			"readyPXC", cluster.Status.PXC.Ready)

		job.Spec.Suspend = ptr.To(false)

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
	cluster *api.PerconaXtraDBCluster,
	cr *api.PerconaXtraDBClusterBackup,
) (*batchv1.Job, error) {
	labelKeyBackupType := naming.GetLabelBackupType(cluster)
	jobName := naming.BackupJobName(cr.Name, cr.Labels[labelKeyBackupType] == "cron")

	job := new(batchv1.Job)

	err := r.client.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: jobName}, job)
	if err != nil {
		return nil, err
	}

	return job, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) cleanUpSuspendedJob(
	ctx context.Context,
	cluster *api.PerconaXtraDBCluster,
	cr *api.PerconaXtraDBClusterBackup,
) error {
	job, err := r.getBackupJob(ctx, cluster, cr)
	if err != nil {
		return errors.Wrap(err, "get job")
	}

	if err := r.client.Delete(ctx, job); err != nil {
		return errors.Wrap(err, "delete job")
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
