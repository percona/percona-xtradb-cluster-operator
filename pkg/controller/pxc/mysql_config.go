package pxc

import (
	"context"
	stderrors "errors"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/config"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const (
	mysqlConfigHashAnnotation        = "percona.com/mysql-config-hash"
	lastAppliedMySQLConfigAnnotation = "percona.com/last-applied-mysql-config"

	liveConfigVersion = "1.21.0"
)

const (
	errIncorrectArguments = 1210
	errReadOnlyVariable   = 1238
	errUnknownVariable    = 1193
)

type globalVariables interface {
	SetGlobalVariable(name, value string) error
	GlobalVariable(name string) (string, error)
	Close() error
}

type globalVariablesConnector func(ctx context.Context, pod corev1.Pod) (globalVariables, error)

func (r *ReconcilePerconaXtraDBCluster) reconcileMySQLConfig(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	return r.syncMySQLConfig(ctx, cr, r.connectGlobalVariables(cr))
}

func (r *ReconcilePerconaXtraDBCluster) syncMySQLConfig(ctx context.Context, cr *api.PerconaXtraDBCluster, connect globalVariablesConnector) error {
	if cr.CompareVersionWith(liveConfigVersion) < 0 || cr.Spec.Pause {
		return nil
	}

	log := logf.FromContext(ctx)

	sts := statefulset.NewNode(cr).StatefulSet()
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		return errors.Wrap(client.IgnoreNotFound(err), "get statefulset")
	}
	if !sts.DeletionTimestamp.IsZero() {
		return nil
	}

	desired, err := r.desiredMySQLConfig(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "get mysql config")
	}

	if err := r.restoreStashedMySQLConfig(ctx, cr, sts); err != nil {
		return errors.Wrap(err, "restore stashed mysql config")
	}

	recorded, ok := sts.Annotations[lastAppliedMySQLConfigAnnotation]
	if !ok {
		// pods read the configuration as they start
		return r.patchMySQLConfigAnnotations(ctx, sts, desired, false)
	}
	applied, err := config.ParseMySQLdJSON(recorded)
	if err != nil {
		return errors.Wrap(err, "parse last applied mysql config")
	}

	changed, removed := desired.Diff(applied)
	if len(changed) == 0 && len(removed) == 0 {
		return nil
	}

	if len(removed) > 0 {
		log.Info("mysqld options removed, restarting PXC pods", "options", removed)
		return r.patchMySQLConfigAnnotations(ctx, sts, desired, true)
	}

	if rolloutInProgress(sts) {
		log.V(1).Info("PXC pods are being replaced, deferring mysqld options", "options", changed)
		return nil
	}

	pods, err := r.readyPXCPods(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "get ready pods")
	}
	if pods == nil {
		log.V(1).Info("not all PXC pods are ready, deferring mysqld options", "options", changed)
		return nil
	}

	restartNeeded, err := setGlobalVariables(ctx, pods, connect, desired, changed)
	if err != nil {
		return errors.Wrap(err, "set global variables")
	}
	if restartNeeded {
		log.Info("mysqld options need a restart, restarting PXC pods", "options", changed)
	} else {
		log.Info("mysqld options applied without restart", "options", changed)
	}

	return r.patchMySQLConfigAnnotations(ctx, sts, desired, restartNeeded)
}

func (r *ReconcilePerconaXtraDBCluster) desiredMySQLConfig(ctx context.Context, cr *api.PerconaXtraDBCluster) (config.MySQLdConfig, error) {
	auto, err := r.autoConfigParams(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "get auto config")
	}
	user, err := r.userMySQLConfig(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "get user configuration")
	}

	parts := user
	if auto != nil {
		parts = append([]string{auto.content}, user...)
	}
	return config.ParseMySQLd(parts...)
}

func (r *ReconcilePerconaXtraDBCluster) patchMySQLConfigAnnotations(
	ctx context.Context,
	sts *appsv1.StatefulSet,
	conf config.MySQLdConfig,
	restart bool,
) error {
	data, err := conf.JSON()
	if err != nil {
		return err
	}

	patch := client.MergeFrom(sts.DeepCopy())
	if sts.Annotations == nil {
		sts.Annotations = make(map[string]string)
	}
	sts.Annotations[lastAppliedMySQLConfigAnnotation] = data

	if restart {
		hash, err := conf.Hash()
		if err != nil {
			return err
		}
		if sts.Spec.Template.Annotations == nil {
			sts.Spec.Template.Annotations = make(map[string]string)
		}
		sts.Spec.Template.Annotations[mysqlConfigHashAnnotation] = hash
	}

	return errors.Wrapf(r.client.Patch(ctx, sts, patch), "patch statefulset/%s", sts.Name)
}

func (r *ReconcilePerconaXtraDBCluster) stashAppliedMySQLConfig(ctx context.Context, cr *api.PerconaXtraDBCluster, sts *appsv1.StatefulSet) error {
	applied, ok := sts.Annotations[lastAppliedMySQLConfigAnnotation]
	if !ok {
		return nil
	}
	return k8s.AnnotateObject(ctx, r.client, cr, map[string]string{lastAppliedMySQLConfigAnnotation: applied})
}

func (r *ReconcilePerconaXtraDBCluster) restoreStashedMySQLConfig(ctx context.Context, cr *api.PerconaXtraDBCluster, sts *appsv1.StatefulSet) error {
	stashed, ok := cr.Annotations[lastAppliedMySQLConfigAnnotation]
	if !ok {
		return nil
	}

	if _, onSts := sts.Annotations[lastAppliedMySQLConfigAnnotation]; !onSts {
		patch := client.MergeFrom(sts.DeepCopy())
		if sts.Annotations == nil {
			sts.Annotations = make(map[string]string)
		}
		sts.Annotations[lastAppliedMySQLConfigAnnotation] = stashed
		if err := r.client.Patch(ctx, sts, patch); err != nil {
			return errors.Wrapf(err, "patch statefulset/%s", sts.Name)
		}
	}

	return k8s.DeannotateObject(ctx, r.client, cr, lastAppliedMySQLConfigAnnotation)
}

func rolloutInProgress(sts *appsv1.StatefulSet) bool {
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	return sts.Status.ObservedGeneration < sts.Generation || sts.Status.UpdatedReplicas < replicas
}

func (r *ReconcilePerconaXtraDBCluster) readyPXCPods(ctx context.Context, cr *api.PerconaXtraDBCluster) ([]corev1.Pod, error) {
	if cr.Status.PXC.Status != api.AppStateReady {
		return nil, nil
	}

	podList := new(corev1.PodList)
	if err := r.client.List(ctx, podList, client.InNamespace(cr.Namespace), client.MatchingLabels(naming.LabelsPXC(cr))); err != nil {
		return nil, errors.Wrap(err, "list pods")
	}

	if len(podList.Items) != int(cr.Spec.PXC.Size) {
		return nil, nil
	}
	for _, pod := range podList.Items {
		if !k8s.IsPodReady(pod) {
			return nil, nil
		}
	}
	return podList.Items, nil
}

func (r *ReconcilePerconaXtraDBCluster) connectGlobalVariables(cr *api.PerconaXtraDBCluster) globalVariablesConnector {
	return func(ctx context.Context, pod corev1.Pod) (globalVariables, error) {
		db, err := queries.New(ctx, r.client, cr.Namespace, internalSecretsPrefix+cr.Name, users.Operator,
			pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, 33062, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
		if err != nil {
			return nil, err
		}
		return &db, nil
	}
}

func setGlobalVariables(
	ctx context.Context,
	pods []corev1.Pod,
	connect globalVariablesConnector,
	conf config.MySQLdConfig,
	options []string,
) (bool, error) {
	restartNeeded := false
	for _, pod := range pods {
		restart, err := setPodGlobalVariables(ctx, pod, connect, conf, options)
		if err != nil {
			return false, errors.Wrapf(err, "pod %s", pod.Name)
		}
		restartNeeded = restartNeeded || restart
	}
	return restartNeeded, nil
}

func setPodGlobalVariables(
	ctx context.Context,
	pod corev1.Pod,
	connect globalVariablesConnector,
	conf config.MySQLdConfig,
	options []string,
) (restartNeeded bool, err error) {
	log := logf.FromContext(ctx)

	db, err := connect(ctx, pod)
	if err != nil {
		return false, errors.Wrap(err, "connect")
	}
	defer func() {
		err = stderrors.Join(err, db.Close())
	}()

	for _, option := range options {
		name := config.VariableName(option)
		value := conf[option]

		err := db.SetGlobalVariable(name, config.FormatValue(value))
		if err == nil {
			continue
		}

		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) {
			return false, errors.Wrapf(err, "set %s", name)
		}

		switch {
		case mysqlErr.Number == errUnknownVariable && config.IsLooseOption(option):
			log.V(1).Info("skipping unknown loose option", "option", option, "pod", pod.Name)
		case mysqlErr.Number == errUnknownVariable:
			// a startup option without a system variable
			restartNeeded = true
		case mysqlErr.Number == errReadOnlyVariable,
			mysqlErr.Number == errIncorrectArguments && name == config.ProviderOptionsVariable:
			current, getErr := db.GlobalVariable(name)
			if getErr != nil || !config.ValueMatches(name, current, value) {
				restartNeeded = true
			}
		default:
			return false, errors.Wrapf(err, "set %s", name)
		}
	}
	return restartNeeded, nil
}
