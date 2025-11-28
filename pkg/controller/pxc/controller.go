package pxc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cm "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

const (
	secretsNameField = ".spec.secretsName"
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

	return &ReconcilePerconaXtraDBCluster{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		crons:         NewCronRegistry(),
		serverVersion: sv,
		clientcmd:     cli,
		lockers:       newLockStore(),
		recorder:      mgr.GetEventRecorderFor(naming.OperatorController),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	if err := setupSecretNameFieldIndexer(mgr); err != nil {
		return errors.Wrap(err, "setup field indexers")
	}
	return builder.ControllerManagedBy(mgr).
		Named(naming.OperatorController).
		Watches(&api.PerconaXtraDBCluster{}, &handler.EnqueueRequestForObject{}).
		Watches(&corev1.Secret{}, enqueuePXCReferencingSecret(mgr.GetClient())).
		Complete(r)
}

func setupSecretNameFieldIndexer(mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.TODO(), &api.PerconaXtraDBCluster{}, secretsNameField, func(o client.Object) []string {
		cluster, ok := o.(*api.PerconaXtraDBCluster)
		if !ok || cluster.Spec.SecretsName == "" {
			return nil
		}
		return []string{cluster.Spec.SecretsName}
	})
}

// enqueuePXCReferencingSecret returns an EventHandler that returns a list of all
// pxc-clusters that reference the secret via `.spec.secretsName`.
func enqueuePXCReferencingSecret(c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		secret, ok := o.(*corev1.Secret)
		if !ok {
			return nil
		}
		list := &api.PerconaXtraDBClusterList{}
		err := c.List(ctx, list, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(secretsNameField, secret.GetName()),
			Namespace:     secret.GetNamespace(),
		})
		log := logf.FromContext(ctx)
		if err != nil {
			log.Error(err, "failed to list clusters referencing secret", "secret", secret.GetName())
		}
		var requests []reconcile.Request
		for _, cr := range list.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cr.GetName(),
					Namespace: cr.GetNamespace(),
				},
			})
		}
		return requests
	})
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
	recorder       record.EventRecorder
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
	ensureVersionJobs *sync.Map
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

const (
	stateFree   = 0
	stateLocked = 1
)

func NewCronRegistry() CronRegistry {
	c := CronRegistry{
		crons:             cron.New(),
		ensureVersionJobs: new(sync.Map),
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
	log := logf.FromContext(ctx)

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

	defer func() {
		uerr := r.updateStatus(ctx, o, false, err)
		if uerr != nil {
			log.Error(uerr, "Update status")
		}
	}()

	if err := r.setCRVersion(ctx, o); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "set CR version")
	}

	err = o.CheckNSetDefaults(r.serverVersion, log)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "wrong PXC options")
	}

	if o.ObjectMeta.DeletionTimestamp != nil {
		finalizers := []string{}
		for _, fnlz := range o.GetFinalizers() {
			var sfs api.StatefulApp
			switch fnlz {
			case naming.FinalizerDeleteSSL:
				err = r.deleteCerts(ctx, o)
			case naming.FinalizerDeleteProxysqlPvc:
				sfs = statefulset.NewProxy(o)
				// deletePVC is always true on this stage
				// because we never reach this point without finalizers
				err = r.deleteStatefulSet(o, sfs, true, false)
			case naming.FinalizerDeletePxcPvc:
				sfs = statefulset.NewNode(o)
				err = r.deleteStatefulSet(o, sfs, true, true)
			// nil error gonna be returned only when there is no more pods to delete (only 0 left)
			// until than finalizer won't be deleted
			case naming.FinalizerDeletePxcPodsInOrder:
				err = r.deletePXCPods(ctx, o)
			}
			if err != nil {
				finalizers = append(finalizers, fnlz)
			}
		}

		err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			cr := new(api.PerconaXtraDBCluster)
			err := r.client.Get(ctx, request.NamespacedName, cr)
			if err != nil {
				return errors.Wrap(err, "get cr")
			}

			cr.SetFinalizers(finalizers)
			return r.client.Update(ctx, cr)
		})
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update cr finalizers")
		}

		// object is being deleted, no need in further actions
		return rr, nil
	}

	// wait until token issued to run PXC in data encrypted mode.
	if o.ShouldWaitForTokenIssue() {
		log.Info("wait for token issuing")
		return rr, nil
	}

	if o.CompareVersionWith("1.7.0") >= 0 && *o.Spec.PXC.AutoRecovery {
		err = r.recoverFullClusterCrashIfNeeded(ctx, o)
		if err != nil {
			log.Info("Failed to check if cluster needs to recover", "err", err.Error())
		}
	}

	userSecret, err := r.reconcileUsersSecret(ctx, o)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "reconcile users secret")
	}

	// TODO: We should not use ReconcileUsersResult. Instead, we should update the statefulset annotations in the reconcileUsers method as soon as possible.
	// Currently, if an error occurs before the statefulsets are updated with annotations, and reconcileUsers has a different result on the next reconcile, the statefulsets will not have the required annotations.
	userReconcileResult := &ReconcileUsersResult{}

	urr, err := r.reconcileUsers(ctx, o, userSecret)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "reconcile users")
	}
	if urr != nil {
		userReconcileResult = urr
	}

	err = r.reconcileCustomUsers(ctx, o)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "reconcile custom users")
	}

	r.resyncPXCUsersWithProxySQL(ctx, o)
	if o.Status.PXC.Version == "" || strings.HasSuffix(o.Status.PXC.Version, "intermediate") {
		err := r.ensurePXCVersion(ctx, o, VersionServiceClient{OpVersion: o.Version().String()})
		if err != nil {
			log.Info("failed to ensure version, running with default", "error", err)
		}
	}
	err = r.reconcilePersistentVolumes(ctx, o)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "reconcile persistent volumes")
	}

	err = r.reconcileSSL(ctx, o)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to reconcile SSL. Please create your TLS secret %s and %s manually or setup cert-manager correctly", o.Spec.PXC.SSLSecretName, o.Spec.PXC.SSLInternalSecretName)
	}

	err = r.deploy(ctx, o)
	if err != nil {
		return reconcile.Result{}, err
	}

	pxcSet := statefulset.NewNode(o)
	err = r.updatePod(ctx, pxcSet, o.Spec.PXC.PodSpec, o, userReconcileResult.pxcAnnotations, true)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "pxc upgrade error")
	}

	saveOldSvcMeta := true
	if o.CompareVersionWith("1.14.0") >= 0 {
		saveOldSvcMeta = len(o.Spec.PXC.Expose.Labels) == 0 && len(o.Spec.PXC.Expose.Annotations) == 0
	}
	err = r.createOrUpdateService(ctx, o, pxc.NewServicePXC(o), saveOldSvcMeta)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "PXC service upgrade error")
	}
	err = r.createOrUpdateService(ctx, o, pxc.NewServicePXCUnready(o), true)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "PXC service upgrade error")
	}

	if o.Spec.PXC.Expose.Enabled {
		err = r.ensurePxcPodServices(ctx, o)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "create replication services")
		}
	} else {
		err = r.removePxcPodServices(o)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "remove pxc pod services")
		}
	}

	if err := r.reconcileHAProxy(ctx, o, userReconcileResult.haproxyAnnotations); err != nil {
		return reconcile.Result{}, err
	}

	proxysqlSet := statefulset.NewProxy(o)
	if o.Spec.ProxySQLEnabled() {
		err = r.updatePod(ctx, proxysqlSet, &o.Spec.ProxySQL.PodSpec, o, userReconcileResult.proxysqlAnnotations, true)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "ProxySQL upgrade error")
		}
		svc := pxc.NewServiceProxySQL(o)

		if o.CompareVersionWith("1.14.0") >= 0 {
			err = r.createOrUpdateService(ctx, o, svc, len(o.Spec.ProxySQL.Expose.Labels) == 0 && len(o.Spec.ProxySQL.Expose.Annotations) == 0)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "%s upgrade error", svc.Name)
			}
		} else {
			err = r.createOrUpdateService(ctx, o, svc, len(o.Spec.ProxySQL.ServiceLabels) == 0 && len(o.Spec.ProxySQL.ServiceAnnotations) == 0)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "%s upgrade error", svc.Name)
			}
		}

		svc = pxc.NewServiceProxySQLUnready(o)
		err = r.createOrUpdateService(ctx, o, svc, true)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "%s upgrade error", svc.Name)
		}
	} else {
		// check if there is need to delete pvc
		deletePVC := false
		for _, fnlz := range o.GetFinalizers() {
			switch fnlz {
			case naming.FinalizerDeleteProxysqlPvc:
				deletePVC = true
				break
			}
		}

		err = r.deleteStatefulSet(o, proxysqlSet, deletePVC, false)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = r.deleteServices(pxc.NewServiceProxySQL(o), pxc.NewServiceProxySQLUnready(o))
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if o.CompareVersionWith("1.9.0") >= 0 {
		err = r.reconcileReplication(ctx, o, userReconcileResult.updateReplicationPassword)
		if err != nil {
			log.Info("reconcile replication error", "err", err.Error())
		}
	}

	err = r.reconcileBackups(ctx, o)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = backup.CheckPITRErrors(ctx, r.client, r.clientcmd, o)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = backup.UpdatePITRTimeline(ctx, r.client, r.clientcmd, o)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.fetchVersionFromPXC(ctx, o, pxcSet); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "update CR version")
	}

	err = r.scheduleEnsurePXCVersion(ctx, o, VersionServiceClient{OpVersion: o.Version().String()})
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to ensure version")
	}

	err = r.scheduleTelemetryRequests(ctx, o, VersionServiceClient{OpVersion: o.Version().String()})
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to schedule telemetry requests")
	}

	return rr, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileHAProxy(ctx context.Context, cr *api.PerconaXtraDBCluster, templateAnnotations map[string]string) error {
	if !cr.HAProxyEnabled() {
		if err := r.deleteServices(pxc.NewServiceHAProxyReplicas(cr)); err != nil {
			return errors.Wrap(err, "delete HAProxy replica service")
		}

		if err := r.deleteServices(pxc.NewServiceHAProxy(cr)); err != nil {
			return errors.Wrap(err, "delete HAProxy service")
		}

		if err := r.deleteStatefulSet(cr, statefulset.NewHAProxy(cr), false, false); err != nil {
			return errors.Wrap(err, "delete HAProxy stateful set")
		}

		return nil
	}

	envVarsSecret := new(corev1.Secret)
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      cr.Spec.HAProxy.EnvVarsSecretName,
		Namespace: cr.Namespace,
	}, envVarsSecret); client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "get haproxy env vars secret")
	}
	sts := statefulset.NewHAProxy(cr)

	if err := r.updatePod(ctx, sts, &cr.Spec.HAProxy.PodSpec, cr, templateAnnotations, true); err != nil {
		return errors.Wrap(err, "HAProxy upgrade error")
	}
	svc := pxc.NewServiceHAProxy(cr)
	podSpec := cr.Spec.HAProxy.PodSpec
	expose := cr.Spec.HAProxy.ExposePrimary

	if cr.CompareVersionWith("1.14.0") >= 0 {
		err := r.createOrUpdateService(ctx, cr, svc, len(expose.Labels) == 0 && len(expose.Annotations) == 0)
		if err != nil {
			return errors.Wrapf(err, "%s upgrade error", svc.Name)
		}
	} else {
		err := r.createOrUpdateService(ctx, cr, svc, len(podSpec.ServiceLabels) == 0 && len(podSpec.ServiceAnnotations) == 0)
		if err != nil {
			return errors.Wrapf(err, "%s upgrade error", svc.Name)
		}
	}

	if cr.HAProxyReplicasServiceEnabled() {
		svc := pxc.NewServiceHAProxyReplicas(cr)

		if cr.CompareVersionWith("1.14.0") >= 0 {
			e := cr.Spec.HAProxy.ExposeReplicas
			err := r.createOrUpdateService(ctx, cr, svc, len(e.ServiceExpose.Labels) == 0 && len(e.ServiceExpose.Annotations) == 0)
			if err != nil {
				return errors.Wrapf(err, "%s upgrade error", svc.Name)
			}
		} else {
			err := r.createOrUpdateService(ctx, cr, svc, len(podSpec.ReplicasServiceLabels) == 0 && len(podSpec.ReplicasServiceAnnotations) == 0)
			if err != nil {
				return errors.Wrapf(err, "%s upgrade error", svc.Name)
			}
		}

	} else {
		if err := r.deleteServices(pxc.NewServiceHAProxyReplicas(cr)); err != nil {
			return errors.Wrap(err, "delete HAProxy replica service")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) deploy(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	deployStatefulApp := func(stsApp api.StatefulApp, podSpec *api.PodSpec) error {
		if err := r.updatePod(ctx, stsApp, podSpec, cr, nil, false); err != nil {
			return errors.Wrapf(err, "updatePod for %s", stsApp.Name())
		}
		if err := r.reconcilePDB(ctx, cr, podSpec.PodDisruptionBudget, stsApp); err != nil {
			return errors.Wrapf(err, "failed to reconcile PodDisruptionBudget for %s", stsApp.Name())
		}
		return nil
	}

	if err := deployStatefulApp(statefulset.NewNode(cr), cr.Spec.PXC.PodSpec); err != nil {
		return errors.Wrap(err, "failed to deploy pxc")
	}
	if cr.HAProxyEnabled() {
		if err := deployStatefulApp(statefulset.NewHAProxy(cr), &cr.Spec.HAProxy.PodSpec); err != nil {
			return errors.Wrap(err, "failed to deploy haproxy")
		}
	}
	if cr.ProxySQLEnabled() {
		if err := deployStatefulApp(statefulset.NewProxy(cr), &cr.Spec.ProxySQL.PodSpec); err != nil {
			return errors.Wrap(err, "failed to deploy proxysql")
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcilePDB(ctx context.Context, cr *api.PerconaXtraDBCluster, spec *api.PodDisruptionBudgetSpec, sfs api.StatefulApp) error {
	if spec == nil {
		return nil
	}

	sts := new(appsv1.StatefulSet)
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(sfs.StatefulSet()), sts); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "get PXC stateful set")
	}

	pdb := pxc.PodDisruptionBudget(cr, spec, sfs.Labels())
	if err := k8s.SetControllerReference(sts, pdb, r.scheme); err != nil {
		return errors.Wrap(err, "set owner reference")
	}

	return errors.Wrap(r.createOrUpdate(ctx, pdb), "reconcile pdb")
}

func (r *ReconcilePerconaXtraDBCluster) deletePXCPods(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	sfs := statefulset.NewNode(cr)
	err := r.deleteStatefulSetPods(cr.Namespace, sfs)
	if err != nil {
		return errors.Wrap(err, "delete statefulset pods")
	}
	if cr.Spec.Backup != nil && cr.Spec.Backup.PITR.Enabled {
		return errors.Wrap(r.deletePITR(ctx, cr), "delete pitr pod")
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

func (r *ReconcilePerconaXtraDBCluster) deleteStatefulSet(cr *api.PerconaXtraDBCluster, sfs api.StatefulApp, deletePVC, deleteSecrets bool) error {
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
	}

	if deleteSecrets {
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
	secrets := []string{
		cr.Spec.SecretsName,
		"internal-" + cr.Name,
		cr.Name + "-mysql-init",
	}

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

func (r *ReconcilePerconaXtraDBCluster) deleteCerts(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	issuers := []string{
		cr.Name + "-pxc-ca-issuer",
		cr.Name + "-pxc-issuer",
	}
	for _, issuerName := range issuers {
		issuer := &cm.Issuer{}
		err := r.client.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: issuerName}, issuer)
		if err != nil {
			continue
		}

		err = r.client.Delete(ctx, issuer, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &issuer.UID}})
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
		err := r.client.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: certName}, cert)
		if err != nil {
			continue
		}

		err = r.client.Delete(ctx, cert, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &cert.UID}})
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
		err := r.client.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: secretName}, secret)
		if err != nil {
			continue
		}

		err = r.client.Delete(ctx, secret, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &secret.UID}})
		if err != nil {
			return errors.Wrapf(err, "delete secret %s", secretName)
		}
	}

	return nil
}

// resyncPXCUsersWithProxySQL calls the method of synchronizing users and makes sure that only one Goroutine works at a time
func (r *ReconcilePerconaXtraDBCluster) resyncPXCUsersWithProxySQL(ctx context.Context, cr *api.PerconaXtraDBCluster) {
	if !cr.Spec.ProxySQLEnabled() {
		return
	}
	if cr.Status.Status != api.AppStateReady || !atomic.CompareAndSwapInt32(&r.syncUsersState, stateFree, stateLocked) {
		return
	}
	go func() {
		err := r.syncPXCUsersWithProxySQL(ctx, cr)
		if err != nil && !k8serrors.IsNotFound(err) {
			logf.FromContext(ctx).Error(err, "sync users")
		}
		atomic.StoreInt32(&r.syncUsersState, stateFree)
	}()
}

func (r *ReconcilePerconaXtraDBCluster) createOrUpdate(ctx context.Context, obj client.Object) error {
	log := logf.FromContext(ctx)

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

	err = r.client.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, oldObject)

	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get object")
	}

	if k8serrors.IsNotFound(err) {
		log.V(1).Info("Creating object", "object", obj.GetName(), "kind", obj.GetObjectKind())
		return r.client.Create(ctx, obj)
	}

	switch obj.(type) {
	case *appsv1.Deployment:
		objAnnotations := oldObject.GetAnnotations()
		delete(objAnnotations, "deployment.kubernetes.io/revision")
		oldObject.SetAnnotations(objAnnotations)
	}

	if oldObject.GetAnnotations()["percona.com/last-config-hash"] != hash || !isObjectMetaEqual(obj, oldObject) {
		switch object := obj.(type) {
		case *corev1.Service:
			object.Spec.ClusterIP = oldObject.(*corev1.Service).Spec.ClusterIP
			if object.Spec.Type == corev1.ServiceTypeLoadBalancer {
				object.Spec.HealthCheckNodePort = oldObject.(*corev1.Service).Spec.HealthCheckNodePort
			}
		case *policyv1.PodDisruptionBudget:
			obj.SetResourceVersion(oldObject.GetResourceVersion())
		}

		log.V(1).Info("Updating object",
			"object", obj.GetName(),
			"kind", obj.GetObjectKind(),
			"hashChanged", oldObject.GetAnnotations()["percona.com/last-config-hash"] != hash,
			"metaChanged", !isObjectMetaEqual(obj, oldObject),
		)
		if util.IsLogLevelVerbose() && !util.IsLogStructured() {
			fmt.Println(cmp.Diff(oldObject, obj))
		}

		return r.client.Update(ctx, obj)
	}

	return nil
}

func setIgnoredAnnotationsAndLabels(cr *api.PerconaXtraDBCluster, obj, oldObject client.Object) {
	oldAnnotations := oldObject.GetAnnotations()
	if oldAnnotations == nil {
		oldAnnotations = make(map[string]string)
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	for _, annotation := range cr.Spec.IgnoreAnnotations {
		if v, ok := oldAnnotations[annotation]; ok {
			annotations[annotation] = v
		}
	}
	obj.SetAnnotations(annotations)

	oldLabels := oldObject.GetLabels()
	if oldLabels == nil {
		oldLabels = make(map[string]string)
	}
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	for _, label := range cr.Spec.IgnoreLabels {
		if v, ok := oldLabels[label]; ok {
			labels[label] = v
		}
	}
	obj.SetLabels(labels)
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

func (r *ReconcilePerconaXtraDBCluster) createOrUpdateService(ctx context.Context, cr *api.PerconaXtraDBCluster, svc *corev1.Service, saveOldMeta bool) error {
	err := k8s.SetControllerReference(cr, svc, r.scheme)
	if err != nil {
		return errors.Wrap(err, "set controller reference")
	}
	if !saveOldMeta && len(cr.Spec.IgnoreAnnotations) == 0 && len(cr.Spec.IgnoreLabels) == 0 {
		return r.createOrUpdate(ctx, svc)
	}
	oldSvc := new(corev1.Service)
	err = r.client.Get(ctx, types.NamespacedName{
		Name:      svc.GetName(),
		Namespace: svc.GetNamespace(),
	}, oldSvc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return r.createOrUpdate(ctx, svc)
		}
		return errors.Wrap(err, "get object")
	}

	if saveOldMeta {
		svc.SetAnnotations(mergeMaps(svc.GetAnnotations(), oldSvc.GetAnnotations()))
		svc.SetLabels(mergeMaps(svc.GetLabels(), oldSvc.GetLabels()))
	}
	setIgnoredAnnotationsAndLabels(cr, svc, oldSvc)

	return r.createOrUpdate(ctx, svc)
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
