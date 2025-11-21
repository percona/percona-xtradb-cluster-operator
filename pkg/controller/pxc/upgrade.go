package pxc

import (
	"context"
	"crypto/md5"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util"
)

var NoProxyDetectedError = errors.New("can't detect enabled proxy, please enable HAProxy or ProxySQL")

func (r *ReconcilePerconaXtraDBCluster) updatePod(
	ctx context.Context,
	sfs api.StatefulApp,
	podSpec *api.PodSpec,
	cr *api.PerconaXtraDBCluster,
	newAnnotations map[string]string,
	smartUpdate bool,
) error {
	log := logf.FromContext(ctx)

	if cr.PVCResizeInProgress() {
		log.V(1).Info("PVC resize in progress, skipping statefulset", "sfs", sfs.Name())
		return nil
	}

	res, err := r.reconcileConfigMaps(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "reconcile config")
	}

	// don't create statefulset if configmap is just created or updated
	if res != controllerutil.OperationResultNone {
		return nil
	}

	// embed DB configuration hash
	configHash, err := r.getConfigHash(ctx, cr, sfs)
	if err != nil {
		return errors.Wrap(err, "get config hash")
	}

	envVarsHash, err := r.getSecretHash(cr, cr.Spec.PXC.EnvVarsSecretName, true)
	if isHAproxy(sfs) {
		envVarsHash, err = r.getSecretHash(cr, cr.Spec.HAProxy.EnvVarsSecretName, true)
	} else if isProxySQL(sfs) {
		envVarsHash, err = r.getSecretHash(cr, cr.Spec.ProxySQL.EnvVarsSecretName, true)
	}
	if err != nil {
		return errors.Wrap(err, "get env vars secret hash")
	}

	var vaultConfigHash, sslHash, sslInternalHash string
	if !isHAproxy(sfs) {
		vaultConfigHash, err = r.getSecretHash(cr, cr.Spec.VaultSecretName, true)
		if err != nil {
			return errors.Wrap(err, "get vault secret hash")
		}
		sslHash, err = r.getSecretHash(cr, cr.Spec.PXC.SSLSecretName, !cr.TLSEnabled())
		if err != nil {
			return errors.Wrap(err, "get ssl secret hash")
		}
		sslInternalHash, err = r.getSecretHash(cr, cr.Spec.PXC.SSLInternalSecretName, !cr.TLSEnabled())
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Wrap(err, "get internal ssl secret hash")
		}
	}

	hashAnnotations := map[string]string{
		"percona.com/configuration-hash":     configHash,
		"percona.com/ssl-hash":               sslHash,
		"percona.com/ssl-internal-hash":      sslInternalHash,
		"percona.com/vault-config-hash":      vaultConfigHash,
		"percona.com/env-secret-config-hash": envVarsHash,
	}

	secrets := new(corev1.Secret)
	err = r.client.Get(ctx, types.NamespacedName{
		Name: "internal-" + cr.Name, Namespace: cr.Namespace,
	}, secrets)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "get internal secret")
	}

	initImageName, err := k8s.GetInitImage(ctx, cr, r.client)
	if err != nil {
		return errors.Wrap(err, "failed to get initImage")
	}

	errStsWillBeDeleted := errors.New("will be deleted")

	err = k8sretry.OnError(k8sretry.DefaultRetry, func(err error) bool {
		return k8serrors.IsAlreadyExists(err) || k8serrors.IsConflict(err)
	}, func() error {
		currentSet := sfs.StatefulSet()
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(currentSet), currentSet); client.IgnoreNotFound(err) != nil {
			return errors.Wrap(err, "get statefulset")
		}
		if !currentSet.DeletionTimestamp.IsZero() {
			return errStsWillBeDeleted
		}
		annotations := currentSet.Spec.Template.Annotations
		labels := currentSet.Spec.Template.Labels

		sts, err := pxc.StatefulSet(ctx, r.client, sfs, podSpec, cr, secrets, initImageName, r.getConfigVolume)
		if err != nil {
			return errors.Wrap(err, "construct statefulset")
		}
		// Keep same volumeClaimTemplates labels if statefulset already exists.
		// We can't update volumeClaimTemplates.
		if cr.CompareVersionWith("1.16.0") >= 0 {
			for i, pvc := range currentSet.Spec.VolumeClaimTemplates {
				sts.Spec.VolumeClaimTemplates[i].Labels = pvc.Labels
			}
		}

		// If currentSet is not found, both annotations and labels will be nil.
		// In such cases, MergeMaps will initialize a new map and return it.
		annotations = util.MergeMaps(annotations, sts.Spec.Template.Annotations, newAnnotations)
		labels = util.MergeMaps(labels, sts.Spec.Template.Labels)

		for k, v := range hashAnnotations {
			if v != "" || k == "percona.com/configuration-hash" {
				annotations[k] = v
			}
		}

		sts.Spec.Template.Annotations = annotations
		sts.Spec.Template.Labels = labels

		if err := k8s.SetControllerReference(cr, sts, r.scheme); err != nil {
			return errors.Wrap(err, "set controller reference")
		}
		err = r.createOrUpdate(ctx, sts)
		if err != nil {
			return errors.Wrap(err, "update error")
		}
		return nil
	})
	if err != nil {
		if k8serrors.IsNotFound(err) || errors.Is(err, errStsWillBeDeleted) {
			return nil
		}
		return errors.Wrap(err, "failed to create or update sts")
	}

	if cr.Spec.UpdateStrategy != api.SmartUpdateStatefulSetStrategyType {
		return nil
	}

	if smartUpdate {
		if err := r.smartUpdate(ctx, sfs, cr); err != nil {
			return errors.Wrap(err, "smart update")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) smartUpdate(ctx context.Context, sfs api.StatefulApp, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	if !isPXC(sfs) {
		return nil
	}

	if cr.Spec.Pause {
		return nil
	}

	if cr.HAProxyEnabled() && cr.Status.HAProxy.Status != api.AppStateReady {
		return nil
	}

	if cr.ProxySQLEnabled() && cr.Status.ProxySQL.Status != api.AppStateReady {
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

	log.Info("statefulSet was changed, run smart update")

	running, err := r.isBackupRunning(cr)
	if err != nil {
		log.Error(err, "can't start 'SmartUpdate'")
		return nil
	}
	if running {
		log.Info("can't start/continue 'SmartUpdate': backup is running")
		return nil
	}

	primary, err := r.getPrimaryPod(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "get primary pod")
	}
	for _, pod := range list.Items {
		if pod.Status.PodIP == primary || pod.Name == primary {
			primary = fmt.Sprintf("%s.%s.%s", pod.Name, currentSet.Name, currentSet.Namespace)
			break
		}
	}

	log.Info("primary pod", "pod", primary)

	waitLimit := 2 * 60 * 60 // 2 hours
	if cr.Spec.PXC.LivenessProbes.InitialDelaySeconds != api.DefaultInitialDelaySeconds {
		waitLimit = int(cr.Spec.PXC.LivenessProbes.InitialDelaySeconds)
	} else if cr.Spec.PXC.LivenessInitialDelaySeconds != nil {
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
			log.Info("apply changes to secondary pod", "pod", pod.Name)
			if err := r.applyNWait(ctx, cr, currentSet, &pod, waitLimit); err != nil {
				return errors.Wrap(err, "failed to apply changes")
			}
		}
	}

	log.Info("apply changes to primary pod", "pod", primaryPod.Name)
	if err := r.applyNWait(ctx, cr, currentSet, &primaryPod, waitLimit); err != nil {
		return errors.Wrap(err, "failed to apply changes")
	}

	log.Info("smart update finished")

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) applyNWait(ctx context.Context, cr *api.PerconaXtraDBCluster, sfs *appsv1.StatefulSet, pod *corev1.Pod, waitLimit int) error {
	log := logf.FromContext(ctx)

	if pod.ObjectMeta.Labels["controller-revision-hash"] == sfs.Status.UpdateRevision {
		log.Info("pod already updated", "pod", pod.Name)
	} else {
		if err := r.client.Delete(ctx, pod); err != nil {
			return errors.Wrap(err, "failed to delete pod")
		}
	}

	orderInSts, err := getPodOrderInSts(sfs.Name, pod.Name)
	if err != nil {
		return errors.Errorf("compute pod order err, sfs name: %s, pod name: %s", sfs.Name, pod.Name)
	}
	if int32(orderInSts) >= *sfs.Spec.Replicas {
		log.Info("sfs scaled down, pod will not be started", "sfs", sfs.Name, "pod", pod.Name)
		return nil
	}

	if err := r.waitPodRestart(ctx, cr, sfs.Status.UpdateRevision, pod, waitLimit); err != nil {
		return errors.Wrap(err, "failed to wait pod")
	}

	if err := r.waitPXCSynced(cr, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, waitLimit); err != nil {
		return errors.Wrap(err, "failed to wait pxc sync")
	}

	if err := r.waitHostgroups(ctx, cr, sfs.Name, pod, waitLimit); err != nil {
		return errors.Wrap(err, "failed to wait hostgroups status")
	}

	if err := r.waitUntilOnline(ctx, cr, sfs.Name, pod, waitLimit); err != nil {
		return errors.Wrap(err, "failed to wait pxc status")
	}

	return nil
}

func getPodOrderInSts(stsName string, podName string) (int, error) {
	return strconv.Atoi(podName[len(stsName)+1:])
}

func (r *ReconcilePerconaXtraDBCluster) waitHostgroups(ctx context.Context, cr *api.PerconaXtraDBCluster, sfsName string, pod *corev1.Pod, waitLimit int) error {
	if !cr.Spec.ProxySQLEnabled() {
		return nil
	}

	database, err := r.connectProxy(cr)
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

			logf.FromContext(ctx).Info("pod present in hostgroups", "pod", pod.Name)
			return true, nil
		})
}

func (r *ReconcilePerconaXtraDBCluster) waitUntilOnline(ctx context.Context, cr *api.PerconaXtraDBCluster, sfsName string, pod *corev1.Pod, waitLimit int) error {
	if !cr.Spec.ProxySQLEnabled() {
		return nil
	}

	database, err := r.connectProxy(cr)
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

			logf.FromContext(ctx).Info("pod is online", "pod", pod.Name)
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

// connectProxy returns a new connection through the proxy (ProxySQL or HAProxy)
func (r *ReconcilePerconaXtraDBCluster) connectProxy(cr *api.PerconaXtraDBCluster) (queries.Database, error) {
	var database queries.Database
	var user, host string
	var port, proxySize int32

	if cr.ProxySQLEnabled() {
		user = users.ProxyAdmin
		host = fmt.Sprintf("%s-proxysql-unready.%s", cr.ObjectMeta.Name, cr.Namespace)
		proxySize = cr.Spec.ProxySQL.Size
		port = 6032
	} else if cr.HAProxyEnabled() {
		user = users.Monitor
		host = fmt.Sprintf("%s-haproxy.%s", cr.Name, cr.Namespace)
		proxySize = cr.Spec.HAProxy.Size

		hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
		if err != nil {
			return database, errors.Wrap(err, "check if config has proxy_protocol_networks key")
		}

		port = 3306
		if hasKey && cr.CompareVersionWith("1.6.0") >= 0 {
			port = 33062
		}
	} else {
		return database, NoProxyDetectedError
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

func (r *ReconcilePerconaXtraDBCluster) getPrimaryPod(ctx context.Context, cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := r.connectProxy(cr)
	if err != nil {
		if errors.Is(err, NoProxyDetectedError) && cr.Spec.PXC.Size == 1 {
			firstReadyPod := func() (string, error) {
				sts := statefulset.NewNode(cr)

				podList := new(corev1.PodList)
				if err := r.client.List(ctx, podList, &client.ListOptions{
					Namespace:     cr.Namespace,
					LabelSelector: labels.SelectorFromSet(sts.Labels()),
				}); err != nil {
					return "", errors.Wrap(err, "get pod list")
				}

				readyPods := make([]corev1.Pod, 0)
				for _, pod := range podList.Items {
					if isPodReady(pod) {
						readyPods = append(readyPods, pod)
					}
				}
				if len(readyPods) == 0 {
					return "", errors.New("no ready pxc pods")
				}
				if len(readyPods) != int(cr.Spec.PXC.Size) {
					return "", errors.New("waiting for pxc resize")
				}

				return readyPods[0].Status.PodIP, nil
			}
			host, rerr := firstReadyPod()
			if rerr == nil {
				return host, nil
			}

			err = stdErrors.Join(rerr, err)
		}
		return "", errors.Wrap(err, "failed to get proxy connection")
	}
	defer conn.Close()

	if cr.HAProxyEnabled() {
		host, err := conn.Hostname()
		if err != nil {
			return "", err
		}

		return host, nil
	}

	return conn.PrimaryHost()
}

func (r *ReconcilePerconaXtraDBCluster) waitPXCSynced(cr *api.PerconaXtraDBCluster, host string, waitLimit int) error {
	secrets := cr.Spec.SecretsName
	port := int32(3306)
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secrets = "internal-" + cr.Name
		port = int32(33062)
	}

	database, err := queries.New(r.client, cr.Namespace, secrets, users.Root, host, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
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

func (r *ReconcilePerconaXtraDBCluster) waitPodRestart(ctx context.Context, cr *api.PerconaXtraDBCluster, updateRevision string, pod *corev1.Pod, waitLimit int) error {
	log := logf.FromContext(ctx)

	return retry(time.Second*10, time.Duration(waitLimit)*time.Second,
		func() (bool, error) {
			err := r.client.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
			if err != nil && !k8serrors.IsNotFound(err) {
				return false, errors.Wrap(err, "fetch pod")
			}

			// We update status in every loop to not wait until the end of smart update
			if err := r.updateStatus(ctx, cr, true, nil); err != nil {
				return false, errors.Wrap(err, "update status")
			}

			ready := false
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == "pxc" {
					ready = container.Ready

					if container.State.Waiting != nil {
						switch container.State.Waiting.Reason {
						case "ImagePullBackOff", "ErrImagePull", "CrashLoopBackOff":
							return false, errors.Errorf("pod %s is in %s state", pod.Name, container.State.Waiting.Reason)
						default:
							log.Info("pod is waiting", "pod", pod.Name, "reason", container.State.Waiting.Reason)
						}
					}
				}
			}

			for _, cond := range pod.Status.Conditions {
				if cond.Type != corev1.PodScheduled {
					continue
				}

				if cond.Status != corev1.ConditionFalse {
					continue
				}

				if time.Since(cond.LastTransitionTime.Time) > time.Duration(120*time.Second) {
					return false, errors.Errorf("pod %s is not scheduled: %s", pod.Name, cond.Message)
				}
			}

			if pod.Status.Phase == corev1.PodFailed {
				return false, errors.Errorf("pod %s is in failed phase", pod.Name)
			}

			if pod.Status.Phase != corev1.PodRunning {
				log.Info("Pod is not running", "pod", pod.Name, "phase", pod.Status.Phase)
				return false, nil
			}

			if pod.ObjectMeta.Labels["controller-revision-hash"] != updateRevision {
				log.Info(
					"Pod is not updated",
					"pod", pod.Name,
					"currentRevision", pod.ObjectMeta.Labels["controller-revision-hash"],
					"targetRevision", updateRevision,
				)
				return false, nil
			}

			if ready {
				log.Info("Pod is updated, running and ready", "pod", pod.Name)
				return true, nil
			}

			return false, nil
		})
}

func isPXC(sfs api.StatefulApp) bool {
	return sfs.Labels()[naming.LabelAppKubernetesComponent] == "pxc"
}

func isHAproxy(sfs api.StatefulApp) bool {
	return sfs.Labels()[naming.LabelAppKubernetesComponent] == "haproxy"
}

func isProxySQL(sfs api.StatefulApp) bool {
	return sfs.Labels()[naming.LabelAppKubernetesComponent] == "proxysql"
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
		case api.RestoreStopCluster, api.RestoreRestore,
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

func (r *ReconcilePerconaXtraDBCluster) getConfigHash(ctx context.Context, cr *api.PerconaXtraDBCluster, sfs api.StatefulApp) (string, error) {
	ls := sfs.Labels()

	name := types.NamespacedName{
		Namespace: cr.Namespace,
		Name:      ls[naming.LabelAppKubernetesInstance] + "-" + ls[naming.LabelAppKubernetesComponent],
	}

	obj, err := r.getFirstExisting(ctx, name, &corev1.Secret{}, &corev1.ConfigMap{})
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

func (r *ReconcilePerconaXtraDBCluster) getFirstExisting(ctx context.Context, name types.NamespacedName, objs ...client.Object) (client.Object, error) {
	for _, o := range objs {
		err := r.client.Get(ctx, name, o)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if err == nil {
			return o, nil
		}
	}
	return nil, nil
}

func (r *ReconcilePerconaXtraDBCluster) getSecretHash(cr *api.PerconaXtraDBCluster, secretName string, allowNonExistingSecret bool) (string, error) {
	if allowNonExistingSecret && secretName == "" {
		return "", nil
	}
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
