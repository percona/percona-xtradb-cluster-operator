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

	pxcStatus, err := r.appStatus(statefulset.NewNode(cr), cr.Spec.PXC.PodSpec, cr.Namespace, cr.CompareVersionWith("1.7.0") >= 0)
	if err != nil {
		return fmt.Errorf("get pxc status: %v", err)
	}
	pxcStatus.Version = cr.Status.PXC.Version
	pxcStatus.Image = cr.Status.PXC.Image
	if pxcStatus.Status != cr.Status.PXC.Status {
		if pxcStatus.Status == api.AppStateReady {
			clusterCondition = api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.ClusterPXCReady,
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
		}

		if pxcStatus.Status == api.AppStateError {
			clusterCondition = api.ClusterCondition{
				Status:             api.ConditionTrue,
				Message:            "PXC" + pxcStatus.Message,
				Reason:             "ErrorPXC",
				Type:               api.ClusterError,
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
		}
	}

	cr.Status.PXC = pxcStatus
	cr.Status.Host = cr.Name + "-" + "pxc." + cr.Namespace
	if cr.Status.PXC.Message != "" {
		cr.Status.Messages = append(cr.Status.Messages, "PXC: "+cr.Status.PXC.Message)
	}

	inProgres := false

	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		haProxyStatus, err := r.appStatus(statefulset.NewHAProxy(cr), cr.Spec.HAProxy, cr.Namespace, cr.CompareVersionWith("1.7.0") >= 0)
		if err != nil {
			return fmt.Errorf("get haproxy status: %v", err)
		}
		haProxyStatus.Version = cr.Status.HAProxy.Version

		if haProxyStatus.Status != cr.Status.HAProxy.Status {
			if haProxyStatus.Status == api.AppStateReady {
				clusterCondition = api.ClusterCondition{
					Status:             api.ConditionTrue,
					Type:               api.ClusterHAProxyReady,
					LastTransitionTime: metav1.NewTime(time.Now()),
				}
			}

			if haProxyStatus.Status == api.AppStateError {
				clusterCondition = api.ClusterCondition{
					Status:             api.ConditionTrue,
					Message:            "HAProxy:" + haProxyStatus.Message,
					Reason:             "ErrorHAProxy",
					Type:               api.ClusterError,
					LastTransitionTime: metav1.NewTime(time.Now()),
				}
			}
		}

		cr.Status.HAProxy = haProxyStatus

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
		inProgres, err = r.upgradeInProgress(cr, "haproxy")
		if err != nil {
			return fmt.Errorf("check haproxy upgrade progress: %v", err)
		}
	} else {
		cr.Status.HAProxy = api.AppStatus{}
	}

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		proxyStatus, err := r.appStatus(statefulset.NewProxy(cr), cr.Spec.ProxySQL, cr.Namespace, cr.CompareVersionWith("1.7.0") >= 0)
		if err != nil {
			return fmt.Errorf("get proxysql status: %v", err)
		}
		proxyStatus.Version = cr.Status.ProxySQL.Version

		if proxyStatus.Status != cr.Status.ProxySQL.Status {
			if proxyStatus.Status == api.AppStateReady {
				clusterCondition = api.ClusterCondition{
					Status:             api.ConditionTrue,
					Type:               api.ClusterProxyReady,
					LastTransitionTime: metav1.NewTime(time.Now()),
				}
			}

			if proxyStatus.Status == api.AppStateError {
				clusterCondition = api.ClusterCondition{
					Status:             api.ConditionTrue,
					Message:            "ProxySQL:" + proxyStatus.Message,
					Reason:             "ErrorProxySQL",
					Type:               api.ClusterError,
					LastTransitionTime: metav1.NewTime(time.Now()),
				}
			}
		}

		cr.Status.ProxySQL = proxyStatus

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
		inProgres, err = r.upgradeInProgress(cr, "proxysql")
		if err != nil {
			return fmt.Errorf("check proxysql upgrade progress: %v", err)
		}
	} else {
		cr.Status.ProxySQL = api.AppStatus{}
	}

	if !inProgres {
		inProgres, err = r.upgradeInProgress(cr, app.Name)
		if err != nil {
			return fmt.Errorf("check pxc upgrade progress: %v", err)
		}
	}

	switch {
	case (cr.Status.PXC.Status == cr.Status.ProxySQL.Status && cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled) ||
		(cr.Status.PXC.Status == cr.Status.HAProxy.Status && cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled):
		if cr.Status.PXC.Status == api.AppStateReady {
			clusterCondition = api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.ClusterReady,
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
		}
		cr.Status.Status = cr.Status.PXC.Status
	case (cr.Spec.ProxySQL == nil || !cr.Spec.ProxySQL.Enabled) &&
		(cr.Spec.HAProxy == nil || !cr.Spec.HAProxy.Enabled) &&
		cr.Status.PXC.Status == api.AppStateReady:
		clusterCondition = api.ClusterCondition{
			Status:             api.ConditionTrue,
			Type:               api.ClusterReady,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		cr.Status.Status = cr.Status.PXC.Status
	case cr.Status.PXC.Status == api.AppStateError ||
		cr.Status.ProxySQL.Status == api.AppStateError ||
		cr.Status.HAProxy.Status == api.AppStateError:
		clusterCondition = api.ClusterCondition{
			Status:             api.ConditionTrue,
			Type:               api.ClusterError,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		cr.Status.Status = api.AppStateError
	case cr.Status.PXC.Status == api.AppStateInit ||
		(cr.Spec.ProxySQL != nil && cr.Status.ProxySQL.Status == api.AppStateInit) ||
		(cr.Spec.HAProxy != nil && cr.Status.HAProxy.Status == api.AppStateInit):
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

		switch {
		case lastClusterCondition.Type != clusterCondition.Type:
			cr.Status.Conditions = append(cr.Status.Conditions, clusterCondition)
		default:
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

func (r *ReconcilePerconaXtraDBCluster) appStatus(app api.StatefulApp, podSpec *api.PodSpec, namespace string, cr170OrGreater bool) (api.AppStatus, error) {
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
	sfs := app.StatefulSet()
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: sfs.Name, Namespace: sfs.Namespace}, sfs)
	if err != nil {
		return api.AppStatus{}, fmt.Errorf("get statefulset: %v", err)
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
					if !isPXC(app) {
						status.Ready++
					} else {
						isPodWaitingForRecovery, _, err := r.isPodWaitingForRecovery(namespace, pod.Name)
						if err != nil {
							return api.AppStatus{}, fmt.Errorf("parse %s pod logs: %v", pod.Name, err)
						}

						isPodReady := !isPodWaitingForRecovery
						if cr170OrGreater {
							isPodReady = isPodReady && pod.ObjectMeta.Labels["controller-revision-hash"] == sfs.Status.UpdateRevision
						}

						if isPodReady {
							status.Ready++
						}
					}
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
