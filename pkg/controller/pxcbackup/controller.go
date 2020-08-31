package pxcbackup

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/version"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_perconaxtradbclusterbackup")

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
		return nil, fmt.Errorf("get version: %v", err)
	}

	return &ReconcilePerconaXtraDBClusterBackup{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		serverVersion: sv,
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

	serverVersion *version.ServerVersion
}

// Reconcile reads that state of the cluster for a PerconaXtraDBClusterBackup object and makes changes based on the state read
// and what is in the PerconaXtraDBClusterBackup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBClusterBackup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	// reqLogger.Info("Reconciling PerconaXtraDBClusterBackup")

	rr := reconcile.Result{
		RequeueAfter: time.Second * 5,
	}

	// Fetch the PerconaXtraDBClusterBackup instance
	instance := &api.PerconaXtraDBClusterBackup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.Status.State == api.BackupSucceeded || instance.Status.State == api.BackupFailed {
		// Skip finished backups
		return reconcile.Result{}, nil
	}

	cluster, err := r.getClusterConfig(instance)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("invalid backup cluster: %v", err)
	}

	_, err = cluster.CheckNSetDefaults(r.serverVersion)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("wrong PXC options: %v", err)
	}

	if cluster.Spec.Backup == nil {
		return reconcile.Result{}, fmt.Errorf("a backup image should be set in the PXC config")
	}

	if cluster.Status.PXC.Status != api.AppStateReady {
		return reconcile.Result{}, fmt.Errorf("failed to run backup on cluster with status %s", cluster.Status.Status)
	}

	bcpStorage, ok := cluster.Spec.Backup.Storages[instance.Spec.StorageName]
	if !ok {
		return reconcile.Result{}, fmt.Errorf("bcpStorage %s doesn't exist", instance.Spec.StorageName)
	}

	bcp := backup.New(cluster)
	job := bcp.Job(instance, cluster)
	job.Spec, err = bcp.JobSpec(instance.Spec, cluster.Spec, job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("can't create job spec: %w", err)
	}

	var destination string
	var s3status *api.BackupStorageS3Spec

	switch bcpStorage.Type {
	case api.BackupStorageFilesystem:
		pvc := backup.NewPVC(instance)
		pvc.Spec = *bcpStorage.Volume.PersistentVolumeClaim

		destination = "pvc/" + pvc.Name

		// Set PerconaXtraDBClusterBackup instance as the owner and controller
		if err := setControllerReference(instance, pvc, r.scheme); err != nil {
			return reconcile.Result{}, fmt.Errorf("setControllerReference: %v", err)
		}

		// Check if this PVC already exists
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, pvc)
		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Creating a new volume for backup", "Namespace", pvc.Namespace, "Name", pvc.Name)
			err = r.client.Create(context.TODO(), pvc)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("create backup pvc: %v", err)
			}
		} else if err != nil {
			return reconcile.Result{}, fmt.Errorf("get backup pvc: %v", err)
		}

		err := bcp.SetStoragePVC(&job.Spec, cluster, pvc.Name)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("set storage FS: %v", err)
		}
	case api.BackupStorageS3:
		destination = bcpStorage.S3.Bucket + "/" + instance.Spec.PXCCluster + "-" + instance.CreationTimestamp.Time.Format("2006-01-02-15:04:05") + "-full"
		if !strings.HasPrefix(bcpStorage.S3.Bucket, "s3://") {
			destination = "s3://" + destination
		}
		err := bcp.SetStorageS3(&job.Spec, cluster, bcpStorage.S3, destination)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("set storage FS: %v", err)
		}

		s3status = &bcpStorage.S3
	}

	// Set PerconaXtraDBClusterBackup instance as the owner and controller
	if err := setControllerReference(instance, job, r.scheme); err != nil {
		return reconcile.Result{}, fmt.Errorf("job/setControllerReference: %v", err)
	}

	err = r.client.Create(context.TODO(), job)
	if err != nil && !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, fmt.Errorf("create backup job: %v", err)
	} else if err == nil {
		reqLogger.Info("Created a new backup job", "Namespace", job.Namespace, "Name", job.Name)
	}

	err = r.updateJobStatus(instance, job, destination, instance.Spec.StorageName, s3status)

	return rr, err
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
		return nil, fmt.Errorf("get clusters list: %v", err)
	}

	availableClusters := make([]string, 0)
	for _, cluster := range clusterList.Items {
		if cluster.Name == cr.Spec.PXCCluster {
			return &cluster, nil
		}
		availableClusters = append(availableClusters, cluster.Name)
	}

	return nil, fmt.Errorf("wrong cluster name: %q. Clusters avaliable: %q", cr.Spec.PXCCluster, availableClusters)
}

func (r *ReconcilePerconaXtraDBClusterBackup) updateJobStatus(bcp *api.PerconaXtraDBClusterBackup, job *batchv1.Job, destination, storageName string, s3 *api.BackupStorageS3Spec) error {
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, job)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("get backup status: %v", err)
	}

	status := api.PXCBackupStatus{
		State:       api.BackupStarting,
		Destination: destination,
		StorageName: storageName,
		S3:          s3,
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
		// may be it's k8s v1.10 and erlier (e.g. oc3.9) that doesn't support status updates
		// so try to update whole CR
		err := r.client.Update(context.TODO(), bcp)
		if err != nil {
			return fmt.Errorf("send update: %v", err)
		}
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
