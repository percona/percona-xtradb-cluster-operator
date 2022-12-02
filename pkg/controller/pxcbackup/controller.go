package pxcbackup

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
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

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/deployment"
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
	clientcmd           *clientcmd.Client
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
func (r *ReconcilePerconaXtraDBClusterBackup) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
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
	}

	bcp := backup.New(cluster)
	job := bcp.Job(cr, cluster)
	job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job)
	if err != nil {
		return rr, errors.Wrap(err, "can't create job spec")
	}

	switch storage.Type {
	case api.BackupStorageFilesystem:
		pvc := backup.NewPVC(cr)
		pvc.Spec = *storage.Volume.PersistentVolumeClaim

		cr.Status.Destination = "pvc/" + pvc.Name

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

		err := backup.SetStoragePVC(&job.Spec, cr, pvc.Name)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS")
		}
	case api.BackupStorageS3:
		if storage.S3 == nil {
			return rr, errors.New("s3 storage is not specified")
		}
		cr.Status.Destination = storage.S3.Bucket + "/" + cr.Spec.PXCCluster + "-" + cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05") + "-full"
		if !strings.HasPrefix(storage.S3.Bucket, "s3://") {
			cr.Status.Destination = "s3://" + cr.Status.Destination
		}

		err := backup.SetStorageS3(&job.Spec, cr)
		if err != nil {
			return rr, errors.Wrap(err, "set storage FS")
		}
	case api.BackupStorageAzure:
		if storage.Azure == nil {
			return rr, errors.New("azure storage is not specified")
		}
		cr.Status.Destination = storage.Azure.ContainerPath + "/" + cr.Spec.PXCCluster + "-" + cr.CreationTimestamp.Time.Format("2006-01-02-15:04:05") + "-full"
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
		logger.Info("Created a new backup job", "Namespace", job.Namespace, "Name", job.Name)
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

			r.logger(cr.Name, cr.Namespace).Info("all workers are busy - skip backup deletion for now",
				"backup", cr.Name, "in progress", strings.Join(inprog, ", "))
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runDeleteBackupFinalizer(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) {
	logger := r.logger(cr.Name, cr.Namespace)

	defer func() {
		r.bcpDeleteInProgress.Delete(cr.Name)
		<-r.chLimit
	}()

	var finalizers []string
	for _, f := range cr.GetFinalizers() {
		var err error
		switch f {
		case api.FinalizerDeleteS3Backup:
			if (cr.Status.S3 == nil && cr.Status.Azure == nil) || cr.Status.Destination == "" {
				continue
			}
			switch cr.Status.StorageType {
			case api.BackupStorageS3:
				if !strings.HasPrefix(cr.Status.Destination, "s3://") {
					continue
				}
				err = r.runS3BackupFinalizer(cr)
			case api.BackupStorageAzure:
				err = r.runAzureBackupFinalizer(ctx, cr)
			default:
				continue
			}
		default:
			finalizers = append(finalizers, f)
		}
		if err != nil {
			logger.Info("failed to delete backup", "backup path", cr.Status.Destination, "error", err.Error())
			finalizers = append(finalizers, f)
		} else if f == api.FinalizerDeleteS3Backup {
			logger.Info("backup was removed", "name", cr.Name)
		}
	}
	cr.SetFinalizers(finalizers)

	err := r.client.Update(ctx, cr)
	if err != nil {
		logger.Error(err, "failed to update finalizers for backup", "backup", cr.Name)
	}
}

func (r *ReconcilePerconaXtraDBClusterBackup) runS3BackupFinalizer(cr *api.PerconaXtraDBClusterBackup) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Status.S3 == nil {
		return errors.New("s3 storage is not specified")
	}

	s3cli, err := r.s3cli(cr)
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to create s3 client for backup %s", cr.Name)
	} else if k8sErrors.IsNotFound(err) {
		return nil
	}
	logger.Info("deleting backup from s3", "name", cr.Name)

	spl := strings.Split(cr.Status.Destination, "/")
	backup := spl[len(spl)-1]
	err = retry.OnError(retry.DefaultBackoff, func(e error) bool { return true }, removeS3Backup(cr.Status.S3.Bucket, backup, s3cli))
	if err != nil {
		return errors.Wrapf(err, "failed to delete backup %s", cr.Name)
	}
	return nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) runAzureBackupFinalizer(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) error {
	if cr.Status.Azure == nil {
		return errors.New("azure storage is not specified")
	}
	cli, err := r.azureClient(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "new azure client")
	}
	container, _ := cr.Status.Azure.ContainerAndPrefix()
	destination := strings.TrimPrefix(cr.Status.Destination, container+"/")

	err = retry.OnError(retry.DefaultBackoff,
		func(e error) bool {
			return true
		},
		removeAzureBackup(ctx, cli, container, destination))
	if err != nil {
		return errors.Wrapf(err, "failed to delete backup %s", cr.Name)
	}
	return nil
}

func removeAzureBackup(ctx context.Context, cli *azblob.Client, container, destination string) func() error {
	return func() error {
		blobs, err := azureListBlobs(ctx, cli, container, destination+"/")
		if err != nil {
			return errors.Wrap(err, "list backup blobs")
		}
		for _, blob := range blobs {
			_, err = cli.DeleteBlob(ctx, container, url.QueryEscape(blob), nil)
			if err != nil {
				return errors.Wrapf(err, "delete blob %s", blob)
			}
		}
		blobs, err = azureListBlobs(ctx, cli, container, destination+".sst_info/")
		if err != nil {
			return errors.Wrap(err, "list backup blobs")
		}
		for _, blob := range blobs {
			_, err = cli.DeleteBlob(ctx, container, url.QueryEscape(blob), nil)
			if err != nil {
				return errors.Wrapf(err, "delete blob %s", blob)
			}
		}
		return nil
	}
}

func removeS3Backup(bucket, backup string, s3cli *minio.Client) func() error {
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

func (r *ReconcilePerconaXtraDBClusterBackup) getCluster(cr *api.PerconaXtraDBClusterBackup) (*api.PerconaXtraDBCluster, error) {
	cluster := api.PerconaXtraDBCluster{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: cr.Namespace, Name: cr.Spec.PXCCluster}, &cluster)
	if err != nil {
		return nil, errors.Wrap(err, "get PXC cluster")
	}

	return &cluster, nil
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
func (r *ReconcilePerconaXtraDBClusterBackup) azureClient(ctx context.Context, cr *api.PerconaXtraDBClusterBackup) (*azblob.Client, error) {
	secret := new(corev1.Secret)
	err := r.client.Get(ctx, types.NamespacedName{Name: cr.Status.Azure.CredentialsSecret, Namespace: cr.Namespace}, secret)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}
	accountName := string(secret.Data["AZURE_STORAGE_ACCOUNT_NAME"])
	accountKey := string(secret.Data["AZURE_STORAGE_ACCOUNT_KEY"])

	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, errors.Wrap(err, "new credentials")
	}
	endpoint := fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	if cr.Status.Azure.Endpoint != "" {
		endpoint = cr.Status.Azure.Endpoint
	}
	cli, err := azblob.NewClientWithSharedKeyCredential(endpoint, credential, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new client")
	}
	return cli, nil
}

func azureListBlobs(ctx context.Context, client *azblob.Client, containerName, prefix string) ([]string, error) {
	var blobs []string
	pager := client.NewListBlobsFlatPager(containerName, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "list blobs next page")
		}
		if resp.Segment != nil {
			for _, item := range resp.Segment.BlobItems {
				if item != nil && item.Name != nil {
					blobs = append(blobs, *item.Name)
				}
			}
		}
	}
	return blobs, nil
}

func (r *ReconcilePerconaXtraDBClusterBackup) updateJobStatus(bcp *api.PerconaXtraDBClusterBackup, job *batchv1.Job,
	storageName string, storage *api.BackupStorageSpec, cluster *api.PerconaXtraDBCluster) error {
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

	if status.State == api.BackupSucceeded && cluster.PITREnabled() {
		collectorPod, err := deployment.GetBinlogCollectorPod(context.TODO(), r.client, cluster)
		if err != nil {
			return errors.Wrap(err, "get binlog collector pod")
		}

		if err := deployment.RemoveGapFile(context.TODO(), r.clientcmd, collectorPod); err != nil {
			if !errors.Is(err, deployment.GapFileNotFound) {
				return errors.Wrap(err, "remove gap file")
			}
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
