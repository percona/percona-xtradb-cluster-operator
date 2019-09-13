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
)

func (r *ReconcilePerconaXtraDBCluster) updateStatus(cr *api.PerconaXtraDBCluster, reconcileErr error) (err error) {
	if reconcileErr != nil {
		if cr.Status.Status != api.ClusterError {
			cr.Status.Conditions = append(cr.Status.Conditions, api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.ClusterError,
				Message:            reconcileErr.Error(),
				Reason:             "ErrorReconcile",
				LastTransitionTime: metav1.NewTime(time.Now()),
			})

			cr.Status.Messages = append(cr.Status.Messages, "Error: "+reconcileErr.Error())
			cr.Status.Status = api.ClusterError
		}

		return r.writeStatus(cr)
	}

	cr.Status.Messages = cr.Status.Messages[:0]

	pxcStatus, err := r.appStatus(statefulset.NewNode(cr), cr.Spec.PXC, cr.Namespace)
	if err != nil {
		return fmt.Errorf("get pxc status: %v", err)
	}
	if pxcStatus.Status != cr.Status.PXC.Status {
		if pxcStatus.Status == api.AppStateReady {
			cr.Status.Conditions = append(cr.Status.Conditions, api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.ClusterPXCReady,
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
		}

		if pxcStatus.Status == api.AppStateError {
			cr.Status.Conditions = append(cr.Status.Conditions, api.ClusterCondition{
				Status:             api.ConditionTrue,
				Message:            "PXC" + pxcStatus.Message,
				Reason:             "ErrorPXC",
				Type:               api.ClusterError,
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
		}
	}

	cr.Status.PXC = pxcStatus
	cr.Status.Host = cr.Name + "-" + "pxc"
	if cr.Status.PXC.Message != "" {
		cr.Status.Messages = append(cr.Status.Messages, "PXC: "+cr.Status.PXC.Message)
	}
	inProgres := false
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		proxyStatus, err := r.appStatus(statefulset.NewProxy(cr), cr.Spec.ProxySQL, cr.Namespace)
		if err != nil {
			return fmt.Errorf("get proxysql status: %v", err)
		}

		if proxyStatus.Status != cr.Status.ProxySQL.Status {
			if proxyStatus.Status == api.AppStateReady {
				cr.Status.Conditions = append(cr.Status.Conditions, api.ClusterCondition{
					Status:             api.ConditionTrue,
					Type:               api.ClusterProxyReady,
					LastTransitionTime: metav1.NewTime(time.Now()),
				})
			}

			if proxyStatus.Status == api.AppStateError {
				cr.Status.Conditions = append(cr.Status.Conditions, api.ClusterCondition{
					Status:             api.ConditionTrue,
					Message:            "ProxySQL:" + proxyStatus.Message,
					Reason:             "ErrorProxySQL",
					Type:               api.ClusterError,
					LastTransitionTime: metav1.NewTime(time.Now()),
				})
			}
		}

		cr.Status.ProxySQL = proxyStatus
		cr.Status.Host = cr.Name + "-" + "proxysql"
		if cr.Status.ProxySQL.Message != "" {
			cr.Status.Messages = append(cr.Status.Messages, "ProxySQL: "+cr.Status.ProxySQL.Message)
		}
		inProgres, err = r.upgradeInProgress(cr, "proxysql")
		if err != nil {
			return fmt.Errorf("check proxysql upgrade progress: %v", err)
		}
	}

	if !inProgres {
		inProgres, err = r.upgradeInProgress(cr, app.Name)
		if err != nil {
			return fmt.Errorf("check pxc upgrade progress: %v", err)
		}
	}

	switch {
	case cr.Status.PXC.Status == cr.Status.ProxySQL.Status:
		if cr.Status.Status != api.AppStateReady &&
			cr.Status.PXC.Status == api.AppStateReady {
			cr.Status.Conditions = append(cr.Status.Conditions, api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.ClusterReady,
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
		}
		cr.Status.Status = cr.Status.PXC.Status
	case cr.Status.PXC.Status == api.AppStateError || cr.Status.ProxySQL.Status == api.AppStateError:
		cr.Status.Status = api.AppStateError
	case cr.Status.PXC.Status == api.AppStateInit || cr.Status.ProxySQL.Status == api.AppStateInit:
		cr.Status.Status = api.AppStateInit
	default:
		cr.Status.Status = api.AppStateUnknown
	}

	if len(cr.Status.Conditions) == 0 {
		cr.Status.Conditions = append(cr.Status.Conditions, api.ClusterCondition{
			Status:             api.ConditionTrue,
			Type:               api.ClusterInit,
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
	}

	if inProgres {
		cr.Status.Status = api.AppStateInit
	}

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
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(app.Labels()),
		},
		&list,
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
