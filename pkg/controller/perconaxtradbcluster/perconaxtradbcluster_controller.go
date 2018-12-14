package perconaxtradbcluster

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/version"
)

var log = logf.Log.WithName("controller_perconaxtradbcluster")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PerconaXtraDBCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
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

	return &ReconcilePerconaXtraDBCluster{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		serverVersion: sv,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("perconaxtradbcluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PerconaXtraDBCluster
	err = c.Watch(&source.Kind{Type: &api.PerconaXtraDBCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner PerconaXtraDBCluster
	// err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
	// 	IsController: true,
	// 	OwnerType:    &api.PerconaXtraDBCluster{},
	// })
	// if err != nil {
	// 	return err
	// }

	return nil
}

var _ reconcile.Reconciler = &ReconcilePerconaXtraDBCluster{}

// ReconcilePerconaXtraDBCluster reconciles a PerconaXtraDBCluster object
type ReconcilePerconaXtraDBCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	serverVersion *api.ServerVersion
}

// Reconcile reads that state of the cluster for a PerconaXtraDBCluster object and makes changes based on the state read
// and what is in the PerconaXtraDBCluster.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling PerconaXtraDBCluster")

	rr := reconcile.Result{
		RequeueAfter: time.Second * 5,
	}
	// Fetch the PerconaXtraDBCluster instance
	o := &api.PerconaXtraDBCluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, o)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return rr, err
	}

	// gvk, err := apiutil.GVKForObject(o, r.scheme)
	// if err != nil {
	// 	return rr, err
	// }

	// fmt.Println("KIND3=>>", gvk.Kind)
	// sv, err := version.Server()
	// if err != nil {
	// 	return rr, fmt.Errorf("get version: %v", err)
	// }
	// h := pxc.New(*sv)

	// 	// use the CR's defenition of platform in case it has set
	// 	if o.Spec.Platform != nil {
	// 		h.serverVersion.Platform = *o.Spec.Platform
	// 	}

	o.Spec.SetDefaults()

	// TODO (ap): the status checking now is fake. Just a stub for further work
	if o.Status.State == api.ClusterStateInit {
		err := r.deploy(o)
		if err != nil {
			// log.Error(err, "cluster deploy error:")
			return rr, err
		}
	}

	err = r.updatePod(statefulset.NewNode(o), o.Spec.PXC, o)
	if err != nil {
		log.Error(err, "pxc upgrade error")
	}

	if o.Spec.ProxySQL.Enabled {
		err = r.updatePod(statefulset.NewProxy(o), o.Spec.ProxySQL, o)
		if err != nil {
			log.Error(err, "proxySQL upgrade error:")
		}
	} else {
		r.client.Delete(context.TODO(), statefulset.NewProxy(o).StatefulSet())
	}

	// // Check if this Pod already exists
	// found := &corev1.Pod{}
	// err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	// if err != nil && errors.IsNotFound(err) {
	// 	reqLogger.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
	// 	err = r.client.Create(context.TODO(), pod)
	// 	if err != nil {
	// 		return rr, err
	// 	}

	// 	// Pod created successfully - don't requeue
	// 	return rr, nil
	// } else if err != nil {
	// 	return rr, err
	// }

	// Pod already exists - don't requeue
	// reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)
	return rr, nil
}

func (r *ReconcilePerconaXtraDBCluster) deploy(cr *api.PerconaXtraDBCluster) error {
	serverVersion := r.serverVersion
	if cr.Spec.Platform != nil {
		serverVersion.Platform = *cr.Spec.Platform
	}

	nodeSet, err := pxc.StatefulSet(statefulset.NewNode(cr), cr.Spec.PXC, cr, serverVersion)
	if err != nil {
		return err
	}

	err = setControllerReference(cr, nodeSet, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Create(context.TODO(), nodeSet)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create newStatefulSetNode: %v", err)
	}

	nodesService := pxc.NewServiceNodes(cr)
	err = setControllerReference(cr, nodesService, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Create(context.TODO(), nodesService)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create PXC Service: %v", err)
	}

	if cr.Spec.ProxySQL.Enabled {
		proxySet, err := pxc.StatefulSet(statefulset.NewProxy(cr), cr.Spec.ProxySQL, cr, serverVersion)
		if err != nil {
			return fmt.Errorf("failed to create ProxySQL Service: %v", err)
		}
		err = setControllerReference(cr, proxySet, r.scheme)
		if err != nil {
			return err
		}

		err = r.client.Create(context.TODO(), proxySet)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create newStatefulSetProxySQL: %v", err)
		}

		proxys := pxc.NewServiceProxySQL(cr)
		err = setControllerReference(cr, proxys, r.scheme)
		if err != nil {
			return err
		}

		err = r.client.Create(context.TODO(), proxys)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create PXC Service: %v", err)
		}
	}

	return nil
}

func setControllerReference(cr *api.PerconaXtraDBCluster, obj metav1.Object, scheme *runtime.Scheme) error {
	ownerRef, err := cr.OwnerRef(scheme)
	if err != nil {
		return err
	}
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ownerRef))
	return nil
}
