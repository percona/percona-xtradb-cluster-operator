package pxc

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	cm "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
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
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			return rr, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	changed, err := o.CheckNSetDefaults()
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("wrong PXC options: %v", err)
	}

	// update CR if there was changes that may be read by another cr (e.g. pxc-backup)
	if changed {
		err = r.client.Update(context.TODO(), o)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("update PXC CR: %v", err)
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
		return reconcile.Result{}, fmt.Errorf("pxc not specified")
	}

	err = r.deploy(o)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.updatePod(statefulset.NewNode(o), o.Spec.PXC, o)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("pxc upgrade error: %v", err)
	}

	proxysqlSet := statefulset.NewProxy(o)
	if o.Spec.ProxySQL != nil && o.Spec.ProxySQL.Enabled {
		err = r.updatePod(proxysqlSet, o.Spec.ProxySQL, o)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("ProxySQL upgrade error: %v", err)
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

	err = r.updateStatus(o)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("update status: %v", err)
	}

	return rr, nil
}

func (r *ReconcilePerconaXtraDBCluster) deploy(cr *api.PerconaXtraDBCluster) error {
	serverVersion := r.serverVersion
	if cr.Spec.Platform != nil {
		serverVersion.Platform = *cr.Spec.Platform
	}

	stsApp := statefulset.NewNode(cr)
	if cr.Spec.PXC.Configuration != "" {
		ls := stsApp.Labels()
		configMap := configmap.NewConfigMap(cr, ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"])
		err := setControllerReference(cr, configMap, r.scheme)
		if err != nil {
			return err
		}

		err = r.client.Create(context.TODO(), configMap)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create newConfigMap: %v", err)
		}
	}

	nodeSet, err := pxc.StatefulSet(stsApp, cr.Spec.PXC, cr, serverVersion)
	if err != nil {
		return err
	}

	err = r.reconsileSSL(cr, nodeSet.Namespace)
	if err != nil {
		return fmt.Errorf(`TLS secrets handler: "%v". Please create your TLS secret `+cr.Spec.PXC.SSLSecretName+` and `+cr.Spec.PXC.SSLInternalSecretName+` manually or setup cert-manager correctly`, err)
	}

	err = setControllerReference(cr, nodeSet, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Create(context.TODO(), nodeSet)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create newStatefulSetNode: %v", err)
	}

	nodesServiceUnready := pxc.NewServicePXCUnready(cr)
	err = setControllerReference(cr, nodesServiceUnready, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Create(context.TODO(), nodesServiceUnready)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create PXC Service: %v", err)
	}

	nodesService := pxc.NewServicePXC(cr)
	err = setControllerReference(cr, nodesService, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Create(context.TODO(), nodesService)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create PXC Service: %v", err)
	}

	// PodDisruptionBudget object for nodes
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: nodeSet.Name, Namespace: nodeSet.Namespace}, nodeSet)
	if err == nil {
		err := r.reconcilePDB(cr.Spec.PXC.PodDisruptionBudget, stsApp, cr.Namespace, nodeSet)
		if err != nil {
			return fmt.Errorf("PodDisruptionBudget for %s: %v", nodeSet.Name, err)
		}
	} else if !errors.IsNotFound(err) {
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

		err = r.client.Create(context.TODO(), proxySet)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create newStatefulSetProxySQL: %v", err)
		}

		// ProxySQL Service
		proxys := pxc.NewServiceProxySQL(cr)
		err = setControllerReference(cr, proxys, r.scheme)
		if err != nil {
			return err
		}

		err = r.client.Create(context.TODO(), proxys)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create ProxySQL Service: %v", err)
		}

		// ProxySQL Unready Service
		proxysh := pxc.NewServiceProxySQLUnready(cr)
		err = setControllerReference(cr, proxysh, r.scheme)
		if err != nil {
			return err
		}

		err = r.client.Create(context.TODO(), proxysh)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create ProxySQL Unready Service: %v", err)
		}

		// PodDisruptionBudget object for ProxySQL
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: proxySet.Name, Namespace: proxySet.Namespace}, proxySet)
		if err == nil {
			err := r.reconcilePDB(cr.Spec.ProxySQL.PodDisruptionBudget, sfsProxy, cr.Namespace, proxySet)
			if err != nil {
				return fmt.Errorf("PodDisruptionBudget for %s: %v", proxySet.Name, err)
			}
		} else if !errors.IsNotFound(err) {
			return fmt.Errorf("get ProxySQL stateful set: %v", err)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconsileSSL(cr *api.PerconaXtraDBCluster, namespace string) error {
	if cr.Spec.PXC.AllowUnsafeConfig {
		return nil
	}
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: namespace,
			Name:      cr.Spec.PXC.SSLSecretName,
		},
		&secretObj,
	)
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("get secret: %v", err)
	}

	issuerKind := "Issuer"
	issuerName := cr.Name + "-pxc-ca"

	err = r.client.Create(context.TODO(), &cm.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      issuerName,
			Namespace: namespace,
		},
		Spec: cm.IssuerSpec{
			IssuerConfig: cm.IssuerConfig{
				SelfSigned: &cm.SelfSignedIssuer{},
			},
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create issuer: %v", err)
	}

	err = r.client.Create(context.TODO(), &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ssl",
			Namespace: namespace,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLSecretName,
			CommonName: cr.Name + "-proxysql",
			DNSNames: []string{
				"*." + cr.Name + "-proxysql",
			},
			IsCA: true,
			IssuerRef: cm.ObjectReference{
				Name: issuerName,
				Kind: issuerKind,
			},
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create certificate: %v", err)
	}

	if cr.Spec.PXC.SSLSecretName == cr.Spec.PXC.SSLInternalSecretName {
		return nil
	}

	err = r.client.Create(context.TODO(), &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ssl-internal",
			Namespace: namespace,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLInternalSecretName,
			CommonName: cr.Name + "-pxc",
			DNSNames: []string{
				"*." + cr.Name + "-pxc",
			},
			IsCA: true,
			IssuerRef: cm.ObjectReference{
				Name: issuerName,
				Kind: issuerKind,
			},
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create internal certificate: %v", err)
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
	if err != nil && errors.IsNotFound(err) {
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
	if err != nil && !errors.IsNotFound(err) {
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
