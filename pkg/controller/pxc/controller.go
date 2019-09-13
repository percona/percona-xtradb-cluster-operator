package pxc

import (
	"context"
	"crypto/md5"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/configmap"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/version"
)

var log = logf.Log.WithName("controller_perconaxtradbcluster")

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
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	// reqLogger.Info("Reconciling PerconaXtraDBCluster")

	rr := reconcile.Result{
		RequeueAfter: time.Second * 5,
	}
	// Fetch the PerconaXtraDBCluster instance
	o := &api.PerconaXtraDBCluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, o)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	defer func() {
		uerr := r.updateStatus(o, err)
		if uerr != nil {
			log.Error(uerr, "Update status")
		}
	}()

	changed, err := o.CheckNSetDefaults()
	if err != nil {
		err = fmt.Errorf("wrong PXC options: %v", err)
		return reconcile.Result{}, err
	}

	// update CR if there was changes that may be read by another cr (e.g. pxc-backup)
	if changed {
		err = r.client.Update(context.TODO(), o)
		if err != nil {
			err = fmt.Errorf("update PXC CR: %v", err)
			return reconcile.Result{}, err
		}
	}

	if o.ObjectMeta.DeletionTimestamp != nil {
		finalizers := []string{}
		for _, fnlz := range o.GetFinalizers() {
			var sfs api.StatefulApp
			switch fnlz {
			case "delete-proxysql-pvc":
				sfs = statefulset.NewProxy(o)
				// deletePVC is always true on this stage
				// because we never reach this point without finalizers
				err = r.deleteStatfulSet(o.Namespace, sfs, true)
			case "delete-pxc-pvc":
				sfs = statefulset.NewNode(o)
				err = r.deleteStatfulSet(o.Namespace, sfs, true)
			// nil error gonna be returned only when there is no more pods to delete (only 0 left)
			// until than finalizer won't be deleted
			case "delete-pxc-pods-in-order":
				sfs = statefulset.NewNode(o)
				err = r.deleteStatfulSetPods(o.Namespace, sfs)
			}
			if err != nil {
				finalizers = append(finalizers, fnlz)
			}
		}

		o.SetFinalizers(finalizers)
		r.client.Update(context.TODO(), o)

		// If we're waiting for the pods, technically it's not an error
		if err == ErrWaitingForDeletingPods {
			err = nil
		}

		// object is being deleted, no need in further actions
		return rr, err
	}

	if o.Spec.PXC == nil {
		err = fmt.Errorf("pxc not specified")
		return reconcile.Result{}, err
	}

	err = r.reconcileUsersSecret(o)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("reconcile users secret: %v", err)
	}

	err = r.deploy(o)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.updatePod(statefulset.NewNode(o), o.Spec.PXC, o)
	if err != nil {
		err = fmt.Errorf("pxc upgrade error: %v", err)
		return reconcile.Result{}, err
	}

	proxysqlSet := statefulset.NewProxy(o)
	if o.Spec.ProxySQL != nil && o.Spec.ProxySQL.Enabled {
		err = r.updatePod(proxysqlSet, o.Spec.ProxySQL, o)
		if err != nil {
			err = fmt.Errorf("ProxySQL upgrade error: %v", err)
			return reconcile.Result{}, err
		}
	} else {
		// check if there is need to delete pvc
		deletePVC := false
		for _, fnlz := range o.GetFinalizers() {
			if fnlz == "delete-proxysql-pvc" {
				deletePVC = true
				break
			}
		}

		err = r.deleteStatfulSet(o.Namespace, proxysqlSet, deletePVC)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	err = r.reconcileBackups(o)
	if err != nil {
		return reconcile.Result{}, err
	}

	return rr, nil
}

func (r *ReconcilePerconaXtraDBCluster) deploy(cr *api.PerconaXtraDBCluster) error {
	serverVersion := r.serverVersion
	if cr.Spec.Platform != nil {
		serverVersion.Platform = *cr.Spec.Platform
	}

	stsApp := statefulset.NewNode(cr)
	err := r.reconcileConfigMap(cr)
	if err != nil {
		return err
	}

	nodeSet, err := pxc.StatefulSet(stsApp, cr.Spec.PXC, cr, serverVersion)
	if err != nil {
		return err
	}

	// TODO: code duplication with updatePod function
	configString := cr.Spec.PXC.Configuration
	configHash := fmt.Sprintf("%x", md5.Sum([]byte(configString)))
	if nodeSet.Spec.Template.Annotations == nil {
		nodeSet.Spec.Template.Annotations = make(map[string]string)
	}
	nodeSet.Spec.Template.Annotations["percona.com/configuration-hash"] = configHash

	err = r.reconsileSSL(cr)
	if err != nil {
		return fmt.Errorf(`TLS secrets handler: "%v". Please create your TLS secret `+cr.Spec.PXC.SSLSecretName+` and `+cr.Spec.PXC.SSLInternalSecretName+` manually or setup cert-manager correctly`, err)
	}

	sslHash, err := r.getTLSHash(cr, cr.Spec.PXC.SSLSecretName)
	if err != nil {
		return fmt.Errorf("get secret hash error: %v", err)
	}
	nodeSet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash

	sslInternalHash, err := r.getTLSHash(cr, cr.Spec.PXC.SSLInternalSecretName)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get secret hash error: %v", err)
	}
	if !k8serrors.IsNotFound(err) {
		nodeSet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
	}

	err = setControllerReference(cr, nodeSet, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Create(context.TODO(), nodeSet)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create newStatefulSetNode: %v", err)
	}

	err = r.createService(cr, pxc.NewServicePXCUnready(cr))
	if err != nil {
		return errors.Wrap(err, "create PXC ServiceUnready")
	}
	err = r.createService(cr, pxc.NewServicePXC(cr))
	if err != nil {
		return errors.Wrap(err, "create PXC Service")
	}

	// PodDisruptionBudget object for nodes
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: nodeSet.Name, Namespace: nodeSet.Namespace}, nodeSet)
	if err == nil {
		err := r.reconcilePDB(cr.Spec.PXC.PodDisruptionBudget, stsApp, cr.Namespace, nodeSet)
		if err != nil {
			return fmt.Errorf("PodDisruptionBudget for %s: %v", nodeSet.Name, err)
		}
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get PXC stateful set: %v", err)
	}

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		sfsProxy := statefulset.NewProxy(cr)
		proxySet, err := pxc.StatefulSet(sfsProxy, cr.Spec.ProxySQL, cr, serverVersion)
		if err != nil {
			return fmt.Errorf("create ProxySQL Service: %v", err)
		}
		err = setControllerReference(cr, proxySet, r.scheme)
		if err != nil {
			return err
		}

		// TODO: code duplication with updatePod function
		if proxySet.Spec.Template.Annotations == nil {
			proxySet.Spec.Template.Annotations = make(map[string]string)
		}
		proxyConfigString := cr.Spec.ProxySQL.Configuration
		proxyConfigHash := fmt.Sprintf("%x", md5.Sum([]byte(proxyConfigString)))
		if nodeSet.Spec.Template.Annotations == nil {
			nodeSet.Spec.Template.Annotations = make(map[string]string)
		}
		proxySet.Spec.Template.Annotations["percona.com/configuration-hash"] = proxyConfigHash
		proxySet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash
		proxySet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash

		err = r.client.Create(context.TODO(), proxySet)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create newStatefulSetProxySQL: %v", err)
		}

		// ProxySQL Service
		err = r.createService(cr, pxc.NewServiceProxySQL(cr))
		if err != nil {
			return errors.Wrap(err, "create ProxySQL Service")
		}

		// ProxySQL Unready Service
		err = r.createService(cr, pxc.NewServiceProxySQLUnready(cr))
		if err != nil {
			return errors.Wrap(err, "create ProxySQL ServiceUnready")
		}

		// PodDisruptionBudget object for ProxySQL
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: proxySet.Name, Namespace: proxySet.Namespace}, proxySet)
		if err == nil {
			err := r.reconcilePDB(cr.Spec.ProxySQL.PodDisruptionBudget, sfsProxy, cr.Namespace, proxySet)
			if err != nil {
				return fmt.Errorf("PodDisruptionBudget for %s: %v", proxySet.Name, err)
			}
		} else if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get ProxySQL stateful set: %v", err)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) createService(cr *api.PerconaXtraDBCluster, svc *corev1.Service) error {
	err := setControllerReference(cr, svc, r.scheme)
	if err != nil {
		return errors.Wrap(err, "setControllerReference")
	}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &corev1.Service{})
	if err != nil && k8serrors.IsNotFound(err) {
		err := r.client.Create(context.TODO(), svc)
		return errors.WithMessage(err, "create")
	}

	return errors.WithMessage(err, "check if exists")
}

func (r *ReconcilePerconaXtraDBCluster) reconcileConfigMap(cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.PXC.Configuration != "" {
		stsApp := statefulset.NewNode(cr)
		ls := stsApp.Labels()
		configMap := configmap.NewConfigMap(cr, ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"])
		err := setControllerReference(cr, configMap, r.scheme)
		if err != nil {
			return err
		}
		err = r.client.Create(context.TODO(), configMap)
		if err != nil && k8serrors.IsAlreadyExists(err) {
			err = r.client.Update(context.TODO(), configMap)
			if err != nil {
				return fmt.Errorf("update ConfigMap: %v", err)
			}
		} else if err != nil {
			return fmt.Errorf("create ConfigMap: %v", err)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcilePDB(spec *api.PodDisruptionBudgetSpec, sfs api.StatefulApp, namespace string, owner runtime.Object) error {
	if spec == nil {
		return nil
	}

	pdb := pxc.PodDisruptionBudget(spec, sfs, namespace)
	err := setControllerReference(owner, pdb, r.scheme)
	if err != nil {
		return fmt.Errorf("set owner reference: %v", err)
	}

	cpdb := &policyv1beta1.PodDisruptionBudget{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pdb.Name, Namespace: namespace}, cpdb)
	if err != nil && k8serrors.IsNotFound(err) {
		return r.client.Create(context.TODO(), pdb)
	} else if err != nil {
		return fmt.Errorf("get: %v", err)
	}

	cpdb.Spec = pdb.Spec
	return r.client.Update(context.TODO(), cpdb)
}

// ErrWaitingForDeletingPods indicating that the stateful set have more than a one pods left
var ErrWaitingForDeletingPods = fmt.Errorf("waiting for pods to be deleted")

func (r *ReconcilePerconaXtraDBCluster) deleteStatfulSetPods(namespace string, sfs api.StatefulApp) error {
	list := corev1.PodList{}

	err := r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
		&list,
	)
	if err != nil {
		return fmt.Errorf("get list: %v", err)
	}

	// the last pod left - we can leave it for the stateful set
	if len(list.Items) <= 1 {
		time.Sleep(time.Second * 3)
		return nil
	}

	// after setting the pods for delete we need to downscale statefulset to 1 under,
	// otherwise it will be trying to deploy the nodes again to reach the desired replicas count
	cSet := sfs.StatefulSet()
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cSet.Name, Namespace: cSet.Namespace}, cSet)
	if err != nil {
		return fmt.Errorf("get StatefulSet: %v", err)
	}

	dscaleTo := int32(1)
	cSet.Spec.Replicas = &dscaleTo
	err = r.client.Update(context.TODO(), cSet)
	if err != nil {
		return fmt.Errorf("downscale StatefulSet: %v", err)
	}

	return ErrWaitingForDeletingPods
}

func (r *ReconcilePerconaXtraDBCluster) deleteStatfulSet(namespace string, sfs api.StatefulApp, deletePVC bool) error {
	err := r.client.Delete(context.TODO(), sfs.StatefulSet())
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete proxysql: %v", err)
	}
	if deletePVC {
		err = r.deletePVC(namespace, sfs.Labels())
		if err != nil {
			return fmt.Errorf("delete proxysql pvc: %v", err)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deletePVC(namespace string, lbls map[string]string) error {
	list := corev1.PersistentVolumeClaimList{}
	err := r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(lbls),
		},
		&list,
	)
	if err != nil {
		return fmt.Errorf("get list: %v", err)
	}

	for _, pvc := range list.Items {
		err := r.client.Delete(context.TODO(), &pvc)
		if err != nil {
			return fmt.Errorf("delete: %v", err)
		}
	}

	return nil
}

func setControllerReference(ro runtime.Object, obj metav1.Object, scheme *runtime.Scheme) error {
	ownerRef, err := OwnerRef(ro, scheme)
	if err != nil {
		return err
	}
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ownerRef))
	return nil
}

// OwnerRef returns OwnerReference to object
func OwnerRef(ro runtime.Object, scheme *runtime.Scheme) (metav1.OwnerReference, error) {
	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return metav1.OwnerReference{}, err
	}

	trueVar := true

	ca, err := meta.Accessor(ro)
	if err != nil {
		return metav1.OwnerReference{}, err
	}

	return metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       ca.GetName(),
		UID:        ca.GetUID(),
		Controller: &trueVar,
	}, nil
}
