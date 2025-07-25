package pxc

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/config"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileConfigMaps(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	result := controllerutil.OperationResultNone

	res, err := r.reconcileAutotuneConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile autotune config")
	}
	result = res

	res, err = r.reconcileCustomConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile custom config")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	res, err = r.reconcileProxySQLConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile proxysql config map")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	res, err = r.reconcileHAProxyConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile haproxy config map")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	res, err = r.reconcileLogcollectorConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile logcollector config map")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	_, err = r.reconcileHookScriptConfigMaps(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile hookscript config maps")
	}

	return result, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileAutotuneConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	autotuneCm := config.AutoTuneConfigMapName(cr.Name, "pxc")

	_, ok := cr.Spec.PXC.Resources.Limits[corev1.ResourceMemory]
	if !ok {
		err := deleteConfigMapIfExists(ctx, r.client, cr, autotuneCm)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete configmap")
	}

	configMap, err := config.NewAutoTuneConfigMap(cr, cr.Spec.PXC.Resources.Limits.Memory(), autotuneCm)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "new configmap")
	}

	err = k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update configmap")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileCustomConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	pxcConfigName := config.CustomConfigMapName(cr.Name, "pxc")

	if cr.Spec.PXC.Configuration == "" {
		err := deleteConfigMapIfExists(ctx, r.client, cr, pxcConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	configMap := config.NewConfigMap(cr, pxcConfigName, "init.cnf", cr.Spec.PXC.Configuration)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileHookScriptConfigMaps(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	pxcHookScriptName := config.HookScriptConfigMapName(cr.Name, "pxc")
	if cr.Spec.PXC != nil && cr.Spec.PXC.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.PXC.HookScript, pxcHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create pxc hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, pxcHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete pxc hookscript config map")
		}
	}

	proxysqlHookScriptName := config.HookScriptConfigMapName(cr.Name, "proxysql")
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.ProxySQL.HookScript, proxysqlHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create proxysql hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, proxysqlHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete proxysql hookscript config map")
		}
	}

	haproxyHookScriptName := config.HookScriptConfigMapName(cr.Name, "haproxy")
	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.HAProxy.HookScript, haproxyHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create haproxy hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, haproxyHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete haproxy config map")
		}
	}

	logCollectorHookScriptName := config.HookScriptConfigMapName(cr.Name, "logcollector")
	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.LogCollector.HookScript, logCollectorHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create logcollector hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, logCollectorHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete logcollector config map")
		}
	}

	return controllerutil.OperationResultNone, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileProxySQLConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	proxysqlConfigName := config.CustomConfigMapName(cr.Name, "proxysql")

	if !cr.Spec.ProxySQLEnabled() || cr.Spec.ProxySQL.Configuration == "" {
		err := deleteConfigMapIfExists(ctx, r.client, cr, proxysqlConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	configMap := config.NewConfigMap(cr, proxysqlConfigName, "proxysql.cnf", cr.Spec.ProxySQL.Configuration)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileHAProxyConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	haproxyConfigName := config.CustomConfigMapName(cr.Name, "haproxy")

	if !cr.HAProxyEnabled() || cr.Spec.HAProxy.Configuration == "" {
		err := deleteConfigMapIfExists(ctx, r.client, cr, haproxyConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	configMap := config.NewConfigMap(cr, haproxyConfigName, "haproxy-global.cfg", cr.Spec.HAProxy.Configuration)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileLogcollectorConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	logCollectorConfigName := config.CustomConfigMapName(cr.Name, "logcollector")

	if cr.Spec.LogCollector == nil || cr.Spec.LogCollector.Configuration == "" {
		err := deleteConfigMapIfExists(ctx, r.client, cr, logCollectorConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	configMap := config.NewConfigMap(cr, logCollectorConfigName, "fluentbit_custom.conf", cr.Spec.LogCollector.Configuration)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) createHookScriptConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster, hookScript string, configMapName string) error {
	configMap := config.NewConfigMap(cr, configMapName, "hook.sh", hookScript)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return errors.Wrap(err, "set controller ref")
	}

	_, err = createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return errors.Wrap(err, "create or update configmap")
	}

	return nil
}

func createOrUpdateConfigmap(ctx context.Context, cl client.Client, configMap *corev1.ConfigMap) (controllerutil.OperationResult, error) {
	log := logf.FromContext(ctx)

	nn := types.NamespacedName{Namespace: configMap.Namespace, Name: configMap.Name}

	currMap := &corev1.ConfigMap{}
	err := cl.Get(ctx, nn, currMap)
	if err != nil && !k8serrors.IsNotFound(err) {
		return controllerutil.OperationResultNone, errors.Wrap(err, "get current configmap")
	}

	if k8serrors.IsNotFound(err) {
		log.V(1).Info("Creating object", "object", configMap.Name, "kind", configMap.GetObjectKind())
		return controllerutil.OperationResultCreated, cl.Create(ctx, configMap)
	}

	if !reflect.DeepEqual(currMap.Data, configMap.Data) {
		log.V(1).Info("Updating object", "object", configMap.Name, "kind", configMap.GetObjectKind())
		err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			cm := &corev1.ConfigMap{}

			err := cl.Get(ctx, nn, cm)
			if err != nil {
				return err
			}

			cm.Data = configMap.Data

			return cl.Update(ctx, cm)
		})
		return controllerutil.OperationResultUpdated, err
	}

	return controllerutil.OperationResultNone, nil
}

func deleteConfigMapIfExists(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster, cmName string) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{}

	err := cl.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cmName}, configMap)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get config map")
	}

	if k8serrors.IsNotFound(err) {
		return nil
	}

	if !metav1.IsControlledBy(configMap, cr) {
		return nil
	}

	log.V(1).Info("Deleting object", "object", configMap.Name, "kind", configMap.GetObjectKind())
	return cl.Delete(ctx, configMap)
}
