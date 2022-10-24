package pxcrestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/version"
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

	zapLog, err := zap.NewProduction()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create logger")
	}

	return &ReconcilePerconaXtraDBClusterRestore{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		serverVersion: sv,
		log:           zapr.NewLogger(zapLog),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("perconaxtradbclusterrestore-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PerconaXtraDBClusterRestore
	err = c.Watch(&source.Kind{Type: &api.PerconaXtraDBClusterRestore{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePerconaXtraDBClusterRestore{}

// ReconcilePerconaXtraDBClusterRestore reconciles a PerconaXtraDBClusterRestore object
type ReconcilePerconaXtraDBClusterRestore struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	serverVersion *version.ServerVersion
	log           logr.Logger
}

func (r *ReconcilePerconaXtraDBClusterRestore) logger(name, namespace string) logr.Logger {
	return r.log.WithName("perconaxtradbclusterrestore").WithValues("restore", name, "namespace", namespace)
}

// Reconcile reads that state of the cluster for a PerconaXtraDBClusterRestore object and makes changes based on the state read
// and what is in the PerconaXtraDBClusterRestore.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBClusterRestore) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	rr := reconcile.Result{}

	cr := &api.PerconaXtraDBClusterRestore{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cr)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return rr, err
	}
	if cr.Status.State != api.RestoreNew {
		return rr, nil
	}

	lgr := r.logger(request.Name, request.Namespace)
	lgr.Info("backup restore request")

	err = r.setStatus(cr, api.RestoreStarting, "")
	if err != nil {
		return rr, errors.Wrap(err, "set status")
	}
	rJobsList := &api.PerconaXtraDBClusterRestoreList{}
	err = r.client.List(
		context.TODO(),
		rJobsList,
		&client.ListOptions{
			Namespace: cr.Namespace,
		},
	)
	if err != nil {
		return rr, errors.Wrap(err, "get restore jobs list")
	}

	returnMsg := fmt.Sprintf(backupRestoredMsg, cr.Name, cr.Spec.PXCCluster, cr.Name)

	defer func() {
		status := api.BcpRestoreStates(api.RestoreSucceeded)
		if err != nil {
			status = api.RestoreFailed
			returnMsg = err.Error()
		}
		r.setStatus(cr, status, returnMsg)
	}()

	for _, j := range rJobsList.Items {
		if j.Spec.PXCCluster == cr.Spec.PXCCluster &&
			j.Name != cr.Name && j.Status.State != api.RestoreFailed &&
			j.Status.State != api.RestoreSucceeded {
			err = errors.Errorf("unable to continue, concurent restore job %s running now.", j.Name)
			return rr, err
		}
	}

	err = cr.CheckNsetDefaults()
	if err != nil {
		return rr, err
	}
	bcp, err := r.getBackup(cr)
	if err != nil {
		return rr, errors.Wrap(err, "get backup")
	}

	cluster := api.PerconaXtraDBCluster{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.PXCCluster, Namespace: cr.Namespace}, &cluster)
	if err != nil {
		err = errors.Wrapf(err, "get cluster %s", cr.Spec.PXCCluster)
		return rr, err
	}
	clusterOrig := cluster.DeepCopy()

	err = cluster.CheckNSetDefaults(r.serverVersion, r.log)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("wrong PXC options: %v", err)
	}

	lgr.Info("stopping cluster", "cluster", cr.Spec.PXCCluster)
	err = r.setStatus(cr, api.RestoreStopCluster, "")
	if err != nil {
		err = errors.Wrap(err, "set status")
		return rr, err
	}
	err = r.stopCluster(cluster.DeepCopy())
	if err != nil {
		err = errors.Wrapf(err, "stop cluster %s", cluster.Name)
		return rr, err
	}

	lgr.Info("starting restore", "cluster", cr.Spec.PXCCluster, "backup", cr.Spec.BackupName)
	err = r.setStatus(cr, api.RestoreRestore, "")
	if err != nil {
		err = errors.Wrap(err, "set status")
		return rr, err
	}
	err = r.restore(cr, bcp, cluster.Spec)
	if err != nil {
		err = errors.Wrap(err, "run restore")
		return rr, err
	}

	lgr.Info("starting cluster", "cluster", cr.Spec.PXCCluster)
	err = r.setStatus(cr, api.RestoreStartCluster, "")
	if err != nil {
		err = errors.Wrap(err, "set status")
		return rr, err
	}

	if cr.Spec.PITR != nil {
		oldSize := cluster.Spec.PXC.Size
		oldUnsafe := cluster.Spec.AllowUnsafeConfig
		cluster.Spec.PXC.Size = 1
		cluster.Spec.AllowUnsafeConfig = true

		if err := r.startCluster(&cluster); err != nil {
			return rr, errors.Wrap(err, "restart cluster for pitr")
		}

		lgr.Info("point-in-time recovering", "cluster", cr.Spec.PXCCluster)
		err = r.setStatus(cr, api.RestorePITR, "")
		if err != nil {
			return rr, errors.Wrap(err, "set status")
		}

		err = r.pitr(cr, bcp, cluster.Spec)
		if err != nil {
			return rr, errors.Wrap(err, "run pitr")
		}

		cluster.Spec.PXC.Size = oldSize
		cluster.Spec.AllowUnsafeConfig = oldUnsafe
	}

	err = r.startCluster(clusterOrig)
	if err != nil {
		err = errors.Wrap(err, "restart cluster")
		return rr, err
	}

	lgr.Info(returnMsg)

	return rr, err
}

func (r *ReconcilePerconaXtraDBClusterRestore) getBackup(cr *api.PerconaXtraDBClusterRestore) (*api.PerconaXtraDBClusterBackup, error) {
	if cr.Spec.BackupSource != nil {
		return &api.PerconaXtraDBClusterBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
			Spec: api.PXCBackupSpec{
				PXCCluster:  cr.Spec.PXCCluster,
				StorageName: cr.Spec.BackupSource.StorageName,
			},
			Status: api.PXCBackupStatus{
				State:       api.BackupSucceeded,
				Destination: cr.Spec.BackupSource.Destination,
				StorageName: cr.Spec.BackupSource.StorageName,
				S3:          cr.Spec.BackupSource.S3,
			},
		}, nil
	}

	bcp := &api.PerconaXtraDBClusterBackup{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.BackupName, Namespace: cr.Namespace}, bcp)
	if err != nil {
		err = errors.Wrapf(err, "get backup %s", cr.Spec.BackupName)
		return bcp, err
	}
	if bcp.Status.State != api.BackupSucceeded {
		err = errors.Errorf("backup %s didn't finished yet, current state: %s", bcp.Name, bcp.Status.State)
		return bcp, err
	}

	return bcp, nil
}

const backupRestoredMsg = `You can view xtrabackup log:
$ kubectl logs job/restore-job-%s-%s
If everything is fine, you can cleanup the job:
$ kubectl delete pxc-restore/%s
`

func (r *ReconcilePerconaXtraDBClusterRestore) stopCluster(c *api.PerconaXtraDBCluster) error {
	var gracePeriodSec int64

	if c.Spec.PXC != nil && c.Spec.PXC.TerminationGracePeriodSeconds != nil {
		gracePeriodSec = int64(c.Spec.PXC.Size) * *c.Spec.PXC.TerminationGracePeriodSeconds
	}

	patch := client.MergeFrom(c.DeepCopy())
	c.Spec.Pause = true
	err := r.client.Patch(context.TODO(), c, patch)
	if err != nil {
		return errors.Wrap(err, "shutdown pods")
	}

	ls := statefulset.NewNode(c).Labels()
	err = r.waitForPodsShutdown(ls, c.Namespace, gracePeriodSec)
	if err != nil {
		return errors.Wrap(err, "shutdown pods")
	}

	pvcs := corev1.PersistentVolumeClaimList{}
	err = r.client.List(
		context.TODO(),
		&pvcs,
		&client.ListOptions{
			Namespace:     c.Namespace,
			LabelSelector: labels.SelectorFromSet(ls),
		},
	)
	if err != nil {
		return errors.Wrap(err, "get pvc list")
	}

	pxcNode := statefulset.NewNode(c)
	pvcNameTemplate := statefulset.DataVolumeName + "-" + pxcNode.StatefulSet().Name
	for _, pvc := range pvcs.Items {
		// check prefix just in case, to be sure we're not going to delete a wrong pvc
		if pvc.Name == pvcNameTemplate+"-0" || !strings.HasPrefix(pvc.Name, pvcNameTemplate) {
			continue
		}

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

func (r *ReconcilePerconaXtraDBClusterRestore) startCluster(cr *api.PerconaXtraDBCluster) (err error) {
	// tryin several times just to avoid possible conflicts with the main controller
	err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// need to get the object with latest version of meta-data for update
		current := &api.PerconaXtraDBCluster{}
		rerr := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, current)
		if rerr != nil {
			return errors.Wrap(err, "get cluster")
		}
		current.Spec = cr.Spec
		return r.client.Update(context.TODO(), current)
	})
	if err != nil {
		return errors.Wrap(err, "update cluster")
	}

	// give time for process new state
	time.Sleep(10 * time.Second)

	var waitLimit int32 = 2 * 60 * 60 // 2 hours
	if cr.Spec.PXC.LivenessInitialDelaySeconds != nil {
		waitLimit = *cr.Spec.PXC.LivenessInitialDelaySeconds * cr.Spec.PXC.Size
	}

	for i := int32(0); i < waitLimit; i++ {
		current := &api.PerconaXtraDBCluster{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, current)
		if err != nil {
			return errors.Wrap(err, "get cluster")
		}
		if current.Status.ObservedGeneration == current.Generation && current.Status.PXC.Status == api.AppStateReady {
			return nil
		}
		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}

const waitLimitSec int64 = 300

func (r *ReconcilePerconaXtraDBClusterRestore) waitForPodsShutdown(ls map[string]string, namespace string, gracePeriodSec int64) error {
	for i := int64(0); i < waitLimitSec+gracePeriodSec; i++ {
		pods := corev1.PodList{}

		err := r.client.List(
			context.TODO(),
			&pods,
			&client.ListOptions{
				Namespace:     namespace,
				LabelSelector: labels.SelectorFromSet(ls),
			},
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

func (r *ReconcilePerconaXtraDBClusterRestore) waitForPVCShutdown(ls map[string]string, namespace string) error {
	for i := int64(0); i < waitLimitSec; i++ {
		pvcs := corev1.PersistentVolumeClaimList{}

		err := r.client.List(
			context.TODO(),
			&pvcs,
			&client.ListOptions{
				Namespace:     namespace,
				LabelSelector: labels.SelectorFromSet(ls),
			},
		)
		if err != nil {
			return errors.Wrap(err, "get pvc list")
		}

		if len(pvcs.Items) == 1 {
			return nil
		}

		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}

func (r *ReconcilePerconaXtraDBClusterRestore) setStatus(cr *api.PerconaXtraDBClusterRestore, state api.BcpRestoreStates, comments string) error {
	cr.Status.State = state
	switch state {
	case api.RestoreSucceeded:
		tm := metav1.NewTime(time.Now())
		cr.Status.CompletedAt = &tm
	}

	cr.Status.Comments = comments

	err := r.client.Status().Update(context.TODO(), cr)
	if err != nil {
		return errors.Wrap(err, "send update")
	}

	return nil
}
