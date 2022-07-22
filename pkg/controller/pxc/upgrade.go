package pxc

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
)

func (r *ReconcilePerconaXtraDBCluster) updatePod(ctx context.Context, stsApp api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, initContainers []corev1.Container) error {
	secretsName := cr.Spec.SecretsName
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secretsName = internalPrefix + cr.Name
	}
	secret := new(corev1.Secret)
	err := r.client.Get(ctx, types.NamespacedName{
		Name: secretsName, Namespace: cr.Namespace,
	}, secret)
	if client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "get internal secret")
	}
	sts, err := pxc.StatefulSet(stsApp, podSpec, cr, secret, initContainers, r.logger(cr.Name, cr.Namespace), r.getConfigVolume)
	if err != nil {
		return errors.Wrap(err, "create HAProxy StatefulSet")
	}

	currentSet := new(appsv1.StatefulSet)
	err = r.client.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, currentSet)
	if client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "failed to get sate")
	}
	if v, ok := currentSet.Spec.Template.Annotations["last-applied-secret"]; ok {
		sts.Spec.Template.Annotations["last-applied-secret"] = v
	}

	err = r.reconcileConfigMap(cr)
	if err != nil {
		return errors.Wrap(err, "upgradePod/updateApp error: update db config error")
	}

	// embed DB configuration hash
	configHash, err := r.getConfigHash(cr, sts)
	if err != nil {
		return errors.Wrap(err, "getting config hash")
	}

	if cr.CompareVersionWith("1.1.0") >= 0 {
		sts.Spec.Template.Annotations["percona.com/configuration-hash"] = configHash
	}

	// change TLS secret configuration
	sslHash, err := r.getSecretHash(cr, cr.Spec.PXC.SSLSecretName, cr.Spec.AllowUnsafeConfig)
	if err != nil {
		return errors.Wrap(err, "upgradePod/updateApp error: update secret error")
	}
	if sslHash != "" && cr.CompareVersionWith("1.1.0") >= 0 {
		sts.Spec.Template.Annotations["percona.com/ssl-hash"] = sslHash
	}

	sslInternalHash, err := r.getSecretHash(cr, cr.Spec.PXC.SSLInternalSecretName, cr.Spec.AllowUnsafeConfig)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "upgradePod/updateApp error: update secret error")
	}
	if sslInternalHash != "" && cr.CompareVersionWith("1.1.0") >= 0 {
		sts.Spec.Template.Annotations["percona.com/ssl-internal-hash"] = sslInternalHash
	}

	vaultConfigHash, err := r.getSecretHash(cr, cr.Spec.VaultSecretName, true)
	if err != nil {
		return errors.Wrap(err, "upgradePod/updateApp error: update secret error")
	}
	if vaultConfigHash != "" && cr.CompareVersionWith("1.6.0") >= 0 && !isHAproxy(stsApp) {
		sts.Spec.Template.Annotations["percona.com/vault-config-hash"] = vaultConfigHash
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		envVarsHash, err := r.getSecretHash(cr, cr.Spec.PXC.EnvVarsSecretName, true)
		if isHAproxy(stsApp) {
			envVarsHash, err = r.getSecretHash(cr, cr.Spec.HAProxy.EnvVarsSecretName, true)
		} else if isProxySQL(stsApp) {
			envVarsHash, err = r.getSecretHash(cr, cr.Spec.ProxySQL.EnvVarsSecretName, true)
		}
		if err != nil {
			return errors.Wrap(err, "upgradePod/updateApp error: update secret error")
		}
		if envVarsHash != "" {
			sts.Spec.Template.Annotations["percona.com/env-secret-config-hash"] = envVarsHash
		}
	}

	if isHAproxy(stsApp) && cr.CompareVersionWith("1.6.0") >= 0 {
		delete(sts.Spec.Template.Annotations, "percona.com/ssl-internal-hash")
		delete(sts.Spec.Template.Annotations, "percona.com/ssl-hash")
	}

	err = setControllerReference(cr, sts, r.scheme)
	if err != nil {
		return err
	}
	err = r.createOrUpdate(sts)
	if err != nil {
		return errors.Wrap(err, "update error")
	}

	time.Sleep(time.Second)

	err = r.client.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, currentSet)
	if err != nil {
		return errors.Wrap(err, "failed to get statefulset")
	}

	// PodDisruptionBudget object for sts
	pdb := cr.Spec.PXC.PodDisruptionBudget
	if isHAproxy(stsApp) {
		pdb = cr.Spec.HAProxy.PodDisruptionBudget
	} else if isProxySQL(stsApp) {
		pdb = cr.Spec.ProxySQL.PodDisruptionBudget
	}
	err = r.reconcilePDB(pdb, stsApp, cr.Namespace, currentSet)
	if err != nil {
		return errors.Wrapf(err, "PodDisruptionBudget for %s", sts.Name)
	}

	if cr.Spec.UpdateStrategy != api.SmartUpdateStatefulSetStrategyType {
		return nil
	}

	return r.smartUpdate(ctx, stsApp, cr)
}

func (r *ReconcilePerconaXtraDBCluster) smartUpdate(ctx context.Context, sfs api.StatefulApp, cr *api.PerconaXtraDBCluster) error {
	if !isPXC(sfs) {
		return nil
	}

	if cr.Spec.Pause {
		return nil
	}

	// sleep to get new sfs revision
	time.Sleep(time.Second)

	currentSet := sfs.StatefulSet()
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      currentSet.Name,
		Namespace: currentSet.Namespace,
	}, currentSet)
	if err != nil {
		return errors.Wrap(err, "failed to get current sfs")
	}

	list := corev1.PodList{}
	if err := r.client.List(ctx,
		&list,
		&client.ListOptions{
			Namespace:     currentSet.Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
	); err != nil {
		return errors.Wrap(err, "get pod list")
	}
	statefulSetChanged := false
	for _, pod := range list.Items {
		if pod.ObjectMeta.Labels["controller-revision-hash"] != currentSet.Status.UpdateRevision {
			statefulSetChanged = true
			break
		}
	}
	if !statefulSetChanged {
		return nil
	}

	logger := r.logger(cr.Name, cr.Namespace)

	logger.Info("statefulSet was changed, run smart update")

	running, err := r.isBackupRunning(cr)
	if err != nil {
		logger.Error(err, "can't start 'SmartUpdate'")
		return nil
	}
	if running {
		logger.Info("can't start/continue 'SmartUpdate': backup is running")
		return nil
	}

	if currentSet.Status.ReadyReplicas < currentSet.Status.Replicas {
		logger.Info("can't start/continue 'SmartUpdate': waiting for all replicas are ready")
		return nil
	}

	primary, err := r.getPrimaryPod(cr)
	if err != nil {
		return errors.Wrap(err, "get primary pod")
	}
	for _, pod := range list.Items {
		if pod.Status.PodIP == primary || pod.Name == primary {
			primary = fmt.Sprintf("%s.%s.%s", pod.Name, currentSet.Name, currentSet.Namespace)
			break
		}
	}

	logger.Info("primary pod", "pod name", primary)

	waitLimit := 2 * 60 * 60 // 2 hours
	if cr.Spec.PXC.LivenessInitialDelaySeconds != nil {
		waitLimit = int(*cr.Spec.PXC.LivenessInitialDelaySeconds)
	}

	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name > list.Items[j].Name
	})

	var primaryPod corev1.Pod
	for _, pod := range list.Items {
		pod := pod
		if strings.HasPrefix(primary, fmt.Sprintf("%s.%s.%s", pod.Name, currentSet.Name, currentSet.Namespace)) {
			primaryPod = pod
		} else {
			logger.Info("apply changes to secondary pod", "pod name", pod.Name)
			if err := r.applyNWait(cr, currentSet, &pod, waitLimit); err != nil {
				return errors.Wrap(err, "failed to apply changes")
			}
		}
	}

	logger.Info("apply changes to primary pod", "pod name", primaryPod.Name)
	if err := r.applyNWait(cr, currentSet, &primaryPod, waitLimit); err != nil {
		return errors.Wrap(err, "failed to apply changes")
	}

	logger.Info("smart update finished")

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) applyNWait(cr *api.PerconaXtraDBCluster, sfs *appsv1.StatefulSet, pod *corev1.Pod, waitLimit int) error {
	logger := r.logger(cr.Name, cr.Namespace)

	if pod.ObjectMeta.Labels["controller-revision-hash"] == sfs.Status.UpdateRevision {
		logger.Info("pod is already updated", "pod name", pod.Name)
	} else {
		if err := r.client.Delete(context.TODO(), pod); err != nil {
			return errors.Wrap(err, "failed to delete pod")
		}
	}

	orderInSts, err := getPodOrderInSts(sfs.Name, pod.Name)
	if err != nil {
		return errors.Errorf("compute pod order err, sfs name: %s, pod name: %s", sfs.Name, pod.Name)
	}
	if int32(orderInSts) >= *sfs.Spec.Replicas {
		logger.Info(fmt.Sprintf("sfs %s is scaled down, pod %s will not be started ", sfs.Name, pod.Name))
		return nil
	}

	if err := r.waitPodRestart(cr, sfs.Status.UpdateRevision, pod, waitLimit, logger); err != nil {
		return errors.Wrap(err, "failed to wait pod")
	}

	if err := r.waitPXCSynced(cr, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, waitLimit); err != nil {
		return errors.Wrap(err, "failed to wait pxc sync")
	}

	if err := r.waitHostgroups(cr, sfs.Name, pod, waitLimit, logger); err != nil {
		return errors.Wrap(err, "failed to wait hostgroups status")
	}

	if err := r.waitUntilOnline(cr, sfs.Name, pod, waitLimit, logger); err != nil {
		return errors.Wrap(err, "failed to wait pxc status")
	}

	return nil
}

func getPodOrderInSts(stsName string, podName string) (int, error) {
	return strconv.Atoi(podName[len(stsName)+1:])
}

func (r *ReconcilePerconaXtraDBCluster) waitHostgroups(cr *api.PerconaXtraDBCluster, sfsName string, pod *corev1.Pod, waitLimit int, logger logr.Logger) error {
	if cr.Spec.ProxySQL == nil || !cr.Spec.ProxySQL.Enabled {
		return nil
	}

	database, err := r.proxyDB(cr)
	if err != nil {
		return errors.Wrap(err, "failed to get proxySQL db")
	}

	defer database.Close()

	podNamePrefix := fmt.Sprintf("%s.%s.%s", pod.Name, sfsName, cr.Namespace)

	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			present, err := database.PresentInHostgroups(podNamePrefix)
			if err != nil && err != queries.ErrNotFound {
				return false, errors.Wrap(err, "failed to get hostgroup status")
			}
			if !present {
				return false, nil
			}

			logger.Info("pod present in hostgroups", "pod name", pod.Name)
			return true, nil
		})
}

func (r *ReconcilePerconaXtraDBCluster) waitUntilOnline(cr *api.PerconaXtraDBCluster, sfsName string, pod *corev1.Pod, waitLimit int, logger logr.Logger) error {
	if cr.Spec.ProxySQL == nil || !cr.Spec.ProxySQL.Enabled {
		return nil
	}

	database, err := r.proxyDB(cr)
	if err != nil {
		return errors.Wrap(err, "failed to get proxySQL db")
	}

	defer database.Close()

	podNamePrefix := fmt.Sprintf("%s.%s.%s", pod.Name, sfsName, cr.Namespace)

	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			statuses, err := database.ProxySQLInstanceStatus(podNamePrefix)
			if err != nil && err != queries.ErrNotFound {
				return false, errors.Wrap(err, "failed to get status")
			}

			for _, status := range statuses {
				if status != "ONLINE" {
					return false, nil
				}
			}

			logger.Info("pod is online", "pod name", pod.Name)
			return true, nil
		})
}

// retry runs func "f" every "in" time until "limit" is reached
// it also doesn't have an extra tail wait after the limit is reached
// and f func runs first time instantly
func retry(in, limit time.Duration, f func() (bool, error)) error {
	fdone, err := f()
	if err != nil {
		return err
	}
	if fdone {
		return nil
	}

	done := time.NewTimer(limit)
	defer done.Stop()
	tk := time.NewTicker(in)
	defer tk.Stop()

	for {
		select {
		case <-done.C:
			return errors.New("reach pod wait limit")
		case <-tk.C:
			fdone, err := f()
			if err != nil {
				return err
			}
			if fdone {
				return nil
			}
		}
	}
}

func (r *ReconcilePerconaXtraDBCluster) proxyDB(cr *api.PerconaXtraDBCluster) (queries.Database, error) {
	var database queries.Database
	var user, host string
	var port, proxySize int32

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		user = "proxyadmin"
		host = fmt.Sprintf("%s-proxysql-unready.%s", cr.ObjectMeta.Name, cr.Namespace)
		proxySize = cr.Spec.ProxySQL.Size
		port = 6032
	} else if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		user = "monitor"
		host = fmt.Sprintf("%s-haproxy.%s", cr.Name, cr.Namespace)
		proxySize = cr.Spec.HAProxy.Size

		hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
		if err != nil {
			return database, errors.Wrap(err, "check if config has proxy_protocol_networks key")
		}

		if hasKey && cr.CompareVersionWith("1.6.0") >= 0 {
			port = 33062
		} else {
			port = 3306
		}
	} else {
		return database, errors.New("can't detect enabled proxy, please enable HAProxy or ProxySQL")
	}
	secrets := cr.Spec.SecretsName
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secrets = "internal-" + cr.Name
	}
	for i := 0; ; i++ {
		db, err := queries.New(r.client, cr.Namespace, secrets, user, host, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
		if err != nil && i < int(proxySize) {
			time.Sleep(time.Second)
		} else if err != nil && i == int(proxySize) {
			return database, err
		} else {
			database = db
			break
		}
	}

	return database, nil
}

func (r *ReconcilePerconaXtraDBCluster) getPrimaryPod(cr *api.PerconaXtraDBCluster) (string, error) {
	database, err := r.proxyDB(cr)
	if err != nil {
		return "", errors.Wrap(err, "failed to get proxySQL db")
	}

	defer database.Close()

	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		host, err := database.Hostname()
		if err != nil {
			return "", err
		}

		return host, nil
	}

	return database.PrimaryHost()
}

func (r *ReconcilePerconaXtraDBCluster) waitPXCSynced(cr *api.PerconaXtraDBCluster, host string, waitLimit int) error {
	user := "root"
	secrets := cr.Spec.SecretsName
	port := int32(3306)
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secrets = "internal-" + cr.Name
		port = int32(33062)
	}

	database, err := queries.New(r.client, cr.Namespace, secrets, user, host, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return errors.Wrap(err, "failed to access PXC database")
	}

	defer database.Close()

	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			state, err := database.WsrepLocalStateComment()
			if err != nil {
				return false, errors.Wrap(err, "failed to get wsrep local state")
			}

			if state == "Synced" {
				return true, nil
			}

			return false, nil
		})
}

func (r *ReconcilePerconaXtraDBCluster) waitPodRestart(cr *api.PerconaXtraDBCluster, updateRevision string, pod *corev1.Pod, waitLimit int, logger logr.Logger) error {
	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			err := r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
			if err != nil && !k8serrors.IsNotFound(err) {
				return false, errors.Wrap(err, "fetch pod")
			}

			// We update status in every loop to not wait until the end of smart update
			if err := r.updateStatus(cr, true, nil); err != nil {
				return false, errors.Wrap(err, "update status")
			}

			ready := false
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == "pxc" {
					ready = container.Ready
				}
			}

			if pod.Status.Phase == corev1.PodFailed {
				return false, errors.Errorf("pod %s is in failed phase", pod.Name)
			}

			if pod.Status.Phase == corev1.PodRunning && pod.ObjectMeta.Labels["controller-revision-hash"] == updateRevision && ready {
				logger.Info("pod is running", "pod name", pod.Name)
				return true, nil
			}

			return false, nil
		})
}

func isPXC(sfs api.StatefulApp) bool {
	return sfs.Labels()["app.kubernetes.io/component"] == "pxc"
}

func isHAproxy(sfs api.StatefulApp) bool {
	return sfs.Labels()["app.kubernetes.io/component"] == "haproxy"
}

func isProxySQL(sfs api.StatefulApp) bool {
	return sfs.Labels()["app.kubernetes.io/component"] == "proxysql"
}

func (r *ReconcilePerconaXtraDBCluster) isBackupRunning(cr *api.PerconaXtraDBCluster) (bool, error) {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := r.client.List(context.TODO(), &bcpList, &client.ListOptions{Namespace: cr.Namespace}); err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to get backup object")
	}

	for _, bcp := range bcpList.Items {
		if bcp.Spec.PXCCluster != cr.Name {
			continue
		}

		if bcp.Status.State == api.BackupRunning || bcp.Status.State == api.BackupStarting {
			return true, nil
		}
	}

	return false, nil
}

func (r *ReconcilePerconaXtraDBCluster) isRestoreRunning(clusterName, namespace string) (bool, error) {
	restoreList := api.PerconaXtraDBClusterRestoreList{}

	err := r.client.List(context.TODO(), &restoreList, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to get restore list")
	}

	for _, v := range restoreList.Items {
		if v.Spec.PXCCluster != clusterName {
			continue
		}

		switch v.Status.State {
		case api.RestoreStarting, api.RestoreStopCluster, api.RestoreRestore,
			api.RestoreStartCluster, api.RestorePITR:
			return true, nil
		}
	}
	return false, nil
}

func getCustomConfigHashHex(strData map[string]string, binData map[string][]byte) (string, error) {
	content := struct {
		StrData map[string]string `json:"str_data,omitempty"`
		BinData map[string][]byte `json:"bin_data,omitempty"`
	}{
		StrData: strData,
		BinData: binData,
	}

	allData, err := json.Marshal(content)
	if err != nil {
		return "", errors.Wrap(err, "failed to concat data for config hash")
	}

	hashHex := fmt.Sprintf("%x", md5.Sum(allData))

	return hashHex, nil
}

func (r *ReconcilePerconaXtraDBCluster) getConfigHash(cr *api.PerconaXtraDBCluster, sts *appsv1.StatefulSet) (string, error) {
	ls := sts.Labels

	name := types.NamespacedName{
		Namespace: cr.Namespace,
		Name:      ls["app.kubernetes.io/instance"] + "-" + ls["app.kubernetes.io/component"],
	}

	obj, err := r.getFirstExisting(name, &corev1.Secret{}, &corev1.ConfigMap{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get custom config")
	}

	switch obj := obj.(type) {
	case *corev1.Secret:
		return getCustomConfigHashHex(obj.StringData, obj.Data)
	case *corev1.ConfigMap:
		return getCustomConfigHashHex(obj.Data, obj.BinaryData)
	default:
		return fmt.Sprintf("%x", md5.Sum([]byte{})), nil
	}
}

func (r *ReconcilePerconaXtraDBCluster) getFirstExisting(name types.NamespacedName, objs ...client.Object) (client.Object, error) {
	for _, o := range objs {
		err := r.client.Get(context.TODO(), name, o)
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			return o, nil
		}
	}
	return nil, nil
}

func (r *ReconcilePerconaXtraDBCluster) getSecretHash(cr *api.PerconaXtraDBCluster, secretName string, allowNonExistingSecret bool) (string, error) {
	secretObj := corev1.Secret{}
	if err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&secretObj,
	); err != nil && k8serrors.IsNotFound(err) && allowNonExistingSecret {
		return "", nil
	} else if err != nil {
		return "", err
	}

	secretString := fmt.Sprintln(secretObj.Data)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(secretString)))

	return hash, nil
}
