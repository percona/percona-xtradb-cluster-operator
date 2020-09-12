package pxc

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/pkg/errors"
)

const maxStatusesQuantity = 20

type clusterStatus struct {
	PXCStatus       api.AppState
	HAProxyStatus   api.AppState
	ProxySQLStatus  api.AppState
	HAProxyEnabled  bool
	ProxySQLEnabled bool
}

func (cs clusterStatus) ProxyEnabled() bool {
	return cs.HAProxyEnabled || cs.ProxySQLEnabled
}

func (cs clusterStatus) ProxyReady() bool {
	return (cs.HAProxyEnabled && cs.HAProxyStatus == api.AppStateReady) ||
		(cs.ProxySQLEnabled && cs.ProxySQLStatus == api.AppStateReady)
}

func (cs clusterStatus) ClusterInit() bool {
	return cs.PXCStatus == api.AppStateInit ||
		cs.HAProxyStatus == api.AppStateInit ||
		cs.ProxySQLStatus == api.AppStateInit
}

func (cs clusterStatus) ClusterError() bool {
	return cs.PXCStatus == api.AppStateError ||
		cs.HAProxyStatus == api.AppStateError ||
		cs.ProxySQLStatus == api.AppStateError
}

func (cs clusterStatus) PXCReady() bool {
	return cs.PXCStatus == api.AppStateReady
}

func (r *ReconcilePerconaXtraDBCluster) updatePXCStatus(cr *api.PerconaXtraDBCluster, clusterStatus *clusterStatus) error {
	pxcStatus, err := r.appStatus(statefulset.NewNode(cr), cr.Spec.PXC, cr.Namespace)
	if err != nil {
		return fmt.Errorf("get pxc status: %v", err)
	}
	pxcStatus.Version = cr.Status.PXC.Version
	pxcStatus.Image = cr.Status.PXC.Image

	cr.Status.PXC = pxcStatus
	clusterStatus.PXCStatus = pxcStatus.Status

	cr.Status.Host = cr.Name + "-" + "pxc." + cr.Namespace
	if cr.Status.PXC.Message != "" {
		cr.Status.Messages = append(cr.Status.Messages, "PXC: "+cr.Status.PXC.Message)
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateHAProxyStatus(cr *api.PerconaXtraDBCluster, inProgres *bool, clusterStatus *clusterStatus) error {
	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		clusterStatus.HAProxyEnabled = true
		haProxyStatus, err := r.appStatus(statefulset.NewHAProxy(cr), cr.Spec.HAProxy, cr.Namespace)
		if err != nil {
			return fmt.Errorf("get haproxy status: %v", err)
		}
		haProxyStatus.Version = cr.Status.HAProxy.Version
		cr.Status.HAProxy = haProxyStatus
		clusterStatus.HAProxyStatus = haProxyStatus.Status

		cr.Status.Host = cr.Name + "-" + "haproxy." + cr.Namespace
		if cr.Spec.HAProxy.ServiceType == corev1.ServiceTypeLoadBalancer {
			svc := &corev1.Service{}
			err := r.client.Get(context.TODO(),
				types.NamespacedName{
					Namespace: cr.Namespace,
					Name:      cr.Name + "-" + "haproxy",
				}, svc)
			if err != nil {
				return errors.Wrap(err, "get haproxy service")
			}
			for _, i := range svc.Status.LoadBalancer.Ingress {
				cr.Status.Host = i.IP
				if len(i.Hostname) > 0 {
					cr.Status.Host = i.Hostname
				}
			}
		}
		if cr.Status.HAProxy.Message != "" {
			cr.Status.Messages = append(cr.Status.Messages, "HAProxy: "+cr.Status.HAProxy.Message)
		}
		*inProgres, err = r.upgradeInProgress(cr, "haproxy")
		if err != nil {
			return fmt.Errorf("check haproxy upgrade progress: %v", err)
		}
	} else {
		cr.Status.HAProxy = api.AppStatus{}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateProxySQLStatus(cr *api.PerconaXtraDBCluster, inProgres *bool, clusterStatus *clusterStatus) error {
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		clusterStatus.ProxySQLEnabled = true
		proxyStatus, err := r.appStatus(statefulset.NewProxy(cr), cr.Spec.ProxySQL, cr.Namespace)
		if err != nil {
			return fmt.Errorf("get proxysql status: %v", err)
		}
		proxyStatus.Version = cr.Status.ProxySQL.Version
		cr.Status.ProxySQL = proxyStatus
		clusterStatus.ProxySQLStatus = proxyStatus.Status

		cr.Status.Host = cr.Name + "-" + "proxysql." + cr.Namespace
		if cr.Spec.ProxySQL.ServiceType == corev1.ServiceTypeLoadBalancer {
			svc := &corev1.Service{}
			err := r.client.Get(context.TODO(),
				types.NamespacedName{
					Namespace: cr.Namespace,
					Name:      cr.Name + "-" + "proxysql",
				}, svc)
			if err != nil {
				return errors.Wrap(err, "get proxysql service")
			}
			for _, i := range svc.Status.LoadBalancer.Ingress {
				cr.Status.Host = i.IP
				if len(i.Hostname) > 0 {
					cr.Status.Host = i.Hostname
				}
			}
		}

		if cr.Status.ProxySQL.Message != "" {
			cr.Status.Messages = append(cr.Status.Messages, "ProxySQL: "+cr.Status.ProxySQL.Message)
		}
		*inProgres, err = r.upgradeInProgress(cr, "proxysql")
		if err != nil {
			return fmt.Errorf("check proxysql upgrade progress: %v", err)
		}
	} else {
		cr.Status.ProxySQL = api.AppStatus{}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) updateStatus(cr *api.PerconaXtraDBCluster, reconcileErr error) (err error) {
	clusterCondition := api.ClusterCondition{
		Status:             api.ConditionTrue,
		Type:               api.ClusterInit,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}

	if reconcileErr != nil {
		if cr.Status.Status != api.ClusterError {
			clusterCondition = api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.ClusterError,
				Message:            reconcileErr.Error(),
				Reason:             "ErrorReconcile",
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
			cr.Status.Conditions = append(cr.Status.Conditions, clusterCondition)

			cr.Status.Messages = append(cr.Status.Messages, "Error: "+reconcileErr.Error())
			cr.Status.Status = api.ClusterError
		}

		return r.writeStatus(cr)
	}

	cr.Status.Messages = cr.Status.Messages[:0]
	clusterStatus := clusterStatus{}

	if err := r.updatePXCStatus(cr, &clusterStatus); err != nil {
		return err
	}

	inProgres := false

	if err := r.updateHAProxyStatus(cr, &inProgres, &clusterStatus); err != nil {
		return err
	}

	if err := r.updateProxySQLStatus(cr, &inProgres, &clusterStatus); err != nil {
		return err
	}

	if !inProgres {
		inProgres, err = r.upgradeInProgress(cr, app.Name)
		if err != nil {
			return fmt.Errorf("check pxc upgrade progress: %v", err)
		}
	}

	switch {
	case clusterStatus.ProxyReady() && clusterStatus.PXCReady():
		clusterCondition = api.ClusterCondition{
			Status:             api.ConditionTrue,
			Type:               api.ClusterReady,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		cr.Status.Status = cr.Status.PXC.Status
	case !clusterStatus.ProxyEnabled() && clusterStatus.PXCReady():
		clusterCondition = api.ClusterCondition{
			Status:             api.ConditionTrue,
			Type:               api.ClusterReady,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		cr.Status.Status = cr.Status.PXC.Status
	case clusterStatus.ClusterError():
		clusterCondition = api.ClusterCondition{
			Status:             api.ConditionTrue,
			Type:               api.ClusterError,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		cr.Status.Status = api.AppStateError
	case clusterStatus.ClusterInit():
		clusterCondition = api.ClusterCondition{
			Status:             api.ConditionTrue,
			Type:               api.ClusterInit,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		cr.Status.Status = api.AppStateInit
	default:
		cr.Status.Status = api.AppStateUnknown
	}

	if len(cr.Status.Conditions) == 0 {
		cr.Status.Conditions = append(cr.Status.Conditions, clusterCondition)
	} else {
		lastClusterCondition := cr.Status.Conditions[len(cr.Status.Conditions)-1]

		if lastClusterCondition.Type != clusterCondition.Type {
			cr.Status.Conditions = append(cr.Status.Conditions, clusterCondition)
		} else {
			cr.Status.Conditions[len(cr.Status.Conditions)-1] = lastClusterCondition
		}
	}

	if len(cr.Status.Conditions) > maxStatusesQuantity {
		cr.Status.Conditions = cr.Status.Conditions[len(cr.Status.Conditions)-maxStatusesQuantity:]
	}

	if inProgres {
		cr.Status.Status = api.AppStateInit
	}
	cr.Status.ObservedGeneration = cr.ObjectMeta.Generation
	return r.writeStatus(cr)
}

func (r *ReconcilePerconaXtraDBCluster) writeStatus(cr *api.PerconaXtraDBCluster) error {
	err := r.client.Status().Update(context.TODO(), cr)
	if err != nil {
		// may be it's k8s v1.10 and erlier (e.g. oc3.9) that doesn't support status updates
		// so try to update whole CR
		err := r.client.Update(context.TODO(), cr)
		if err != nil {
			return fmt.Errorf("send update: %v", err)
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) upgradeInProgress(cr *api.PerconaXtraDBCluster, appName string) (bool, error) {
	sfsObj := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name + "-" + app.Name, Namespace: cr.Namespace}, sfsObj)
	if err != nil {
		return false, err
	}
	return sfsObj.Status.Replicas > sfsObj.Status.UpdatedReplicas, nil
}

func (r *ReconcilePerconaXtraDBCluster) appStatus(app api.App, podSpec *api.PodSpec, namespace string) (api.AppStatus, error) {
	list := corev1.PodList{}
	err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(app.Labels()),
		},
	)
	if err != nil {
		return api.AppStatus{}, fmt.Errorf("get list: %v", err)
	}

	status := api.AppStatus{
		Size:   podSpec.Size,
		Status: api.AppStateInit,
	}

	for _, pod := range list.Items {
		for _, cond := range pod.Status.Conditions {
			switch cond.Type {
			case corev1.ContainersReady:
				if cond.Status == corev1.ConditionTrue {
					status.Ready++
				} else if cond.Status == corev1.ConditionFalse {
					for _, cntr := range pod.Status.ContainerStatuses {
						if cntr.State.Waiting != nil && cntr.State.Waiting.Message != "" {
							status.Message += cntr.Name + ": " + cntr.State.Waiting.Message + "; "
						}
					}
				}
			case corev1.PodScheduled:
				if cond.Reason == corev1.PodReasonUnschedulable &&
					cond.LastTransitionTime.Time.Before(time.Now().Add(-1*time.Minute)) {
					status.Status = api.AppStateError
					status.Message = cond.Message
				}
			}
		}
	}

	if status.Size == status.Ready {
		status.Status = api.AppStateReady
	}

	return status, nil
}
