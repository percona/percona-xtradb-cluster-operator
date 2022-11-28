package pxc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cm "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/config"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/version"
)

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
		return nil, errors.Wrap(err, "get version")
	}

	cli, err := clientcmd.NewClient()
	if err != nil {
		return nil, errors.Wrap(err, "create clientcmd")
	}

	zapLog, err := zap.NewProduction()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create logger")
	}

	return &ReconcilePerconaXtraDBCluster{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		crons:         NewCronRegistry(),
		serverVersion: sv,
		clientcmd:     cli,
		lockers:       newLockStore(),
		log:           zapr.NewLogger(zapLog),
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
	client         client.Client
	scheme         *runtime.Scheme
	crons          CronRegistry
	clientcmd      *clientcmd.Client
	syncUsersState int32
	serverVersion  *version.ServerVersion
	lockers        lockStore
	log            logr.Logger
}

func (r *ReconcilePerconaXtraDBCluster) logger(name, namespace string) logr.Logger {
	return r.log.WithName("perconaxtradbcluster").WithValues("cluster", name, "namespace", namespace)
}

type lockStore struct {
	store *sync.Map
}

func newLockStore() lockStore {
	return lockStore{
		store: new(sync.Map),
	}
}

func (l lockStore) LoadOrCreate(key string) lock {
	val, _ := l.store.LoadOrStore(key, lock{
		statusMutex: new(sync.Mutex),
		updateSync:  new(int32),
	})

	return val.(lock)
}

type lock struct {
	statusMutex *sync.Mutex
	updateSync  *int32
}

const (
	updateDone = 0
	updateWait = 1
)

type CronRegistry struct {
	crons             *cron.Cron
	ensureVersionJobs map[string]Schedule
	backupJobs        *sync.Map
}

// AddFuncWithSeconds does the same as cron.AddFunc but changes the schedule so that the function will run the exact second that this method is called.
func (r *CronRegistry) AddFuncWithSeconds(spec string, cmd func()) (cron.EntryID, error) {
	schedule, err := cron.ParseStandard(spec)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse cron schedule")
	}
	schedule.(*cron.SpecSchedule).Second = uint64(1 << time.Now().Second())
	id := r.crons.Schedule(schedule, cron.FuncJob(cmd))
	return id, nil
}

type Schedule struct {
	ID           int
	CronSchedule string
}

const (
	stateFree   = 0
	stateLocked = 1
)

func NewCronRegistry() CronRegistry {
	c := CronRegistry{
		crons:             cron.New(),
		ensureVersionJobs: make(map[string]Schedule),
		backupJobs:        new(sync.Map),
	}

	c.crons.Start()

	return c
}

// Reconcile reads that state of the cluster for a PerconaXtraDBCluster object and makes changes based on the state read
// and what is in the PerconaXtraDBCluster.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcilePerconaXtraDBCluster) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rr := reconcile.Result{
		RequeueAfter: time.Second * 5,
	}

	// As operator can handle a few clusters
	// lock should be created per cluster to not lock cron jobs of other clusters
	l := r.lockers.LoadOrCreate(request.NamespacedName.String())

	// Fetch the PerconaXtraDBCluster instance
	// PerconaXtraDBCluster object is also accessed and changed by a version service's cron job (that run concurrently)
	l.statusMutex.Lock()
	defer l.statusMutex.Unlock()
	// we have to be sure the reconcile loop will be run at least once
	// in-between any version service jobs (hence any two vs jobs shouldn't be run sequentially).
	// the version service job sets the state to  `updateWait` and the next job can be run only
	// after the state was dropped to`updateDone` again
	defer atomic.StoreInt32(l.updateSync, updateDone)

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

	if err := r.setCRVersion(ctx, o); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "set CR version")
	}

	reqLogger := r.logger(o.Name, o.Namespace)

	if o.ObjectMeta.DeletionTimestamp != nil {
		finalizers := []string{}
		for _, fnlz := range o.GetFinalizers() {
			var sfs api.StatefulApp
			switch fnlz {
			case "delete-ssl":
				err = r.deleteCerts(o)
			case "delete-proxysql-pvc":
				sfs = statefulset.NewProxy(o)
				// deletePVC is always true on this stage
				// because we never reach this point without finalizers
				err = r.deleteStatefulSet(o, sfs, true)
			case "delete-pxc-pvc":
				sfs = statefulset.NewNode(o)
				err = r.deleteStatefulSet(o, sfs, true)
			// nil error gonna be returned only when there is no more pods to delete (only 0 left)
			// until than finalizer won't be deleted
			case "delete-pxc-pods-in-order":
				err = r.deletePXCPods(o)
			}
			if err != nil {
				finalizers = append(finalizers, fnlz)
			}
		}

		o.SetFinalizers(finalizers)
		err = r.client.Update(context.TODO(), o)

		// object is being deleted, no need in further actions
		return rr, err
	}

	// wait until token issued to run PXC in data encrypted mode.
	if o.ShouldWaitForTokenIssue() {
		reqLogger.Info("wait for token issuing")
		return rr, nil
	}

	defer func() {
		uerr := r.updateStatus(o, false, err)
		if uerr != nil {
			reqLogger.Error(uerr, "Update status")
		}
	}()

	err = o.CheckNSetDefaults(r.serverVersion, reqLogger)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "wrong PXC options")
	}

	if o.CompareVersionWith("1.7.0") >= 0 && *o.Spec.PXC.AutoRecovery {
		err = r.recoverFullClusterCrashIfNeeded(o)
		if err != nil {
			reqLogger.Info("Failed to check if cluster needs to recover", "err", err.Error())
		}
	}

	err = r.reconcileUsersSecret(o)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "reconcile users secret")
	}

	userReconcileResult := &ReconcileUsersResult{}

	urr, err := r.reconcileUsers(o)
	if err != nil {
		return rr, errors.Wrap(err, "reconcile users")
	}
	if urr != nil {
		userReconcileResult = urr
	}

	r.resyncPXCUsersWithProxySQL(o)

	if o.Status.PXC.Version == "" || strings.HasSuffix(o.Status.PXC.Version, "intermediate") {
		err := r.ensurePXCVersion(o, VersionServiceClient{OpVersion: o.Version().String()})
		if err != nil {
			reqLogger.Info("failed to ensure version, running with default", "error", err)
		}
	}

	err = r.deploy(o)
	if err != nil {
		return reconcile.Result{}, err
	}

	operatorPod, err := k8s.OperatorPod(r.client)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "get operator deployment")
	}

	inits := []corev1.Container{}
	if o.CompareVersionWith("1.5.0") >= 0 {
		var imageName string
		if len(o.Spec.InitImage) > 0 {
			imageName = o.Spec.InitImage
		} else {
			imageName, err = operatorImageName(&operatorPod)
			if err != nil {
				return reconcile.Result{}, err
			}
			if o.CompareVersionWith(version.Version) != 0 {
				imageName = strings.Split(imageName, ":")[0] + ":" + o.Spec.CRVersion
			}
		}
		var initResources corev1.ResourceRequirements
		if o.CompareVersionWith("1.6.0") >= 0 {
			initResources = o.Spec.PXC.Resources
		}
		initC := statefulset.EntrypointInitContainer(imageName, initResources, o.Spec.PXC.ContainerSecurityContext, o.Spec.PXC.ImagePullPolicy)
		inits = append(inits, initC)
	}

	pxcSet := statefulset.NewNode(o)
	pxc.MergeTemplateAnnotations(pxcSet.StatefulSet(), userReconcileResult.pxcAnnotations)
	err = r.updatePod(pxcSet, o.Spec.PXC.PodSpec, o, inits)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "pxc upgrade error")
	}

	for _, pxcService := range []*corev1.Service{pxc.NewServicePXC(o), pxc.NewServicePXCUnready(o)} {
		err := setControllerReference(o, pxcService, r.scheme)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "setControllerReference")
		}

		err = r.createOrUpdateService(o, pxcService, true)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "PXC service upgrade error")
		}
	}

	if o.Spec.PXC.Expose.Enabled {
		err = r.ensurePxcPodServices(o)
		if err != nil {
			return rr, errors.Wrap(err, "create replication services")
		}
	} else {
		err = r.removePxcPodServices(o)
		if err != nil {
			return rr, errors.Wrap(err, "remove pxc pod services")
		}
	}

	if err := r.reconcileHAProxy(o); err != nil {
		return reconcile.Result{}, err
	}

	proxysqlSet := statefulset.NewProxy(o)
	pxc.MergeTemplateAnnotations(proxysqlSet.StatefulSet(), userReconcileResult.proxysqlAnnotations)

	if o.Spec.ProxySQL != nil && o.Spec.ProxySQL.Enabled {
		err = r.updatePod(proxysqlSet, o.Spec.ProxySQL, o, nil)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "ProxySQL upgrade error")
		}
		svc := pxc.NewServiceProxySQL(o)
		err := setControllerReference(o, svc, r.scheme)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "%s setControllerReference", svc.Name)
		}
		err = r.createOrUpdateService(o, svc, len(o.Spec.ProxySQL.ServiceLabels) == 0 && len(o.Spec.ProxySQL.ServiceAnnotations) == 0)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "%s upgrade error", svc.Name)
		}
		svc = pxc.NewServiceProxySQLUnready(o)
		err = setControllerReference(o, svc, r.scheme)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "%s setControllerReference", svc.Name)
		}
		err = r.createOrUpdateService(o, svc, true)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "%s upgrade error", svc.Name)
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

		err = r.deleteStatefulSet(o, proxysqlSet, deletePVC)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = r.deleteServices(pxc.NewServiceProxySQL(o), pxc.NewServiceProxySQLUnready(o))
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if o.CompareVersionWith("1.9.0") >= 0 {
		err = r.reconcileReplication(o, userReconcileResult.updateReplicationPassword)
		if err != nil {
			reqLogger.Info("reconcile replication error", "err", err.Error())
		}
	}

	err = r.reconcileBackups(o)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.checkPITRErrors(context.TODO(), o)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.fetchVersionFromPXC(o, pxcSet); err != nil {
		return rr, errors.Wrap(err, "update CR version")
	}

	err = r.scheduleEnsurePXCVersion(o, VersionServiceClient{OpVersion: o.Version().String()})
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to ensure version")
	}

	return rr, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileHAProxy(cr *api.PerconaXtraDBCluster) error {
	if !cr.HAProxyEnabled() {
		if err := r.deleteServices(pxc.NewServiceHAProxyReplicas(cr)); err != nil {
			return errors.Wrap(err, "delete HAProxy replica service")
		}

		if err := r.deleteServices(pxc.NewServiceHAProxy(cr)); err != nil {
			return errors.Wrap(err, "delete HAProxy service")
		}

		if err := r.deleteStatefulSet(cr, statefulset.NewHAProxy(cr), false); err != nil {
			return errors.Wrap(err, "delete HAProxy stateful set")
		}

		return nil
	}

	if err := r.updatePod(statefulset.NewHAProxy(cr), &cr.Spec.HAProxy.PodSpec, cr, nil); err != nil {
		return errors.Wrap(err, "HAProxy upgrade error")
	}
	svc := pxc.NewServiceHAProxy(cr)
	err := setControllerReference(cr, svc, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "%s setControllerReference", svc.Name)
	}
	podSpec := cr.Spec.HAProxy.PodSpec
	err = r.createOrUpdateService(cr, svc, len(podSpec.ServiceLabels) == 0 && len(podSpec.ServiceAnnotations) == 0)
	if err != nil {
		return errors.Wrapf(err, "%s upgrade error", svc.Name)
	}
	if cr.HAProxyReplicasServiceEnabled() {
		svc := pxc.NewServiceHAProxyReplicas(cr)
		err := setControllerReference(cr, svc, r.scheme)
		if err != nil {
			return errors.Wrapf(err, "%s setControllerReference", svc.Name)
		}
		err = r.createOrUpdateService(cr, svc, len(podSpec.ReplicasServiceLabels) == 0 && len(podSpec.ReplicasServiceAnnotations) == 0)
		if err != nil {
			return errors.Wrapf(err, "%s upgrade error", svc.Name)
		}
	} else {
		if err := r.deleteServices(pxc.NewServiceHAProxyReplicas(cr)); err != nil {
			return errors.Wrap(err, "delete HAProxy replica service")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deploy(cr *api.PerconaXtraDBCluster) error {
	stsApp := statefulset.NewNode(cr)
	err := r.reconcileConfigMap(cr)
	if err != nil {
		return err
	}

	operatorPod, err := k8s.OperatorPod(r.client)
	if err != nil {
		return errors.Wrap(err, "get operator deployment")
	}

	logger := r.logger(cr.Name, cr.Namespace)
	inits := []corev1.Container{}
	if cr.CompareVersionWith("1.5.0") >= 0 {
		var imageName string
		if len(cr.Spec.InitImage) > 0 {
			imageName = cr.Spec.InitImage
		} else {
			imageName, err = operatorImageName(&operatorPod)
			if err != nil {
				return err
			}
			if cr.CompareVersionWith(version.Version) != 0 {
				imageName = strings.Split(imageName, ":")[0] + ":" + cr.Spec.CRVersion
			}
		}
		var initResources corev1.ResourceRequirements
		if cr.CompareVersionWith("1.6.0") >= 0 {
			initResources = cr.Spec.PXC.Resources
		}
		initC := statefulset.EntrypointInitContainer(imageName, initResources, cr.Spec.PXC.ContainerSecurityContext, cr.Spec.PXC.ImagePullPolicy)
		inits = append(inits, initC)
	}

	secretsName := cr.Spec.SecretsName
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secretsName = "internal-" + cr.Name
	}
	secrets := new(corev1.Secret)
	err = r.client.Get(context.TODO(), types.NamespacedName{
		Name: secretsName, Namespace: cr.Namespace,
	}, secrets)
	if client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "get internal secret")
	}
	nodeSet, err := pxc.StatefulSet(stsApp, cr.Spec.PXC.PodSpec, cr, secrets, inits, logger, r.getConfigVolume)
	if err != nil {
		return errors.Wrap(err, "get pxc statefulset")
	}
	currentNodeSet := new(appsv1.StatefulSet)
	err = r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: nodeSet.Namespace,
		Name:      nodeSet.Name,
	}, currentNodeSet)
	if client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "get current pxc sts")
	}

	// TODO: code duplication with updatePod function
	if nodeSet.Spec.Template.Annotations == nil {
		nodeSet.Spec.Template.Annotations = make(map[string]string)
	}
	if v, ok := currentNodeSet.Spec.Template.Annotations["last-applied-secret"]; ok {
		nodeSet.Spec.Template.Annotations["last-applied-secret"] = v
	}
	if cr.CompareVersionWith("1.1.0") >= 0 {
		hash, err := r.getConfigHash(cr, stsApp)
		if err != nil {
			return errors.Wrap(err, "getting node config hash")
		}
		nodeSet.Spec.Template.Annotations["percona.com/configuration-hash"] = hash
	}

	err = r.reconcileSSL(cr)
	if err != nil {
		return errors.Wrapf(err, "failed to reconcile SSL.Please create your TLS secret %s and %s manually or setup cert-manager correctly",
			cr.Spec.PXC.SSLSecretName, cr.Spec.PXC.SSLInternalSecretName)
	}

	sslHash, err := r.getSecretHash(cr, cr.Spec.PXC.SSLSecretName, cr.Spec.AllowUnsafeConfig)
	if err != nil {
		return errors.Wrap(err, "get secret hash")
	}
	if sslHash != "" && cr.CompareVersionWith("1.1.0") >= 0 {
		nodeSet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash
	}

	sslInternalHash, err := r.getSecretHash(cr, cr.Spec.PXC.SSLInternalSecretName, cr.Spec.AllowUnsafeConfig)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get internal secret hash")
	}
	if sslInternalHash != "" && cr.CompareVersionWith("1.1.0") >= 0 {
		nodeSet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		envVarsHash, err := r.getSecretHash(cr, cr.Spec.PXC.EnvVarsSecretName, true)
		if err != nil {
			return errors.Wrap(err, "upgradePod/updateApp error: update secret error")
		}
		if envVarsHash != "" {
			nodeSet.Spec.Template.Annotations["percona.com/env-secret-config-hash"] = envVarsHash
		}
	}

	vaultConfigHash, err := r.getSecretHash(cr, cr.Spec.VaultSecretName, true)
	if err != nil {
		return errors.Wrap(err, "get vault config hash")
	}
	if vaultConfigHash != "" && cr.CompareVersionWith("1.6.0") >= 0 {
		nodeSet.Spec.Template.Annotations["percona.com/vault-config-hash"] = vaultConfigHash
	}
	nodeSet.Spec.Template.Spec.Tolerations = cr.Spec.PXC.Tolerations
	err = setControllerReference(cr, nodeSet, r.scheme)
	if err != nil {
		return err
	}

	err = r.createOrUpdate(cr, nodeSet)
	if err != nil {
		return errors.Wrap(err, "create newStatefulSetNode")
	}

	// PodDisruptionBudget object for nodes
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: nodeSet.Name, Namespace: nodeSet.Namespace}, nodeSet)
	if err == nil {
		err := r.reconcilePDB(cr, cr.Spec.PXC.PodDisruptionBudget, stsApp, nodeSet)
		if err != nil {
			return errors.Wrapf(err, "PodDisruptionBudget for %s", nodeSet.Name)
		}
	} else if !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get PXC stateful set")
	}

	// HAProxy StatefulSet
	if cr.HAProxyEnabled() {
		sfsHAProxy := statefulset.NewHAProxy(cr)
		haProxySet, err := pxc.StatefulSet(sfsHAProxy, &cr.Spec.HAProxy.PodSpec, cr, secrets, nil, logger, r.getConfigVolume)
		if err != nil {
			return errors.Wrap(err, "create HAProxy StatefulSet")
		}
		err = setControllerReference(cr, haProxySet, r.scheme)
		if err != nil {
			return err
		}

		// TODO: code duplication with updatePod function
		if haProxySet.Spec.Template.Annotations == nil {
			haProxySet.Spec.Template.Annotations = make(map[string]string)
		}
		hash, err := r.getConfigHash(cr, sfsHAProxy)
		if err != nil {
			return errors.Wrap(err, "getting HAProxy config hash")
		}
		haProxySet.Spec.Template.Annotations["percona.com/configuration-hash"] = hash
		if cr.CompareVersionWith("1.5.0") == 0 {
			if sslHash != "" {
				haProxySet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash
			}
			if sslInternalHash != "" {
				haProxySet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
			}
		}
		if cr.CompareVersionWith("1.9.0") >= 0 {
			envVarsHash, err := r.getSecretHash(cr, cr.Spec.HAProxy.EnvVarsSecretName, true)
			if err != nil {
				return errors.Wrap(err, "upgradePod/updateApp error: update secret error")
			}
			if envVarsHash != "" {
				haProxySet.Spec.Template.Annotations["percona.com/env-secret-config-hash"] = envVarsHash
			}
		}
		err = r.client.Create(context.TODO(), haProxySet)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "create newStatefulSetHAProxy")
		}

		// PodDisruptionBudget object for HAProxy
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: haProxySet.Name, Namespace: haProxySet.Namespace}, haProxySet)
		if err == nil {
			err := r.reconcilePDB(cr, cr.Spec.HAProxy.PodDisruptionBudget, sfsHAProxy, haProxySet)
			if err != nil {
				return errors.Wrapf(err, "PodDisruptionBudget for %s", haProxySet.Name)
			}
		} else if !k8serrors.IsNotFound(err) {
			return errors.Wrap(err, "get HAProxy stateful set")
		}
	}

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		sfsProxy := statefulset.NewProxy(cr)
		proxySet, err := pxc.StatefulSet(sfsProxy, cr.Spec.ProxySQL, cr, secrets, nil, logger, r.getConfigVolume)
		if err != nil {
			return errors.Wrap(err, "create ProxySQL Service")
		}
		err = setControllerReference(cr, proxySet, r.scheme)
		if err != nil {
			return err
		}
		currentProxySet := new(appsv1.StatefulSet)
		err = r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: nodeSet.Namespace,
			Name:      nodeSet.Name,
		}, currentProxySet)
		if client.IgnoreNotFound(err) != nil {
			return errors.Wrap(err, "get current proxy sts")
		}

		// TODO: code duplication with updatePod function
		if proxySet.Spec.Template.Annotations == nil {
			proxySet.Spec.Template.Annotations = make(map[string]string)
		}
		if v, ok := currentProxySet.Spec.Template.Annotations["last-applied-secret"]; ok {
			proxySet.Spec.Template.Annotations["last-applied-secret"] = v
		}
		if cr.CompareVersionWith("1.1.0") >= 0 {
			hash, err := r.getConfigHash(cr, sfsProxy)
			if err != nil {
				return errors.Wrap(err, "getting proxySQL config hash")
			}
			proxySet.Spec.Template.Annotations["percona.com/configuration-hash"] = hash
			if sslHash != "" {
				proxySet.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash
			}
			if sslInternalHash != "" {
				proxySet.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
			}
		}
		if cr.CompareVersionWith("1.9.0") >= 0 {
			envVarsHash, err := r.getSecretHash(cr, cr.Spec.ProxySQL.EnvVarsSecretName, true)
			if err != nil {
				return errors.Wrap(err, "upgradePod/updateApp error: update secret error")
			}
			if envVarsHash != "" {
				proxySet.Spec.Template.Annotations["percona.com/env-secret-config-hash"] = envVarsHash
			}
		}
		err = r.client.Create(context.TODO(), proxySet)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "create newStatefulSetProxySQL")
		}

		// PodDisruptionBudget object for ProxySQL
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: proxySet.Name, Namespace: proxySet.Namespace}, proxySet)
		if err == nil {
			err := r.reconcilePDB(cr, cr.Spec.ProxySQL.PodDisruptionBudget, sfsProxy, proxySet)
			if err != nil {
				return errors.Wrapf(err, "PodDisruptionBudget for %s", proxySet.Name)
			}
		} else if !k8serrors.IsNotFound(err) {
			return errors.Wrap(err, "get ProxySQL stateful set")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileConfigMap(cr *api.PerconaXtraDBCluster) error {
	stsApp := statefulset.NewNode(cr)
	ls := stsApp.Labels()

	if cr.CompareVersionWith("1.3.0") >= 0 {
		autotuneCm := "auto-" + ls["app.kubernetes.io/instance"] + "-" + ls["app.kubernetes.io/component"]

		_, ok := cr.Spec.PXC.Resources.Limits[corev1.ResourceMemory]
		if ok {
			configMap, err := config.NewAutoTuneConfigMap(cr, cr.Spec.PXC.Resources.Limits.Memory(), autotuneCm)
			if err != nil {
				return errors.Wrap(err, "new autotune configmap")
			}

			err = setControllerReference(cr, configMap, r.scheme)
			if err != nil {
				return errors.Wrap(err, "set autotune configmap controller ref")
			}

			err = createOrUpdateConfigmap(r.client, configMap)
			if err != nil {
				return errors.Wrap(err, "create or update autotune configmap")
			}
		} else {
			if err := deleteConfigMapIfExists(r.client, cr, autotuneCm); err != nil {
				return errors.Wrap(err, "delete autotune configmap")
			}
		}
	}

	pxcConfigName := ls["app.kubernetes.io/instance"] + "-" + ls["app.kubernetes.io/component"]

	if cr.Spec.PXC.Configuration != "" {
		configMap := config.NewConfigMap(cr, pxcConfigName, "init.cnf", cr.Spec.PXC.Configuration)
		err := setControllerReference(cr, configMap, r.scheme)
		if err != nil {
			return errors.Wrap(err, "set controller ref")
		}

		err = createOrUpdateConfigmap(r.client, configMap)
		if err != nil {
			return errors.Wrap(err, "pxc config map")
		}
	} else {
		if err := deleteConfigMapIfExists(r.client, cr, pxcConfigName); err != nil {
			return errors.Wrap(err, "delete pxc config map")
		}
	}

	if cr.CompareVersionWith("1.11.0") >= 0 {
		pxcHookScriptName := ls["app.kubernetes.io/instance"] + "-" + ls["app.kubernetes.io/component"] + "-hookscript"
		if cr.Spec.PXC != nil && cr.Spec.PXC.HookScript != "" {
			err := r.createHookScriptConfigMap(cr, cr.Spec.PXC.PodSpec.HookScript, pxcHookScriptName)
			if err != nil {
				return errors.Wrap(err, "create pxc hookscript config map")
			}
		} else {
			if err := deleteConfigMapIfExists(r.client, cr, pxcHookScriptName); err != nil {
				return errors.Wrap(err, "delete pxc hookscript config map")
			}
		}

		proxysqlHookScriptName := ls["app.kubernetes.io/instance"] + "-proxysql" + "-hookscript"
		if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.HookScript != "" {
			err := r.createHookScriptConfigMap(cr, cr.Spec.ProxySQL.HookScript, proxysqlHookScriptName)
			if err != nil {
				return errors.Wrap(err, "create proxysql hookscript config map")
			}
		} else {
			if err := deleteConfigMapIfExists(r.client, cr, proxysqlHookScriptName); err != nil {
				return errors.Wrap(err, "delete proxysql hookscript config map")
			}
		}
		haproxyHookScriptName := ls["app.kubernetes.io/instance"] + "-haproxy" + "-hookscript"
		if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.HookScript != "" {
			err := r.createHookScriptConfigMap(cr, cr.Spec.HAProxy.PodSpec.HookScript, haproxyHookScriptName)
			if err != nil {
				return errors.Wrap(err, "create haproxy hookscript config map")
			}
		} else {
			if err := deleteConfigMapIfExists(r.client, cr, haproxyHookScriptName); err != nil {
				return errors.Wrap(err, "delete haproxy config map")
			}
		}
		logCollectorHookScriptName := ls["app.kubernetes.io/instance"] + "-logcollector" + "-hookscript"
		if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.HookScript != "" {
			err := r.createHookScriptConfigMap(cr, cr.Spec.LogCollector.HookScript, logCollectorHookScriptName)
			if err != nil {
				return errors.Wrap(err, "create logcollector hookscript config map")
			}
		} else {
			if err := deleteConfigMapIfExists(r.client, cr, logCollectorHookScriptName); err != nil {
				return errors.Wrap(err, "delete logcollector config map")
			}
		}
	}

	proxysqlConfigName := ls["app.kubernetes.io/instance"] + "-proxysql"

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled && cr.Spec.ProxySQL.Configuration != "" {
		configMap := config.NewConfigMap(cr, proxysqlConfigName, "proxysql.cnf", cr.Spec.ProxySQL.Configuration)
		err := setControllerReference(cr, configMap, r.scheme)
		if err != nil {
			return errors.Wrap(err, "set controller ref ProxySQL")
		}

		err = createOrUpdateConfigmap(r.client, configMap)
		if err != nil {
			return errors.Wrap(err, "proxysql config map")
		}
	} else {
		if err := deleteConfigMapIfExists(r.client, cr, proxysqlConfigName); err != nil {
			return errors.Wrap(err, "delete proxySQL config map")
		}
	}

	haproxyConfigName := ls["app.kubernetes.io/instance"] + "-haproxy"

	if cr.HAProxyEnabled() && cr.Spec.HAProxy.Configuration != "" {
		configMap := config.NewConfigMap(cr, haproxyConfigName, "haproxy-global.cfg", cr.Spec.HAProxy.Configuration)
		err := setControllerReference(cr, configMap, r.scheme)
		if err != nil {
			return errors.Wrap(err, "set controller ref HAProxy")
		}

		err = createOrUpdateConfigmap(r.client, configMap)
		if err != nil {
			return errors.Wrap(err, "haproxy config map")
		}
	} else {
		if err := deleteConfigMapIfExists(r.client, cr, haproxyConfigName); err != nil {
			return errors.Wrap(err, "delete haproxy config map")
		}
	}

	if cr.CompareVersionWith("1.7.0") >= 0 {
		logCollectorConfigName := ls["app.kubernetes.io/instance"] + "-logcollector"

		if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Configuration != "" {
			configMap := config.NewConfigMap(cr, logCollectorConfigName, "fluentbit_custom.conf", cr.Spec.LogCollector.Configuration)
			err := setControllerReference(cr, configMap, r.scheme)
			if err != nil {
				return errors.Wrap(err, "set controller ref LogCollector")
			}
			err = createOrUpdateConfigmap(r.client, configMap)
			if err != nil {
				return errors.Wrap(err, "logcollector config map")
			}
		} else {
			if err := deleteConfigMapIfExists(r.client, cr, logCollectorConfigName); err != nil {
				return errors.Wrap(err, "delete log collector config map")
			}
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) createHookScriptConfigMap(cr *api.PerconaXtraDBCluster, hookScript string, configMapName string) error {
	configMap := config.NewConfigMap(cr, configMapName, "hook.sh", hookScript)
	err := setControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return errors.Wrap(err, "set controller ref")
	}

	err = createOrUpdateConfigmap(r.client, configMap)
	if err != nil {
		return errors.Wrap(err, "create or update configmap")
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcilePDB(cr *api.PerconaXtraDBCluster, spec *api.PodDisruptionBudgetSpec, sfs api.StatefulApp, owner runtime.Object) error {
	if spec == nil {
		return nil
	}

	pdb := pxc.PodDisruptionBudget(spec, sfs.Labels(), cr.Namespace)
	err := setControllerReference(owner, pdb, r.scheme)
	if err != nil {
		return errors.Wrap(err, "set owner reference")
	}

	return errors.Wrap(r.createOrUpdate(cr, pdb), "reconcile pdb")
}

func (r *ReconcilePerconaXtraDBCluster) deletePXCPods(cr *api.PerconaXtraDBCluster) error {
	sfs := statefulset.NewNode(cr)
	err := r.deleteStatefulSetPods(cr.Namespace, sfs)
	if err != nil {
		return errors.Wrap(err, "delete statefulset pods")
	}
	if cr.Spec.Backup != nil && cr.Spec.Backup.PITR.Enabled {
		return errors.Wrap(r.deletePITR(cr), "delete pitr pod")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deleteStatefulSetPods(namespace string, sfs api.StatefulApp) error {
	list := corev1.PodList{}

	err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
	)
	if err != nil {
		return errors.Wrap(err, "get pod list")
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
		return errors.Wrap(err, "get StatefulSet")
	}

	if cSet.Spec.Replicas == nil || *cSet.Spec.Replicas != 1 {
		dscaleTo := int32(1)
		cSet.Spec.Replicas = &dscaleTo
		err = r.client.Update(context.TODO(), cSet)
		if err != nil {
			return errors.Wrap(err, "downscale StatefulSet")
		}
	}
	return errors.New("waiting for pods to be deleted")
}

func (r *ReconcilePerconaXtraDBCluster) deleteStatefulSet(cr *api.PerconaXtraDBCluster, sfs api.StatefulApp, deletePVC bool) error {
	sfsWithOwner := appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Name:      sfs.StatefulSet().Name,
		Namespace: cr.Namespace,
	}, &sfsWithOwner)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrapf(err, "get statefulset: %s", sfs.StatefulSet().Name)
	}

	if k8serrors.IsNotFound(err) {
		return nil
	}

	if !metav1.IsControlledBy(&sfsWithOwner, cr) {
		return nil
	}

	err = r.client.Delete(context.TODO(), &sfsWithOwner, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &sfsWithOwner.UID}})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrapf(err, "delete statefulset: %s", sfs.StatefulSet().Name)
	}
	if deletePVC {
		err = r.deletePVC(cr.Namespace, sfs.Labels())
		if err != nil {
			return errors.Wrapf(err, "delete pvc: %s", sfs.StatefulSet().Name)
		}

		err = r.deleteSecrets(cr)
		if err != nil {
			return errors.Wrap(err, "delete secrets")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deleteServices(svcs ...*corev1.Service) error {
	for _, s := range svcs {
		err := r.client.Get(context.TODO(), types.NamespacedName{
			Name:      s.Name,
			Namespace: s.Namespace,
		}, &corev1.Service{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Wrapf(err, "get service: %s", s.Name)
		}

		if k8serrors.IsNotFound(err) {
			continue
		}

		err = r.client.Delete(context.TODO(), s)
		if err != nil {
			return errors.Wrapf(err, "delete service: %s", s.Name)
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deletePVC(namespace string, lbls map[string]string) error {
	list := corev1.PersistentVolumeClaimList{}
	err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(lbls),
		},
	)
	if err != nil {
		return errors.Wrap(err, "get PVC list")
	}

	for _, pvc := range list.Items {
		err := r.client.Delete(context.TODO(), &pvc, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pvc.UID}})
		if err != nil {
			return errors.Wrapf(err, "delete PVC %s", pvc.Name)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deleteSecrets(cr *api.PerconaXtraDBCluster) error {
	secrets := []string{cr.Spec.SecretsName, "internal-" + cr.Name}

	for _, secretName := range secrets {
		secret := &corev1.Secret{}
		err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		}, secret)

		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Wrap(err, "get secret")
		}

		if k8serrors.IsNotFound(err) {
			continue
		}

		err = r.client.Delete(context.TODO(), secret, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &secret.UID}})
		if err != nil {
			return errors.Wrapf(err, "delete secret %s", secretName)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deleteCerts(cr *api.PerconaXtraDBCluster) error {
	issuers := []string{
		cr.Name + "-pxc-ca-issuer",
		cr.Name + "-pxc-issuer",
	}
	for _, issuerName := range issuers {
		issuer := &cm.Issuer{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: cr.Namespace, Name: issuerName}, issuer)
		if err != nil {
			continue
		}

		err = r.client.Delete(context.TODO(), issuer, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &issuer.UID}})
		if err != nil {
			return errors.Wrapf(err, "delete issuer %s", issuerName)
		}
	}

	certs := []string{
		cr.Name + "-ssl",
		cr.Name + "-ssl-internal",
		cr.Name + "-ca-cert",
	}
	for _, certName := range certs {
		cert := &cm.Certificate{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: cr.Namespace, Name: certName}, cert)
		if err != nil {
			continue
		}

		err = r.client.Delete(context.TODO(), cert, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &cert.UID}})
		if err != nil {
			return errors.Wrapf(err, "delete certificate %s", certName)
		}
	}

	secrets := []string{
		cr.Name + "-ca-cert",
	}

	if len(cr.Spec.SSLSecretName) > 0 {
		secrets = append(secrets, cr.Spec.SSLSecretName)
	} else {
		secrets = append(secrets, cr.Name+"-ssl")
	}

	if len(cr.Spec.SSLInternalSecretName) > 0 {
		secrets = append(secrets, cr.Spec.SSLInternalSecretName)
	} else {
		secrets = append(secrets, cr.Name+"-ssl-internal")
	}

	for _, secretName := range secrets {
		secret := &corev1.Secret{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: cr.Namespace, Name: secretName}, secret)
		if err != nil {
			continue
		}

		err = r.client.Delete(context.TODO(), secret, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &secret.UID}})
		if err != nil {
			return errors.Wrapf(err, "delete secret %s", secretName)
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

// resyncPXCUsersWithProxySQL calls the method of synchronizing users and makes sure that only one Goroutine works at a time
func (r *ReconcilePerconaXtraDBCluster) resyncPXCUsersWithProxySQL(cr *api.PerconaXtraDBCluster) {
	if cr.Spec.ProxySQL == nil || !cr.Spec.ProxySQL.Enabled {
		return
	}
	if cr.Status.Status != api.AppStateReady || !atomic.CompareAndSwapInt32(&r.syncUsersState, stateFree, stateLocked) {
		return
	}
	go func() {
		err := r.syncPXCUsersWithProxySQL(cr)
		if err != nil && !k8serrors.IsNotFound(err) {
			r.logger(cr.Name, cr.Namespace).Error(err, "sync users")
		}
		atomic.StoreInt32(&r.syncUsersState, stateFree)
	}()
}

func createOrUpdateConfigmap(cl client.Client, configMap *corev1.ConfigMap) error {
	currMap := &corev1.ConfigMap{}
	err := cl.Get(context.TODO(), types.NamespacedName{
		Namespace: configMap.Namespace,
		Name:      configMap.Name,
	}, currMap)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get current configmap")
	}

	if k8serrors.IsNotFound(err) {
		return cl.Create(context.TODO(), configMap)
	}

	if !reflect.DeepEqual(currMap.Data, configMap.Data) {
		return cl.Update(context.TODO(), configMap)
	}

	return nil
}

func deleteConfigMapIfExists(cl client.Client, cr *api.PerconaXtraDBCluster, cmName string) error {
	configMap := &corev1.ConfigMap{}

	err := cl.Get(context.TODO(), types.NamespacedName{
		Namespace: cr.Namespace,
		Name:      cmName,
	}, configMap)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get config map")
	}

	if k8serrors.IsNotFound(err) {
		return nil
	}

	if !metav1.IsControlledBy(configMap, cr) {
		return nil
	}

	return cl.Delete(context.Background(), configMap)
}

func (r *ReconcilePerconaXtraDBCluster) createOrUpdate(cr *api.PerconaXtraDBCluster, obj client.Object) error {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(make(map[string]string))
	}

	objAnnotations := obj.GetAnnotations()
	delete(objAnnotations, "percona.com/last-config-hash")
	obj.SetAnnotations(objAnnotations)

	hash, err := getObjectHash(obj)
	if err != nil {
		return errors.Wrap(err, "calculate object hash")
	}

	objAnnotations = obj.GetAnnotations()
	objAnnotations["percona.com/last-config-hash"] = hash
	obj.SetAnnotations(objAnnotations)

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	oldObject := reflect.New(val.Type()).Interface().(client.Object)

	err = r.client.Get(context.Background(), types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, oldObject)

	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get object")
	}

	if k8serrors.IsNotFound(err) {
		return r.client.Create(context.TODO(), obj)
	}

	if oldObject.GetAnnotations()["percona.com/last-config-hash"] != hash ||
		!isObjectMetaEqual(obj, oldObject) {

		obj.SetResourceVersion(oldObject.GetResourceVersion())
		switch object := obj.(type) {
		case *corev1.Service:
			object.Spec.ClusterIP = oldObject.(*corev1.Service).Spec.ClusterIP
			if object.Spec.Type == corev1.ServiceTypeLoadBalancer {
				object.Spec.HealthCheckNodePort = oldObject.(*corev1.Service).Spec.HealthCheckNodePort
			}
		}

		return r.client.Update(context.TODO(), obj)
	}

	return nil
}

func setIgnoredAnnotationsAndLabels(cr *api.PerconaXtraDBCluster, obj, oldObject client.Object) error {
	oldAnnotations := oldObject.GetAnnotations()
	annotations := obj.GetAnnotations()
	for _, annotation := range cr.Spec.IgnoreAnnotations {
		if v, ok := oldAnnotations[annotation]; ok {
			annotations[annotation] = v
		}
	}
	obj.SetAnnotations(annotations)
	oldLabels := oldObject.GetLabels()
	labels := obj.GetLabels()
	for _, label := range cr.Spec.IgnoreLabels {
		if v, ok := oldLabels[label]; ok {
			labels[label] = v
		}
	}
	obj.SetLabels(labels)
	return nil
}

func mergeMaps(x, y map[string]string) map[string]string {
	if x == nil {
		x = make(map[string]string)
	}
	for k, v := range y {
		if _, ok := x[k]; !ok {
			x[k] = v
		}
	}
	return x
}

func (r *ReconcilePerconaXtraDBCluster) createOrUpdateService(cr *api.PerconaXtraDBCluster, svc *corev1.Service, saveOldMeta bool) error {
	if !saveOldMeta && len(cr.Spec.IgnoreAnnotations) == 0 && len(cr.Spec.IgnoreLabels) == 0 {
		return r.createOrUpdate(cr, svc)
	}
	oldSvc := new(corev1.Service)
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Name:      svc.GetName(),
		Namespace: svc.GetNamespace(),
	}, oldSvc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return r.createOrUpdate(cr, svc)
		}
		return errors.Wrap(err, "get object")
	}

	if saveOldMeta {
		svc.SetAnnotations(mergeMaps(svc.GetAnnotations(), oldSvc.GetAnnotations()))
		svc.SetLabels(mergeMaps(svc.GetLabels(), oldSvc.GetLabels()))
	}
	if err = setIgnoredAnnotationsAndLabels(cr, svc, oldSvc); err != nil {
		return errors.Wrap(err, "set ignored annotations and labels")
	}
	return r.createOrUpdate(cr, svc)
}

func getObjectHash(obj runtime.Object) (string, error) {
	var dataToMarshall interface{}
	switch object := obj.(type) {
	case *appsv1.StatefulSet:
		dataToMarshall = object.Spec
	case *appsv1.Deployment:
		dataToMarshall = object.Spec
	case *corev1.Service:
		dataToMarshall = object.Spec
	default:
		dataToMarshall = obj
	}
	data, err := json.Marshal(dataToMarshall)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func isObjectMetaEqual(old, new metav1.Object) bool {
	return compareMaps(old.GetAnnotations(), new.GetAnnotations()) &&
		compareMaps(old.GetLabels(), new.GetLabels())
}

func compareMaps(x, y map[string]string) bool {
	if len(x) != len(y) {
		return false
	}

	for k, v := range x {
		yVal, ok := y[k]
		if !ok || yVal != v {
			return false
		}
	}

	return true
}

func (r *ReconcilePerconaXtraDBCluster) getConfigVolume(nsName, cvName, cmName string, useDefaultVolume bool) (corev1.Volume, error) {
	n := types.NamespacedName{
		Namespace: nsName,
		Name:      cmName,
	}

	err := r.client.Get(context.TODO(), n, &corev1.Secret{})
	if err == nil {
		return app.GetSecretVolumes(cvName, cmName, false), nil
	}
	if !k8serrors.IsNotFound(err) {
		return corev1.Volume{}, err
	}

	err = r.client.Get(context.TODO(), n, &corev1.ConfigMap{})
	if err == nil {
		return app.GetConfigVolumes(cvName, cmName), nil
	}
	if !k8serrors.IsNotFound(err) {
		return corev1.Volume{}, err
	}

	if useDefaultVolume {
		return app.GetConfigVolumes(cvName, cmName), nil
	}

	return corev1.Volume{}, api.NoCustomVolumeErr
}

func operatorImageName(operatorPod *corev1.Pod) (string, error) {
	for _, c := range operatorPod.Spec.Containers {
		if c.Name == "percona-xtradb-cluster-operator" {
			return c.Image, nil
		}
	}
	return "", errors.New("operator image not found")
}
