package perconaxtradbbackuprestore

import (
	"context"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
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
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

var log = logf.Log.WithName("controller_perconaxtradbbackuprestore")

// Add creates a new PerconaXtraDBBackupRestore Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePerconaXtraDBBackupRestore{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("perconaxtradbbackuprestore-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PerconaXtraDBBackupRestore
	err = c.Watch(&source.Kind{Type: &api.PerconaXtraDBBackupRestore{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePerconaXtraDBBackupRestore{}

// ReconcilePerconaXtraDBBackupRestore reconciles a PerconaXtraDBBackupRestore object
type ReconcilePerconaXtraDBBackupRestore struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a PerconaXtraDBBackupRestore object and makes changes based on the state read
// and what is in the PerconaXtraDBBackupRestore.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBBackupRestore) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	rr := reconcile.Result{
		RequeueAfter: time.Second * 5,
	}

	cr := &api.PerconaXtraDBBackupRestore{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cr)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return rr, err
	}
	err = cr.CheckNsetDefaults()
	if err != nil {
		return rr, err
	}

	bcp := &api.PerconaXtraDBBackup{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.BackupName, Namespace: cr.Namespace}, bcp)
	if err != nil {
		return rr, errors.Wrapf(err, "get backup %s", cr.Spec.BackupName)
	}

	if bcp.Status.State != api.BackupSucceeded {
		return rr, errors.Errorf("backup %s didn't finished yet, current state: %s", bcp.Name, bcp.Status.State)
	}

	cluster := api.PerconaXtraDBCluster{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.PXCCluster, Namespace: cr.Namespace}, &cluster)
	if err != nil {
		return rr, errors.Wrapf(err, "get cluster %s", cr.Spec.PXCCluster)
	}

	err = r.stopCluster(cluster)
	if err != nil {
		return rr, errors.Wrapf(err, "stop cluster %s", cluster.Name)
	}
	defer r.startCluster(cluster)

	err = backup.Restore(bcp, r.client)
	if err != nil {
		errors.Wrap(err, "run restore")
	}
	return rr, nil
}

func (r *ReconcilePerconaXtraDBBackupRestore) stopCluster(c api.PerconaXtraDBCluster) error {
	err := r.client.Delete(context.TODO(), &c)
	if err != nil {
		return errors.Wrap(err, "shutdown pods")
	}

	ls := statefulset.NewNode(&c).Labels()
	err = r.waitForPodsShutdown(ls, c.Namespace)
	if err != nil {
		return errors.Wrap(err, "shutdown pods")
	}

	pvcs := corev1.PersistentVolumeClaimList{}
	err = r.client.List(
		context.TODO(),
		&client.ListOptions{
			Namespace:     c.Namespace,
			LabelSelector: labels.SelectorFromSet(ls),
		},
		&pvcs,
	)
	if err != nil {
		return errors.Wrap(err, "get pvc list")
	}

	for _, pvc := range pvcs.Items {
		err = r.client.Delete(context.TODO(), &pvc)
		if err != nil {
			return errors.Wrap(err, "delete pvc")
		}
	}

	err = r.waitForPVCShutdown(ls, c.Namespace)
	if err != nil {
		return errors.Wrap(err, "shutdown pvc")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBBackupRestore) startCluster(c api.PerconaXtraDBCluster) error {
	return r.client.Update(context.TODO(), &c)
}

const waitLimitSec = 300

func (r *ReconcilePerconaXtraDBBackupRestore) waitForPodsShutdown(ls map[string]string, namespace string) error {
	for i := 0; i < waitLimitSec; i++ {
		pods := corev1.PodList{}

		err := r.client.List(
			context.TODO(),
			&client.ListOptions{
				Namespace:     namespace,
				LabelSelector: labels.SelectorFromSet(ls),
			},
			&pods,
		)
		if err != nil {
			return errors.Wrap(err, "get pods list")
		}

		if len(pods.Items) == 0 {
			return nil
		}

		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}

func (r *ReconcilePerconaXtraDBBackupRestore) waitForPVCShutdown(ls map[string]string, namespace string) error {
	for i := 0; i < waitLimitSec; i++ {
		pvcs := corev1.PersistentVolumeClaimList{}

		err := r.client.List(
			context.TODO(),
			&client.ListOptions{
				Namespace:     namespace,
				LabelSelector: labels.SelectorFromSet(ls),
			},
			&pvcs,
		)
		if err != nil {
			return errors.Wrap(err, "get pvc list")
		}

		if len(pvcs.Items) == 0 {
			return nil
		}

		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}
