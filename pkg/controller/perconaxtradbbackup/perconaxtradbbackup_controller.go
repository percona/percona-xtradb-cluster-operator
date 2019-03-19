package perconaxtradbbackup

import (
	"context"
	"fmt"
	"reflect"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/version"
)

var log = logf.Log.WithName("controller_perconaxtradbbackup")

// Add creates a new PerconaXtraDBBackup Controller and adds it to the Manager. The Manager will set fields on the Controller
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

	return &ReconcilePerconaXtraDBBackup{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		serverVersion: sv,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("perconaxtradbbackup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PerconaXtraDBBackup
	err = c.Watch(&source.Kind{Type: &api.PerconaXtraDBBackup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePerconaXtraDBBackup{}

// ReconcilePerconaXtraDBBackup reconciles a PerconaXtraDBBackup object
type ReconcilePerconaXtraDBBackup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	serverVersion *api.ServerVersion
}

// Reconcile reads that state of the cluster for a PerconaXtraDBBackup object and makes changes based on the state read
// and what is in the PerconaXtraDBBackup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBBackup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	// reqLogger.Info("Reconciling PerconaXtraDBBackup")

	rr := reconcile.Result{
		RequeueAfter: time.Second * 5,
	}

	// Fetch the PerconaXtraDBBackup instance
	instance := &api.PerconaXtraDBBackup{}
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

	cluster, err := r.getClusterConfig(instance)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("invalid backup cluster: %v", err)
	}

	if cluster.Spec.Backup == nil {
		return reconcile.Result{}, fmt.Errorf("a backup image should be set in the PXC config")
	}

	bcp := backup.New(cluster, cluster.Spec.Backup)
	job := bcp.Job(instance)

	// Check if this Job already exists
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, job)
	// continue only if job not found - we need to creat it just once.
	if !errors.IsNotFound(err) {
		err = r.updateJobStatus(instance, job, instance.Status.Destination, instance.Spec.StorageName)
		return rr, err
	}

	bcpNode, err := r.SelectNode(instance)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("select backup node: %v", err)
	}

	bcpStorage, ok := cluster.Spec.Backup.Storages[instance.Spec.StorageName]
	if !ok {
		return reconcile.Result{}, fmt.Errorf("bcpStorage %s doesn't exist", instance.Spec.StorageName)
	}

	var volumeName string

	job.Spec = bcp.JobSpec(instance.Spec, bcpNode, r.serverVersion)
	switch bcpStorage.Type {
	case api.BackupStorageFilesystem:
		pvc := backup.NewPVC(instance)
		pvc.Spec = *bcpStorage.Volume.PersistentVolumeClaim //app.VolumeSpec(bcpStorage.Volume)

		volumeName = "pvc/" + pvc.Name

		// Set PerconaXtraDBBackup instance as the owner and controller
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

		// getting the volume status
		var pvcStatus VolumeStatus
		for i := time.Duration(1); i <= 5; i++ {
			pvcStatus, err = r.pvcStatus(pvc)
			if err != nil && !errors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("get pvc status: %v", err)
			}

			if pvcStatus == VolumeBound {
				break
			}
			time.Sleep(time.Second * i)
		}

		if pvcStatus != VolumeBound {
			return reconcile.Result{}, fmt.Errorf("pvc not ready, status: %s", pvcStatus)
		}

		err := bcp.SetStoragePVC(&job.Spec, pvc.Name)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("set storage FS: %v", err)
		}
	case api.BackupStorageS3:
		err := bcp.SetStorageS3(&job.Spec, bcpStorage.S3)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("set storage FS: %v", err)
		}
		volumeName = bcpStorage.S3.Bucket
	}

	// Set PerconaXtraDBBackup instance as the owner and controller
	if err := setControllerReference(instance, job, r.scheme); err != nil {
		return reconcile.Result{}, fmt.Errorf("job/setControllerReference: %v", err)
	}

	reqLogger.Info("Creating a new backup job", "Namespace", job.Namespace, "Name", job.Name)
	err = r.client.Create(context.TODO(), job)
	if err != nil && !errors.IsAlreadyExists(err) {
		return reconcile.Result{}, fmt.Errorf("create backup job: %v", err)
	}

	err = r.updateJobStatus(instance, job, volumeName, instance.Spec.StorageName)

	return rr, err
}

func (r *ReconcilePerconaXtraDBBackup) getClusterConfig(cr *api.PerconaXtraDBBackup) (*api.PerconaXtraDBCluster, error) {
	clusterList := api.PerconaXtraDBClusterList{}
	err := r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace: cr.Namespace,
		},
		&clusterList,
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

// VolumeStatus describe the status backup PVC
type VolumeStatus string

const (
	VolumeUndefined VolumeStatus = "Undefined"
	VolumeBound                  = VolumeStatus(corev1.ClaimBound)
	VolumePending                = VolumeStatus(corev1.ClaimPending)
	VolumeLost                   = VolumeStatus(corev1.ClaimLost)
)

func (r *ReconcilePerconaXtraDBBackup) pvcStatus(pvc *corev1.PersistentVolumeClaim) (VolumeStatus, error) {
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, pvc)
	if err != nil {
		return VolumeUndefined, err
	}

	return VolumeStatus(pvc.Status.Phase), nil
}

func (r *ReconcilePerconaXtraDBBackup) updateJobStatus(bcp *api.PerconaXtraDBBackup, job *batchv1.Job, destination, storageName string) error {
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, job)

	if err != nil {
		return fmt.Errorf("get backup status: %v", err)
	}

	status := api.PXCBackupStatus{
		State:       api.BackupStarting,
		Destination: destination,
		StorageName: storageName,
	}

	switch {
	case job.Status.Active == 1:
		status.State = api.BackupRunning
	case job.Status.Succeeded == 1:
		status.State = api.BackupSucceeded
		status.CompletedAt = job.Status.CompletionTime
	case job.Status.Failed == 1:
		status.State = api.BackupFailed
	}

	// don't update the status if there aren't any changes.
	if reflect.DeepEqual(bcp.Status, status) {
		return nil
	}

	bcp.Status = status
	return r.client.Update(context.TODO(), bcp)
}

func setControllerReference(cr *api.PerconaXtraDBBackup, obj metav1.Object, scheme *runtime.Scheme) error {
	ownerRef, err := cr.OwnerRef(scheme)
	if err != nil {
		return err
	}
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ownerRef))
	return nil
}
