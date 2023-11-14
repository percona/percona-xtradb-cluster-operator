package pxc

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const replicationPodLabel = "percona.com/replicationPod"

var minReplicationVersion = version.Must(version.NewVersion("8.0.23-14.1"))

func (r *ReconcilePerconaXtraDBCluster) ensurePxcPodServices(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
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
	return r.removeOutdatedServices(ctx, cr)
}

func (r *ReconcilePerconaXtraDBCluster) removeOutdatedServices(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	//needed for labels
	svc := NewExposedPXCService("", cr)

	svcNames := make(map[string]struct{}, cr.Spec.PXC.Size)
	for i := 0; i < int(cr.Spec.PXC.Size); i++ {
		svcNames[fmt.Sprintf("%s-pxc-%d", cr.Name, i)] = struct{}{}
	}

	svcList := &corev1.ServiceList{}
	err := r.client.List(ctx,
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
			err = r.client.Delete(ctx, &service)
			if err != nil {
				return errors.Wrapf(err, "failed to delete service %s", service.Name)
			}
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileReplication(ctx context.Context, cr *api.PerconaXtraDBCluster, replicaPassUpdated bool) error {
	log := logf.FromContext(ctx)

	if cr.Status.PXC.Ready < 1 || cr.Spec.Pause {
		return nil
	}

	sfs := statefulset.NewNode(cr)

	listRaw := corev1.PodList{}
	err := r.client.List(ctx,
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

	primaryPod, err := r.getPrimaryPod(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "get primary pxc pod")
	}

	pass, err := r.getUserPass(ctx, cr, users.Operator)
	if err != nil {
		return errors.Wrap(err, "failed to get operator password")
	}
	primaryDB := queries.NewExec(&primaryPod, r.clientcmd, users.Operator, pass, primaryPod.Name+"."+cr.Name+"-pxc."+cr.Namespace)

	dbVer, err := primaryDB.VersionExec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current db version")
	}

	if version.Must(version.NewVersion(dbVer)).Compare(minReplicationVersion) < 0 {
		return nil
	}

	err = removeOutdatedChannels(ctx, primaryDB, cr.Spec.PXC.ReplicationChannels)
	if err != nil {
		return errors.Wrap(err, "remove outdated replication channels")
	}

	err = r.checkReadonlyStatus(ctx, cr.Spec.PXC.ReplicationChannels, podList, cr, r.client)
	if err != nil {
		return errors.Wrap(err, "failed to ensure cluster readonly status")
	}

	if len(cr.Spec.PXC.ReplicationChannels) == 0 {
		return deleteReplicaLabels(ctx, r.client, podList)
	}

	if cr.Spec.PXC.ReplicationChannels[0].IsSource {
		return deleteReplicaLabels(ctx, r.client, podList)
	}

	// if primary pod is not a replica, we need to make it as replica, and stop replication on other pods
	for _, pod := range podList {
		if pod.Name == primaryPod.Name {
			continue
		}
		if _, ok := pod.Labels[replicationPodLabel]; ok {
			pass, err := r.getUserPass(ctx, cr, users.Operator)
			if err != nil {
				return errors.Wrap(err, "failed to get operator password")
			}
			db := queries.NewExec(&pod, r.clientcmd, users.Operator, pass, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace)

			err = db.StopAllReplicationExec(ctx)
			if err != nil {
				return errors.Wrapf(err, "stop replication on pod %s", pod.Name)
			}
			delete(pod.Labels, replicationPodLabel)
			err = r.client.Update(ctx, &pod)
			if err != nil {
				return errors.Wrap(err, "failed to remove primary label from secondary pod")
			}
		}
	}

	if _, ok := primaryPod.Labels[replicationPodLabel]; !ok {
		primaryPod.Labels[replicationPodLabel] = "true"
		err = r.client.Update(ctx, &primaryPod)
		if err != nil {
			return errors.Wrap(err, "add label to main replica pod")
		}
		log.Info("Replication pod has changed", "new replication pod", primaryPod.Name)
	}

	sysUsersSecretObj := corev1.Secret{}
	err = r.client.Get(ctx,
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
		err = handleReplicaPasswordChange(ctx, primaryDB, string(sysUsersSecretObj.Data[users.Replication]))
		if err != nil {
			return errors.Wrap(err, "failed to change replication password")
		}
	}

	for _, channel := range cr.Spec.PXC.ReplicationChannels {
		if channel.IsSource {
			continue
		}

		currConf := currentReplicaConfig(channel.Name, cr.Status.PXCReplication)

		err = manageReplicationChannel(ctx, log, primaryDB, channel, currConf, string(sysUsersSecretObj.Data[users.Replication]))
		if err != nil {
			return errors.Wrapf(err, "manage replication channel %s", channel.Name)
		}
		setReplicationChannelStatus(cr, channel)
	}

	return r.updateStatus(cr, false, nil)
}

func handleReplicaPasswordChange(ctx context.Context, db *queries.DatabaseExec, newPass string) error {
	channels, err := db.CurrentReplicationChannelsExec(ctx)
	if err != nil {
		return errors.Wrap(err, "get current replication channels")
	}

	for _, channel := range channels {
		err := db.ChangeChannelPasswordExec(ctx, channel, newPass)
		if err != nil {
			return errors.Wrapf(err, "change password for channel %s", channel)
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) checkReadonlyStatus(ctx context.Context, channels []api.ReplicationChannel, pods []corev1.Pod, cr *api.PerconaXtraDBCluster, client client.Client) error {
	isReplica := false
	if len(channels) > 0 {
		isReplica = !channels[0].IsSource
	}

	for _, pod := range pods {
		pass, err := r.getUserPass(ctx, cr, users.Operator)
		if err != nil {
			return errors.Wrap(err, "failed to get operator password")
		}
		db := queries.NewExec(&pod, r.clientcmd, users.Operator, pass, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace)

		readonly, err := db.IsReadonlyExec(ctx)
		if err != nil {
			return errors.Wrap(err, "check readonly status")
		}

		if isReplica && readonly || (!isReplica && !readonly) {
			continue
		}

		if isReplica && !readonly {
			err = db.EnableReadonlyExec(ctx)
		}

		if !isReplica && readonly {
			err = db.DisableReadonlyExec(ctx)
		}
		if err != nil {
			return errors.Wrap(err, "enable or disable readonly mode")
		}

	}
	return nil
}

func removeOutdatedChannels(ctx context.Context, db *queries.DatabaseExec, currentChannels []api.ReplicationChannel) error {
	dbChannels, err := db.CurrentReplicationChannelsExec(ctx)
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
		err = db.StopReplicationExec(ctx, channelToRemove)
		if err != nil {
			return errors.Wrapf(err, "stop replication for channel %s", channelToRemove)
		}

		srcList, err := db.ReplicationChannelSourcesExec(ctx, channelToRemove)
		if err != nil && err != queries.ErrNotFound {
			return errors.Wrapf(err, "get src list for outdated channel %s", channelToRemove)
		}
		for _, v := range srcList {
			err = db.DeleteReplicationSourceExec(ctx, channelToRemove, v.Host, v.Port)
			if err != nil {
				return errors.Wrapf(err, "delete replication source for outdated channel %s", channelToRemove)
			}
		}
	}
	return nil
}

func manageReplicationChannel(ctx context.Context, log logr.Logger, primaryDB *queries.DatabaseExec, channel api.ReplicationChannel, currConf api.ReplicationChannelConfig, replicaPW string) error {
	currentSources, err := primaryDB.ReplicationChannelSourcesExec(ctx, channel.Name)
	if err != nil && err != queries.ErrNotFound {
		return errors.Wrapf(err, "get current replication sources for channel %s", channel.Name)
	}

	replicationStatus, err := primaryDB.ReplicationStatusExec(ctx, channel.Name)
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
		err = primaryDB.StopReplicationExec(ctx, channel.Name)
		if err != nil {
			return errors.Wrapf(err, "stop replication for channel %s", channel.Name)
		}
	}

	for _, src := range currentSources {
		err = primaryDB.DeleteReplicationSourceExec(ctx, channel.Name, src.Host, src.Port)
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
		err := primaryDB.AddReplicationSourceExec(ctx, channel.Name, src.Host, src.Port, src.Weight)
		if err != nil {
			return errors.Wrapf(err, "add replication source for channel %s", channel.Name)
		}
	}

	return primaryDB.StartReplicationExec(ctx, replicaPW, queries.ReplicationConfig{
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

func deleteReplicaLabels(ctx context.Context, client client.Client, pods []corev1.Pod) error {
	for _, pod := range pods {
		if _, ok := pod.Labels[replicationPodLabel]; ok {
			delete(pod.Labels, replicationPodLabel)
			err := client.Update(ctx, &pod)
			if err != nil {
				return errors.Wrap(err, "failed to remove replication label from pod")
			}
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) removePxcPodServices(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	if cr.Spec.Pause {
		return nil
	}

	//needed for labels
	svc := NewExposedPXCService("", cr)

	svcList := &corev1.ServiceList{}
	err := r.client.List(ctx,
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
		err = r.client.Delete(ctx, &service)
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
