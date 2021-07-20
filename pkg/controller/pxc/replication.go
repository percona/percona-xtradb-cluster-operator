package pxc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
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

		err = r.createOrUpdate(svc)
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

func (r *ReconcilePerconaXtraDBCluster) reconcileReplication(cr *api.PerconaXtraDBCluster) error {
	if cr.Status.PXC.Ready < 1 || len(cr.Spec.PXC.ReplicationChannels) == 0 {
		return nil
	}

	sfs := statefulset.NewNode(cr)

	list := corev1.PodList{}
	if err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
	); err != nil {
		return errors.Wrap(err, "get pod list")
	}

	primary, err := r.getPrimaryPod(cr)
	if err != nil {
		return errors.Wrap(err, "get primary pxc pod")
	}

	for _, pod := range list.Items {
		if pod.Status.PodIP == primary || pod.Name == primary {
			primary = fmt.Sprintf("%s.%s.%s", pod.Name, sfs.Name(), cr.Namespace)
			break
		}
	}

	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name > list.Items[j].Name
	})

	var primaryPod corev1.Pod
	for _, pod := range list.Items {
		if strings.HasPrefix(primary, fmt.Sprintf("%s.%s.%s", pod.Name, sfs.Name(), cr.Namespace)) {
			primaryPod = pod
			break
		}
	}

	user := "root"
	port := int32(3306)
	if cr.CompareVersionWith("1.6.0") >= 0 {
		port = int32(33062)
	}

	primaryDB, err := queries.New(r.client, cr.Namespace, cr.Spec.SecretsName, user, primaryPod.Name+"."+cr.Name+"-pxc."+cr.Namespace, port)
	if err != nil {
		return errors.Wrap(err, "failed to connect to pod "+primaryPod.Name)
	}

	defer primaryDB.Close()

	isReplica, err := primaryDB.IsReplica()
	if err != nil {
		return errors.Wrap(err, "failed to check if primary is replica")
	}

	// if primary pod is not a replica, we need to make it as replica, and stop replication on other pods
	if !isReplica {
		r.log.Info("primary is not replica, stopping all replication")
		for _, pod := range list.Items {
			if pod.Name == primaryPod.Name {
				continue
			}

			db, err := queries.New(r.client, cr.Namespace, cr.Spec.SecretsName, user, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, port)
			if err != nil {
				return errors.Wrap(err, "failed to connect to pod "+pod.Name)
			}
			err = db.StopAllReplication()
			db.Close()
			if err != nil {
				return errors.Wrap(err, "stop replication on pod "+pod.Name)
			}
		}
	}

	sysUsersSecretObj := corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&sysUsersSecretObj,
	)
	if err != nil {
		return errors.Wrap(err, "get secrets")
	}

	for _, channels := range cr.Spec.PXC.ReplicationChannels {
		if channels.IsSource {
			continue
		}
		err = manageReplicationChannel(r.log, primaryDB, channels, !isReplica, string(sysUsersSecretObj.Data["replication"]))
		if err != nil {
			return errors.Wrap(err, "manage replication channel "+channels.Name)
		}
	}

	return nil
}

func manageReplicationChannel(log logr.Logger, primaryDB queries.Database, channel api.ReplicationChannel, stopped bool, replicaPW string) error {
	currentSources, err := primaryDB.ReplicationChannelSources(channel.Name)
	if err != nil && err != queries.ErrNotFound {
		return errors.Wrap(err, "get current replication channels")
	}

	if err == queries.ErrNotFound {
		for _, src := range channel.SourcesList {
			err := primaryDB.AddReplicationSource(channel.Name, src.Host, src.Port, src.Weight)
			if err != nil {
				return errors.Wrap(err, "add replication source "+channel.Name)
			}
		}
	}

	if !isSourcesChanged(channel.SourcesList, currentSources) {
		return nil
	}

	if !stopped && len(currentSources) > 0 {
		err = primaryDB.StopReplication(channel.Name)
		if err != nil {
			return errors.Wrap(err, "stop replication for channel")
		}
	}

	for _, src := range currentSources {
		err = primaryDB.DeleteReplicationSource(channel.Name, src.Host, src.Port)
		if err != nil {
			return errors.Wrap(err, "delete replication source")
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
			return errors.Wrap(err, "add replication source "+channel.Name)
		}
	}

	return primaryDB.StartReplication(replicaPW, queries.ReplicationChannelSource{
		Name: channel.Name,
		Host: maxWeightSrc.Host,
		Port: maxWeightSrc.Port,
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

	switch cr.Spec.PXC.Expose.Type {
	case corev1.ServiceTypeNodePort:
		svc.Spec.Type = corev1.ServiceTypeNodePort
		svc.Spec.ExternalTrafficPolicy = "Local"
	case corev1.ServiceTypeLoadBalancer:
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.ExternalTrafficPolicy = "Cluster"
	default:
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	}

	return svc
}
