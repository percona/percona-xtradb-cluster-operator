package pxc

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const replicationPodLabel = "percona.com/replicationPod"

var minReplicationVersion = version.Must(version.NewVersion("8.0.23-14.1"))

func (r *ReconcilePerconaXtraDBCluster) ensurePxcPodServices(cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.Pause {
		return nil
	}

	isBackupRunning, err := r.isBackupRunning(cr)
	if err != nil {
		return errors.Wrap(err, "failed to check if backup is running")
	}

	if isBackupRunning {
		return nil
	}

	isRestoreRunning, err := r.isRestoreRunning(cr.Name, cr.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to check if restore is running")
	}

	if isRestoreRunning {
		return nil
	}

	for i := 0; i < int(cr.Spec.PXC.Size); i++ {
		svcName := fmt.Sprintf("%s-pxc-%d", cr.Name, i)
		svc := NewExposedPXCService(svcName, cr)

		err := setControllerReference(cr, svc, r.scheme)
		if err != nil {
			return errors.Wrap(err, "failed to set owner to external service")
		}

		err = r.createOrUpdateService(cr, svc, len(cr.Spec.PXC.Expose.Annotations) == 0)
		if err != nil {
			return errors.Wrap(err, "failed to ensure pxc service")
		}
	}
	return r.removeOutdatedServices(cr)
}

func (r *ReconcilePerconaXtraDBCluster) removeOutdatedServices(cr *api.PerconaXtraDBCluster) error {
	//needed for labels
	svc := NewExposedPXCService("", cr)

	svcNames := make(map[string]struct{}, cr.Spec.PXC.Size)
	for i := 0; i < int(cr.Spec.PXC.Size); i++ {
		svcNames[fmt.Sprintf("%s-pxc-%d", cr.Name, i)] = struct{}{}
	}

	svcList := &corev1.ServiceList{}
	err := r.client.List(context.TODO(),
		svcList,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(svc.Labels),
		},
	)

	if err != nil {
		return errors.Wrap(err, "failed to list external services")
	}

	for _, service := range svcList.Items {
		if _, ok := svcNames[service.Name]; !ok {
			err = r.client.Delete(context.TODO(), &service)
			if err != nil {
				return errors.Wrapf(err, "failed to delete service %s", service.Name)
			}
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileReplication(cr *api.PerconaXtraDBCluster, replicaPassUpdated bool) error {
	if cr.Status.PXC.Ready < 1 || cr.Spec.Pause {
		return nil
	}

	logger := r.logger(cr.Name, cr.Namespace)

	sfs := statefulset.NewNode(cr)

	listRaw := corev1.PodList{}
	err := r.client.List(context.TODO(),
		&listRaw,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
	)
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "get pod list")
	}

	// we need only running pods, because we unable to
	// connect to failed/pending pods
	podList := make([]corev1.Pod, 0)
	for _, pod := range listRaw.Items {
		if isPodReady(pod) {
			podList = append(podList, pod)
		}
	}

	primary, err := r.getPrimaryPod(cr)
	if err != nil {
		return errors.Wrap(err, "get primary pxc pod")
	}

	var primaryPod *corev1.Pod
	for _, pod := range podList {
		if pod.Status.PodIP == primary || pod.Name == primary || strings.HasPrefix(primary, fmt.Sprintf("%s.%s.%s", pod.Name, sfs.StatefulSet().Name, cr.Namespace)) {
			primaryPod = &pod
			break
		}
	}

	if primaryPod == nil {
		logger.Info("Unable to find primary pod for replication. No pod with name or ip like this", "primary name", primary)
		return nil
	}

	user := "operator"
	port := int32(33062)

	primaryDB, err := queries.New(r.client, cr.Namespace, internalSecretsPrefix+cr.Name, user, primaryPod.Name+"."+cr.Name+"-pxc."+cr.Namespace, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
	if err != nil {
		return errors.Wrapf(err, "failed to connect to pod %s", primaryPod.Name)
	}

	defer primaryDB.Close()

	dbVer, err := primaryDB.Version()
	if err != nil {
		return errors.Wrap(err, "failed to get current db version")
	}

	if version.Must(version.NewVersion(dbVer)).Compare(minReplicationVersion) < 0 {
		return nil
	}

	err = removeOutdatedChannels(primaryDB, cr.Spec.PXC.ReplicationChannels)
	if err != nil {
		return errors.Wrap(err, "remove outdated replication channels")
	}

	err = checkReadonlyStatus(cr.Spec.PXC.ReplicationChannels, podList, cr, r.client)
	if err != nil {
		return errors.Wrap(err, "failed to ensure cluster readonly status")
	}

	if len(cr.Spec.PXC.ReplicationChannels) == 0 {
		return deleteReplicaLabels(r.client, podList)
	}

	if cr.Spec.PXC.ReplicationChannels[0].IsSource {
		return deleteReplicaLabels(r.client, podList)
	}

	// if primary pod is not a replica, we need to make it as replica, and stop replication on other pods
	for _, pod := range podList {
		if pod.Name == primaryPod.Name {
			continue
		}
		if _, ok := pod.Labels[replicationPodLabel]; ok {
			db, err := queries.New(r.client, cr.Namespace, internalSecretsPrefix+cr.Name, user, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
			if err != nil {
				return errors.Wrapf(err, "failed to connect to pod %s", pod.Name)
			}
			err = db.StopAllReplication()
			db.Close()
			if err != nil {
				return errors.Wrapf(err, "stop replication on pod %s", pod.Name)
			}
			delete(pod.Labels, replicationPodLabel)
			err = r.client.Update(context.TODO(), &pod)
			if err != nil {
				return errors.Wrap(err, "failed to remove primary label from secondary pod")
			}
		}
	}

	if _, ok := primaryPod.Labels[replicationPodLabel]; !ok {
		primaryPod.Labels[replicationPodLabel] = "true"
		err = r.client.Update(context.TODO(), primaryPod)
		if err != nil {
			return errors.Wrap(err, "add label to main replica pod")
		}
		r.logger(cr.Name, cr.Namespace).Info("Replication pod has changed", "new replication pod", primaryPod.Name)
	}

	sysUsersSecretObj := corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      internalSecretsPrefix + cr.Name,
		},
		&sysUsersSecretObj,
	)
	if err != nil {
		return errors.Wrap(err, "get secrets")
	}

	if replicaPassUpdated {
		err = handleReplicaPasswordChange(primaryDB, string(sysUsersSecretObj.Data["replication"]))
		if err != nil {
			return errors.Wrap(err, "failed to change replication password")
		}
	}

	for _, channel := range cr.Spec.PXC.ReplicationChannels {
		if channel.IsSource {
			continue
		}

		currConf := currentReplicaConfig(channel.Name, cr.Status.PXCReplication)

		err = manageReplicationChannel(r.log, primaryDB, channel, currConf, string(sysUsersSecretObj.Data["replication"]))
		if err != nil {
			return errors.Wrapf(err, "manage replication channel %s", channel.Name)
		}
		setReplicationChannelStatus(cr, channel)
	}

	return r.updateStatus(cr, false, nil)
}

func handleReplicaPasswordChange(db queries.Database, newPass string) error {
	channels, err := db.CurrentReplicationChannels()
	if err != nil {
		return errors.Wrap(err, "get current replication channels")
	}

	for _, channel := range channels {
		err := db.ChangeChannelPassword(channel, newPass)
		if err != nil {
			return errors.Wrapf(err, "change password for channel %s", channel)
		}
	}
	return nil
}

func checkReadonlyStatus(channels []api.ReplicationChannel, pods []corev1.Pod, cr *api.PerconaXtraDBCluster, client client.Client) error {
	isReplica := false
	if len(channels) > 0 {
		isReplica = !channels[0].IsSource
	}

	for _, pod := range pods {
		db, err := queries.New(client, cr.Namespace, internalSecretsPrefix+cr.Name, "operator", pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, 33062, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
		if err != nil {
			return errors.Wrapf(err, "connect to pod %s", pod.Name)
		}
		defer db.Close()
		readonly, err := db.IsReadonly()
		if err != nil {
			return errors.Wrap(err, "check readonly status")
		}

		if isReplica && readonly || (!isReplica && !readonly) {
			continue
		}

		if isReplica && !readonly {
			err = db.EnableReadonly()
		}

		if !isReplica && readonly {
			err = db.DisableReadonly()
		}
		if err != nil {
			return errors.Wrap(err, "enable or disable readonly mode")
		}

	}
	return nil
}

func removeOutdatedChannels(db queries.Database, currentChannels []api.ReplicationChannel) error {
	dbChannels, err := db.CurrentReplicationChannels()
	if err != nil {
		return errors.Wrap(err, "get current replication channels")
	}

	if len(dbChannels) == 0 {
		return nil
	}

	toRemove := make(map[string]struct{})
	for _, v := range dbChannels {
		toRemove[v] = struct{}{}
	}

	for _, v := range currentChannels {
		if !v.IsSource {
			delete(toRemove, v.Name)
		}
	}

	if len(toRemove) == 0 {
		return nil
	}

	for channelToRemove := range toRemove {
		err = db.StopReplication(channelToRemove)
		if err != nil {
			return errors.Wrapf(err, "stop replication for channel %s", channelToRemove)
		}

		srcList, err := db.ReplicationChannelSources(channelToRemove)
		if err != nil && err != queries.ErrNotFound {
			return errors.Wrapf(err, "get src list for outdated channel %s", channelToRemove)
		}
		for _, v := range srcList {
			err = db.DeleteReplicationSource(channelToRemove, v.Host, v.Port)
			if err != nil {
				return errors.Wrapf(err, "delete replication source for outdated channel %s", channelToRemove)
			}
		}
	}
	return nil
}

func manageReplicationChannel(log logr.Logger, primaryDB queries.Database, channel api.ReplicationChannel, currConf api.ReplicationChannelConfig, replicaPW string) error {
	currentSources, err := primaryDB.ReplicationChannelSources(channel.Name)
	if err != nil && err != queries.ErrNotFound {
		return errors.Wrapf(err, "get current replication sources for channel %s", channel.Name)
	}

	replicationStatus, err := primaryDB.ReplicationStatus(channel.Name)
	if err != nil {
		return errors.Wrap(err, "failed to check replication status")
	}

	if !isSourcesChanged(channel.SourcesList, currentSources) {
		if replicationStatus == queries.ReplicationStatusError {
			log.Info("Replication for channel is not running. Please, check the replication status", "channel", channel.Name)
			return nil
		}

		if replicationStatus == queries.ReplicationStatusActive &&
			*channel.Config == currConf {
			return nil
		}
	}

	if replicationStatus == queries.ReplicationStatusActive {
		err = primaryDB.StopReplication(channel.Name)
		if err != nil {
			return errors.Wrapf(err, "stop replication for channel %s", channel.Name)
		}
	}

	for _, src := range currentSources {
		err = primaryDB.DeleteReplicationSource(channel.Name, src.Host, src.Port)
		if err != nil {
			return errors.Wrapf(err, "delete replication source for channel %s", channel.Name)
		}
	}

	maxWeight := 0
	maxWeightSrc := channel.SourcesList[0]

	for _, src := range channel.SourcesList {
		if src.Weight > maxWeight {
			maxWeightSrc = src
		}
		err := primaryDB.AddReplicationSource(channel.Name, src.Host, src.Port, src.Weight)
		if err != nil {
			return errors.Wrapf(err, "add replication source for channel %s", channel.Name)
		}
	}

	return primaryDB.StartReplication(replicaPW, queries.ReplicationConfig{
		Source: queries.ReplicationChannelSource{
			Name: channel.Name,
			Host: maxWeightSrc.Host,
			Port: maxWeightSrc.Port,
		},
		SourceRetryCount:   channel.Config.SourceRetryCount,
		SourceConnectRetry: channel.Config.SourceConnectRetry,
		SSL:                channel.Config.SSL,
		SSLSkipVerify:      channel.Config.SSLSkipVerify,
		CA:                 channel.Config.CA,
	})
}

func isSourcesChanged(new []api.ReplicationSource, old []queries.ReplicationChannelSource) bool {
	if len(new) != len(old) {
		return true
	}

	oldSrc := make(map[string]queries.ReplicationChannelSource)
	for _, src := range old {
		oldSrc[src.Host] = src
	}

	for _, v := range new {
		oldSource, ok := oldSrc[v.Host]
		if !ok {
			return true
		}
		if oldSource.Port != v.Port || oldSource.Weight != v.Weight {
			return true
		}
		delete(oldSrc, v.Host)
	}

	return len(oldSrc) != 0
}

func deleteReplicaLabels(client client.Client, pods []corev1.Pod) error {
	for _, pod := range pods {
		if _, ok := pod.Labels[replicationPodLabel]; ok {
			delete(pod.Labels, replicationPodLabel)
			err := client.Update(context.TODO(), &pod)
			if err != nil {
				return errors.Wrap(err, "failed to remove replication label from pod")
			}
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) removePxcPodServices(cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.Pause {
		return nil
	}

	//needed for labels
	svc := NewExposedPXCService("", cr)

	svcList := &corev1.ServiceList{}
	err := r.client.List(context.TODO(),
		svcList,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(svc.Labels),
		},
	)
	if k8serrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return errors.Wrap(err, "failed to list external services")
	}

	for _, service := range svcList.Items {
		err = r.client.Delete(context.TODO(), &service)
		if err != nil {
			return errors.Wrap(err, "failed to delete external service")
		}
	}
	return nil
}

func NewExposedPXCService(svcName string, cr *api.PerconaXtraDBCluster) *corev1.Service {
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "percona-xtradb-cluster",
				"app.kubernetes.io/instance":  cr.Name,
				"app.kubernetes.io/component": "external-service",
			},
			Annotations: cr.Spec.PXC.Expose.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql",
				},
			},
			LoadBalancerSourceRanges: cr.Spec.PXC.Expose.LoadBalancerSourceRanges,
			Selector: map[string]string{
				"statefulset.kubernetes.io/pod-name": svcName,
			},
		},
	}

	if cr.Spec.PXC.Expose.Type == corev1.ServiceTypeNodePort ||
		cr.Spec.PXC.Expose.Type == corev1.ServiceTypeLoadBalancer {
		switch cr.Spec.PXC.Expose.TrafficPolicy {
		case corev1.ServiceExternalTrafficPolicyTypeLocal, corev1.ServiceExternalTrafficPolicyTypeCluster:
			svc.Spec.ExternalTrafficPolicy = cr.Spec.PXC.Expose.TrafficPolicy
		default:
			svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
		}
	}

	switch cr.Spec.PXC.Expose.Type {
	case corev1.ServiceTypeNodePort:
		svc.Spec.Type = corev1.ServiceTypeNodePort
	case corev1.ServiceTypeLoadBalancer:
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	default:
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	}

	return svc
}

// isPodReady returns a boolean reflecting if a pod is in a "ready" state
func isPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		if condition.Type == corev1.PodReady {
			return true
		}
	}
	return false
}

func currentReplicaConfig(name string, status *api.ReplicationStatus) api.ReplicationChannelConfig {
	res := api.ReplicationChannelConfig{}
	if status == nil {
		return res
	}

	for _, v := range status.Channels {
		if v.Name == name {
			return v.ReplicationChannelConfig
		}
	}
	return res
}

func setReplicationChannelStatus(cr *api.PerconaXtraDBCluster, channel api.ReplicationChannel) {
	status := api.ReplicationChannelStatus{
		Name:                     channel.Name,
		ReplicationChannelConfig: *channel.Config,
	}

	if cr.Status.PXCReplication == nil {
		cr.Status.PXCReplication = &api.ReplicationStatus{
			Channels: []api.ReplicationChannelStatus{status},
		}
		return
	}

	for k, v := range cr.Status.PXCReplication.Channels {
		if channel.Name == v.Name {
			cr.Status.PXCReplication.Channels[k] = status
			return
		}
	}

	cr.Status.PXCReplication.Channels = append(cr.Status.PXCReplication.Channels, status)
}
