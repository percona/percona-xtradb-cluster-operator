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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/deployment"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/version"
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
		Watches(&api.PerconaXtraDBClusterBackup{}, &handler.EnqueueRequestForObject{}).
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
	err := r.client.Get(context.TODO(), request.NamespacedName, cr)
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
		return reconcile.Result{}, errors.Wrap(err, "failed to run finalizers")
	}

	if cr.Status.State == api.BackupSucceeded ||
		cr.Status.State == api.BackupFailed {
		if len(cr.GetFinalizers()) > 0 {
			return rr, nil
		}
		return reconcile.Result{}, nil
	}

	if cr.DeletionTimestamp != nil {
		return rr, nil
	}

	cluster, err := r.getCluster(cr)
	if err != nil {
		log.Error(err, "invalid backup cluster")
		return rr, nil
	}

	err = cluster.CheckNSetDefaults(r.serverVersion, log)
	if err != nil {
		return rr, errors.Wrap(err, "wrong PXC options")
	}

	if cluster.Spec.Backup == nil {
		return rr, errors.New("a backup image should be set in the PXC config")
	}

	if err := cluster.CanBackup(); err != nil {
		return rr, errors.Wrap(err, "failed to run backup")
	}

	if !cluster.Spec.Backup.GetAllowParallel() {
		isRunning, err := r.isOtherBackupRunning(ctx, cr)
		if err != nil {
			return rr, errors.Wrap(err, "failed to check if other backups running")
		}
		if isRunning {
			log.Info("backup already running, waiting until it's done")
			return rr, nil
		}
	}

	storage, ok := cluster.Spec.Backup.Storages[cr.Spec.StorageName]
	if !ok {
		return rr, errors.Errorf("storage %s doesn't exist", cr.Spec.StorageName)
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

	bcp := backup.New(cluster)
	job := bcp.Job(cr, cluster)
	initImage, err := k8s.GetInitImage(ctx, cluster, r.client)
	if err != nil {
		return rr, errors.Wrap(err, "failed to get initImage")
	}
	job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, initImage)
	if err != nil {
		return rr, errors.Wrap(err, "can't create job spec")
	}

	switch storage.Type {
	case api.BackupStorageFilesystem:
		pvc := backup.NewPVC(cr)
		pvc.Spec = *storage.Volume.PersistentVolumeClaim

		cr.Status.Destination.SetPVCDestination(pvc.Name)

		// Set PerconaXtraDBClusterBackup instance as the owner and controller
		if err := setControllerReference(cr, pvc, r.scheme); err != nil {
			return rr, errors.Wrap(err, "setControllerReference")
		}

		// Check if this PVC already exists
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, pvc)
		if err != nil && k8sErrors.IsNotFound(err) {
			log.Info("Creating a new volume for backup", "Namespace", pvc.Namespace, "Name", pvc.Name)
			err = r.client.Create(context.TODO(), pvc)
			if err != nil {
				return rr, errors.Wrap(err, "create backup pvc")
			}
		} else if err != nil {
			return rr, errors.Wrap(err, "get backup pvc")
		}

		err := backup.SetStoragePVC(&job.Spec, cr, pvc.Name)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS")
		}
	case api.BackupStorageS3:
		if storage.S3 == nil {
			return rr, errors.New("s3 storage is not specified")
		}
		cr.Status.Destination.SetS3Destination(storage.S3.Bucket, cr.Spec.PXCCluster+"-"+cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05")+"-full")

		err := backup.SetStorageS3(&job.Spec, cr)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS")
		}
	case api.BackupStorageAzure:
		if storage.Azure == nil {
			return rr, errors.New("azure storage is not specified")
		}
		cr.Status.Destination.SetAzureDestination(storage.Azure.ContainerPath, cr.Spec.PXCCluster+"-"+cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05")+"-full")

		err := backup.SetStorageAzure(&job.Spec, cr)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS for Azure")
		}
	}

	// Set PerconaXtraDBClusterBackup instance as the owner and controller
	if err := setControllerReference(cr, job, r.scheme); err != nil {
		return rr, errors.Wrap(err, "job/setControllerReference")
	}

	err = r.client.Create(context.TODO(), job)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return rr, errors.Wrap(err, "create backup job")
	} else if err == nil {
		log.Info("Created a new backup job", "Namespace", job.Namespace, "Name", job.Name)
	}

	err = r.updateJobStatus(cr, job, cr.Spec.StorageName, storage, cluster)

	return rr, err
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

		go r.runDeleteBackupFinalizer(ctx, cr)
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

func (r *ReconcilePerconaXtraDBClusterBackup) runDeleteBackupFinalizer(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) {
	log := logf.FromContext(ctx)

	defer func() {
		r.bcpDeleteInProgress.Delete(cr.Name)
		<-r.chLimit
	}()

	var finalizers []string
	for _, f := range cr.GetFinalizers() {
		var err error
		switch f {
		case naming.FinalizerS3DeleteBackup:
			log.Info("The finalizer delete-s3-backup is deprecated and will be deleted in 1.18.0. Use percona.com/delete-backup")
			fallthrough
		case naming.FinalizerDeleteBackup:

			if (cr.Status.S3 == nil && cr.Status.Azure == nil) || cr.Status.Destination == "" {
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
			default:
				continue
			}
		default:
			finalizers = append(finalizers, f)
		}
		if err != nil {
			log.Info("failed to delete backup", "backup path", cr.Status.Destination, "error", err.Error())
			finalizers = append(finalizers, f)
		} else if f == naming.FinalizerDeleteBackup || f == naming.FinalizerS3DeleteBackup {

			log.Info("backup was removed", "name", cr.Name)
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

func (r *ReconcilePerconaXtraDBClusterBackup) getCluster(cr *api.PerconaXtraDBClusterBackup) (*api.PerconaXtraDBCluster, error) {
	cluster := api.PerconaXtraDBCluster{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: cr.Namespace, Name: cr.Spec.PXCCluster}, &cluster)
	if err != nil {
		return nil, errors.Wrap(err, "get PXC cluster")
	}

	return &cluster, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) updateJobStatus(bcp *api.PerconaXtraDBClusterBackup, job *batchv1.Job,
	storageName string, storage *api.BackupStorageSpec, cluster *api.PerconaXtraDBCluster,
) error {
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, job)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}

		return errors.Wrap(err, "get backup status")
	}

	status := api.PXCBackupStatus{
		State:                 api.BackupStarting,
		Destination:           bcp.Status.Destination,
		StorageName:           storageName,
		S3:                    storage.S3,
		Azure:                 storage.Azure,
		StorageType:           storage.Type,
		Image:                 bcp.Status.Image,
		SSLSecretName:         bcp.Status.SSLSecretName,
		SSLInternalSecretName: bcp.Status.SSLInternalSecretName,
		VaultSecretName:       bcp.Status.VaultSecretName,
		VerifyTLS:             storage.VerifyTLS,
	}

	if job.Status.Active == 1 {
		status.State = api.BackupRunning
	}

	for _, cond := range job.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}
		switch cond.Type {
		case batchv1.JobFailed:
			status.State = api.BackupFailed
		case batchv1.JobComplete:
			status.State = api.BackupSucceeded
			status.CompletedAt = job.Status.CompletionTime
		}
	}

	// don't update the status if there aren't any changes.
	if reflect.DeepEqual(bcp.Status, status) {
		return nil
	}

	bcp.Status = status

	if status.State == api.BackupSucceeded {
		if cluster.PITREnabled() {
			collectorPod, err := deployment.GetBinlogCollectorPod(context.TODO(), r.client, cluster)
			if err != nil {
				return errors.Wrap(err, "get binlog collector pod")
			}

			if err := deployment.RemoveGapFile(context.TODO(), cluster, r.clientcmd, collectorPod); err != nil {
				if !errors.Is(err, deployment.GapFileNotFound) {
					return errors.Wrap(err, "remove gap file")
				}
			}

			if err := deployment.RemoveTimelineFile(context.TODO(), cluster, r.clientcmd, collectorPod); err != nil {
				return errors.Wrap(err, "remove timeline file")
			}
		}

		initSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name + "-mysql-init",
				Namespace: cluster.Namespace,
			},
		}
		if err := r.client.Delete(context.TODO(), &initSecret); client.IgnoreNotFound(err) != nil {
			return errors.Wrap(err, "delete mysql-init secret")
		}
	}

	err = r.client.Status().Update(context.TODO(), bcp)
	if err != nil {
		return errors.Wrap(err, "send update")
	}

	return nil
}

func setControllerReference(cr *api.PerconaXtraDBClusterBackup, obj metav1.Object, scheme *runtime.Scheme) error {
	ownerRef, err := cr.OwnerRef(scheme)
	if err != nil {
		return err
	}
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ownerRef))
	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) isOtherBackupRunning(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) (bool, error) {
	list := new(batchv1.JobList)
	lbls := map[string]string{
		"type":    "xtrabackup",
		"cluster": cr.Spec.PXCCluster,
	}
	if err := r.client.List(ctx, list, &client.ListOptions{
		Namespace:     cr.Namespace,
		LabelSelector: labels.SelectorFromSet(lbls),
	}); err != nil {
		return false, errors.Wrap(err, "list jobs")
	}

	for _, job := range list.Items {
		if job.Labels["backup-name"] == cr.Name || job.Labels["backup-name"] == "" {
			continue
		}
		if job.Status.Active == 0 && (jobSucceded(&job) || jobFailed(&job)) {
			continue
		}

		return true, nil
	}

	return false, nil
}

func jobFailed(job *batchv1.Job) bool {
	failedCondition := findJobCondition(job.Status.Conditions, batchv1.JobFailed)
	if failedCondition != nil && failedCondition.Status == corev1.ConditionTrue {
		return true
	}
	return false
}

func jobSucceded(job *batchv1.Job) bool {
	succeededCondition := findJobCondition(job.Status.Conditions, batchv1.JobComplete)
	if succeededCondition != nil && succeededCondition.Status == corev1.ConditionTrue {
		return true
	}
	return false
}

func findJobCondition(conditions []batchv1.JobCondition, condType batchv1.JobConditionType) *batchv1.JobCondition {
	for i, cond := range conditions {
		if cond.Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
