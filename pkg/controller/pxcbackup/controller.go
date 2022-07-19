package pxcbackup

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
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

	zapLog, err := zap.NewProduction()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create logger")
	}

	return &ReconcilePerconaXtraDBClusterBackup{
		client:              mgr.GetClient(),
		scheme:              mgr.GetScheme(),
		serverVersion:       sv,
		chLimit:             make(chan struct{}, limit),
		bcpDeleteInProgress: new(sync.Map),
		log:                 zapr.NewLogger(zapLog),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("perconaxtradbclusterbackup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PerconaXtraDBClusterBackup
	err = c.Watch(&source.Kind{Type: &api.PerconaXtraDBClusterBackup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePerconaXtraDBClusterBackup{}

// ReconcilePerconaXtraDBClusterBackup reconciles a PerconaXtraDBClusterBackup object
type ReconcilePerconaXtraDBClusterBackup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	serverVersion       *version.ServerVersion
	chLimit             chan struct{}
	bcpDeleteInProgress *sync.Map
	log                 logr.Logger
}

func (r *ReconcilePerconaXtraDBClusterBackup) logger(name, namespace string) logr.Logger {
	return r.log.WithName("perconaxtradbclusterbackup").WithValues("backup", name, "namespace", namespace)
}

// Reconcile reads that state of the cluster for a PerconaXtraDBClusterBackup object and makes changes based on the state read
// and what is in the PerconaXtraDBClusterBackup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBClusterBackup) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := r.logger(request.Name, request.Namespace)

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

	err = r.tryRunS3BackupFinalizerJob(cr)
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

	cluster, err := r.getClusterConfig(cr)
	if err != nil {
		logger.Error(err, "invalid backup cluster")
		return rr, nil
	}

	err = cluster.CheckNSetDefaults(r.serverVersion, logger)
	if err != nil {
		return rr, errors.Wrap(err, "wrong PXC options")
	}

	if cluster.Spec.Backup == nil {
		return rr, errors.New("a backup image should be set in the PXC config")
	}

	if err := cluster.CanBackup(); err != nil {
		return rr, errors.Wrap(err, "failed to run backup")
	}

	bcpStorage, ok := cluster.Spec.Backup.Storages[cr.Spec.StorageName]
	if !ok {
		return rr, errors.Errorf("bcpStorage %s doesn't exist", cr.Spec.StorageName)
	}

	bcp := backup.New(cluster)
	job := bcp.Job(cr, cluster)
	job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job)
	if err != nil {
		return rr, errors.Wrap(err, "can't create job spec")
	}

	var destination string
	var s3Status *api.BackupStorageS3Spec
	var azureStatus *api.BackupStorageAzureSpec

	switch bcpStorage.Type {
	case api.BackupStorageFilesystem:
		pvc := backup.NewPVC(cr)
		pvc.Spec = *bcpStorage.Volume.PersistentVolumeClaim

		destination = "pvc/" + pvc.Name

		// Set PerconaXtraDBClusterBackup instance as the owner and controller
		if err := setControllerReference(cr, pvc, r.scheme); err != nil {
			return rr, errors.Wrap(err, "setControllerReference")
		}

		// Check if this PVC already exists
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, pvc)
		if err != nil && k8sErrors.IsNotFound(err) {
			logger.Info("Creating a new volume for backup", "Namespace", pvc.Namespace, "Name", pvc.Name)
			err = r.client.Create(context.TODO(), pvc)
			if err != nil {
				return rr, errors.Wrap(err, "create backup pvc")
			}
		} else if err != nil {
			return rr, errors.Wrap(err, "get backup pvc")
		}

		err := bcp.SetStoragePVC(&job.Spec, cluster, pvc.Name)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS")
		}
	case api.BackupStorageS3:
		destination = bcpStorage.S3.Bucket + "/" + cr.Spec.PXCCluster + "-" + cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05") + "-full"
		if !strings.HasPrefix(bcpStorage.S3.Bucket, "s3://") {
			destination = "s3://" + destination
		}

		err := bcp.SetStorageS3(&job.Spec, cluster, bcpStorage.S3, destination)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS")
		}

		s3Status = &bcpStorage.S3
	case api.BackupStorageAzure:
		destination = "azure://" + bcpStorage.Azure.ContainerName + "/" + cr.Spec.PXCCluster + "-" + cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05") + "-full"
		err := bcp.SetStorageAzure(&job.Spec, cluster, bcpStorage.Azure, destination)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS for Azure")
		}
		azureStatus = &bcpStorage.Azure
	}

	// Set PerconaXtraDBClusterBackup instance as the owner and controller
	if err := setControllerReference(cr, job, r.scheme); err != nil {
		return rr, errors.Wrap(err, "job/setControllerReference")
	}

	err = r.client.Create(context.TODO(), job)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return rr, errors.Wrap(err, "create backup job")
	} else if err == nil {
		logger.Info("Created a new backup job", "Namespace", job.Namespace, "Name", job.Name)
	}

	err = r.updateJobStatus(cr, job, destination, cr.Spec.StorageName, s3Status, azureStatus)

	return rr, err
}

func removeS3Finalizer(cl client.Client, cr *api.PerconaXtraDBClusterBackup) error {
	currFins := cr.GetFinalizers()
	nextFins := make([]string, 0, len(currFins))

	for _, f := range currFins {
		if f != api.FinalizerDeleteS3Backup {
			nextFins = append(nextFins, f)
		}
	}

	if len(nextFins) == len(currFins) {
		return nil
	}

	cr.SetFinalizers(nextFins)
	if err := cl.Update(context.TODO(), cr); err != nil {
		return errors.Wrap(err, "update CR")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) tryRunS3BackupFinalizerJob(cr *api.PerconaXtraDBClusterBackup) error {
	if cr.ObjectMeta.DeletionTimestamp == nil {
		return nil
	}

	if cr.Status.S3 == nil || cr.Status.Destination == "" || !strings.HasPrefix(cr.Status.Destination, "s3://") {
		if err := removeS3Finalizer(r.client, cr); err != nil {
			return errors.Wrap(err, "remove S3 finalizers")
		}

		return nil
	}

	select {
	case r.chLimit <- struct{}{}:
		_, ok := r.bcpDeleteInProgress.LoadOrStore(cr.Name, struct{}{})
		if ok {
			<-r.chLimit
			return nil
		}

		go r.runS3BackupFinalizer(cr)
	default:
		if _, ok := r.bcpDeleteInProgress.Load(cr.Name); !ok {
			inprog := []string{}
			r.bcpDeleteInProgress.Range(func(key, value interface{}) bool {
				inprog = append(inprog, key.(string))
				return true
			})

			r.logger(cr.Name, cr.Namespace).Info("all workers are busy - skip backup deletion for now",
				"backup", cr.Name, "in progress", strings.Join(inprog, ", "))
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runS3BackupFinalizer(cr *api.PerconaXtraDBClusterBackup) {
	logger := r.logger(cr.Name, cr.Namespace)

	defer func() {
		r.bcpDeleteInProgress.Delete(cr.Name)
		<-r.chLimit
	}()

	finalizers := []string{}

	s3cli, err := r.s3cli(cr)
	if err != nil && !k8sErrors.IsNotFound(err) {
		logger.Error(err, "failed to create s3 client for backup", "backup", cr.Name)
		return
	} else if k8sErrors.IsNotFound(err) {
		cr.SetFinalizers(finalizers)
		return
	}

	for _, f := range cr.GetFinalizers() {
		if f == api.FinalizerDeleteS3Backup {
			logger.Info("deleting backup from s3", "name", cr.Name)

			spl := strings.Split(cr.Status.Destination, "/")
			backup := spl[len(spl)-1]
			err = retry.OnError(retry.DefaultBackoff, func(e error) bool { return true }, removeBackup(cr.Status.S3.Bucket, backup, s3cli))
			if err != nil {
				break
			}
		} else {
			finalizers = append(finalizers, f)
		}
	}

	cr.SetFinalizers(finalizers)

	if err != nil {
		logger.Info("Failed to delete backup from s3", "backup path", cr.Status.Destination, "error", err.Error())
	} else {
		logger.Info("backup was removed from s3", "name", cr.Name)
	}

	err = r.client.Update(context.TODO(), cr)
	if err != nil {
		logger.Error(err, "failed to update finalizers for backup", "backup", cr.Name)
	}
}

func removeBackup(bucket, backup string, s3cli *minio.Client) func() error {
	return func() error {
		// this is needed to understand if user provided some path
		// on s3, instead of just a bucket name
		bucketSplitted := strings.Split(bucket, "/")
		if len(bucketSplitted) > 1 {
			bucket = bucketSplitted[0]
			backup = strings.Join(append(bucketSplitted[1:], backup), "/")
		}
		objs := s3cli.ListObjects(context.Background(), bucket,
			minio.ListObjectsOptions{
				Recursive: true,
				Prefix:    backup,
			})

		for v := range objs {
			if v.Err != nil {
				return errors.Wrap(v.Err, "failed to list objects")
			}

			err := s3cli.RemoveObject(context.Background(), bucket, v.Key, minio.RemoveObjectOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to remove object %s", v.Key)
			}
		}

		return nil
	}
}

func (r *ReconcilePerconaXtraDBClusterBackup) getClusterConfig(cr *api.PerconaXtraDBClusterBackup) (*api.PerconaXtraDBCluster, error) {
	clusterList := api.PerconaXtraDBClusterList{}
	err := r.client.List(context.TODO(),
		&clusterList,
		&client.ListOptions{
			Namespace: cr.Namespace,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "get clusters list")
	}

	for _, cluster := range clusterList.Items {
		if cluster.Name == cr.Spec.PXCCluster {
			return &cluster, nil
		}
	}

	return nil, errors.Errorf("wrong cluster name: %s", cr.Spec.PXCCluster)
}

func (r *ReconcilePerconaXtraDBClusterBackup) s3cli(cr *api.PerconaXtraDBClusterBackup) (*minio.Client, error) {
	sec := corev1.Secret{}
	err := r.client.Get(context.Background(),
		types.NamespacedName{Name: cr.Status.S3.CredentialsSecret, Namespace: cr.Namespace}, &sec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}

	accessKeyID := string(sec.Data["AWS_ACCESS_KEY_ID"])
	secretAccessKey := string(sec.Data["AWS_SECRET_ACCESS_KEY"])

	secure := true
	if strings.HasPrefix(cr.Status.S3.EndpointURL, "http://") {
		secure = false
	}

	ep := cr.Status.S3.EndpointURL
	if len(ep) == 0 {
		ep = "s3.amazonaws.com"
	}

	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	ep = strings.TrimSuffix(ep, "/")

	return minio.New(ep, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: secure,
		Region: cr.Status.S3.Region,
	})
}

func (r *ReconcilePerconaXtraDBClusterBackup) updateJobStatus(bcp *api.PerconaXtraDBClusterBackup, job *batchv1.Job,
	destination, storageName string, s3 *api.BackupStorageS3Spec, azure *api.BackupStorageAzureSpec) error {
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, job)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}

		return errors.Wrap(err, "get backup status")
	}

	status := api.PXCBackupStatus{
		State:       api.BackupStarting,
		Destination: destination,
		StorageName: storageName,
		S3:          s3,
		Azure:       azure,
	}

	switch {
	case job.Status.Active == 1:
		status.State = api.BackupRunning
	case job.Status.Succeeded == 1:
		status.State = api.BackupSucceeded
		status.CompletedAt = job.Status.CompletionTime
	case job.Status.Failed >= 1:
		status.State = api.BackupFailed
	}

	// don't update the status if there aren't any changes.
	if reflect.DeepEqual(bcp.Status, status) {
		return nil
	}

	bcp.Status = status

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
