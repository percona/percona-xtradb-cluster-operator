package pxc

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/pkg/errors"
)

const maxStatusesQuantity = 20

func (r *ReconcilePerconaXtraDBCluster) updateStatus(cr *api.PerconaXtraDBCluster, reconcileErr error) (err error) {
	if reconcileErr != nil {
		if cr.Status.Status != api.AppStateError {
			clusterCondition := api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.ClusterError,
				Message:            reconcileErr.Error(),
				Reason:             "ErrorReconcile",
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
			cr.Status.Conditions = append(cr.Status.Conditions, clusterCondition)

			cr.Status.Messages = append(cr.Status.Messages, "Error: "+reconcileErr.Error())
			cr.Status.Status = api.AppStateError
		}

		return r.writeStatus(cr)
	}

	cr.Status.Messages = cr.Status.Messages[:0]

	pxc := statefulset.NewNode(cr)
	pxcStatus, pxcHost, err := r.componentStatus(pxc, cr, cr.Spec.PXC.PodSpec, cr.Status.PXC)
	if err != nil {
		return errors.Wrap(err, "get PXC status")
	}
	cr.Status.PXC = pxcStatus
	cr.Status.Host = pxcHost

	if pxcStatus.Message != "" {
		cr.Status.Messages = append(cr.Status.Messages, pxc.Name()+": "+pxcStatus.Message)
	}

	inProgress := false

	cr.Status.HAProxy = api.AppStatus{}
	if cr.HAProxyEnabled() {
		haproxy := statefulset.NewHAProxy(cr)

		haproxyStatus, haproxyHost, err := r.componentStatus(haproxy, cr, cr.Spec.HAProxy, cr.Status.HAProxy)
		if err != nil {
			return errors.Wrap(err, "update HAProxy status")
		}
		cr.Status.HAProxy = haproxyStatus
		cr.Status.Host = haproxyHost

		if haproxyStatus.Message != "" {
			cr.Status.Messages = append(cr.Status.Messages, haproxy.Name()+": "+haproxyStatus.Message)
		}

		inProgress, err = r.upgradeInProgress(cr, haproxy.Name())
		if err != nil {
			return errors.Wrap(err, "check haproxy upgrade progress")
		}
	}

	cr.Status.ProxySQL = api.AppStatus{}
	if cr.ProxySQLEnabled() {
		proxy := statefulset.NewProxy(cr)

		proxyStatus, proxyHost, err := r.componentStatus(proxy, cr, cr.Spec.ProxySQL, cr.Status.ProxySQL)
		if err != nil {
			return errors.Wrap(err, "update ProxySQL status")
		}
		cr.Status.ProxySQL = proxyStatus
		cr.Status.Host = proxyHost

		if proxyStatus.Message != "" {
			cr.Status.Messages = append(cr.Status.Messages, proxy.Name()+": "+proxyStatus.Message)
		}

		inProgress, err = r.upgradeInProgress(cr, proxy.Name())
		if err != nil {
			return errors.Wrap(err, "check proxysql upgrade progress")
		}
	}

	if !inProgress {
		inProgress, err = r.upgradeInProgress(cr, pxc.Name())
		if err != nil {
			return errors.Wrap(err, "check pxc upgrade progress")
		}
	}

	clusterStatus, clusterCondition := cr.Status.ClusterStatus()
	cr.Status.Status = clusterStatus

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

	if inProgress {
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
			return errors.Wrap(err, "send update")
		}
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) upgradeInProgress(cr *api.PerconaXtraDBCluster, appName string) (bool, error) {
	sfsObj := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name + "-" + appName, Namespace: cr.Namespace}, sfsObj)
	if err != nil {
		return false, err
	}
	return sfsObj.Status.Replicas > sfsObj.Status.UpdatedReplicas, nil
}

func (r *ReconcilePerconaXtraDBCluster) componentStatus(app api.StatefulApp, cr *api.PerconaXtraDBCluster, podSpec *api.PodSpec, crStatus api.AppStatus) (api.AppStatus, string, error) {
	status, err := r.appStatus(app, cr.Namespace, podSpec, cr.CompareVersionWith("1.7.0") >= 0)
	if err != nil {
		return api.AppStatus{}, "", errors.Wrap(err, "get app status")
	}
	status.Version = crStatus.Version
	status.Image = crStatus.Image

	host, err := r.appHost(app, cr.Namespace, podSpec)
	if err != nil {
		return api.AppStatus{}, host, errors.Wrap(err, "get app host")
	}

	return status, host, nil
}

func (r *ReconcilePerconaXtraDBCluster) appStatus(app api.StatefulApp, namespace string, podSpec *api.PodSpec, cr170OrGreater bool) (api.AppStatus, error) {
	list := corev1.PodList{}
	err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(app.Labels()),
		},
	)
	if err != nil {
		return api.AppStatus{}, errors.Wrap(err, "get pod list")
	}
	sfs := app.StatefulSet()
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: sfs.Name, Namespace: sfs.Namespace}, sfs)
	if err != nil {
		return api.AppStatus{}, errors.Wrap(err, "get statefulset")
	}

	status := api.AppStatus{
		Size:              podSpec.Size,
		Status:            api.AppStateInit,
		LabelSelectorPath: labels.SelectorFromSet(app.Labels()).String(),
	}

	for _, pod := range list.Items {
		for _, cntr := range pod.Status.ContainerStatuses {
			if cntr.State.Waiting != nil && cntr.State.Waiting.Message != "" {
				status.Message += cntr.Name + ": " + cntr.State.Waiting.Message + "; "
			}

			if cntr.Ready && !isPXC(app) {
				status.Ready++
			}
		}

		for _, cond := range pod.Status.Conditions {
			switch cond.Type {
			case corev1.ContainersReady:
				if cond.Status != corev1.ConditionTrue || !isPXC(app) {
					continue
				}

				if !cr170OrGreater {
					status.Ready++
					continue
				}

				isPodWaitingForRecovery, _, err := r.isPodWaitingForRecovery(namespace, pod.Name)
				if err != nil {
					return api.AppStatus{}, errors.Wrapf(err, "parse %s pod logs", pod.Name)
				}

				if !isPodWaitingForRecovery && pod.ObjectMeta.Labels["controller-revision-hash"] == sfs.Status.UpdateRevision {
					status.Ready++
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

func (r *ReconcilePerconaXtraDBCluster) appHost(app api.StatefulApp, namespace string, podSpec *api.PodSpec) (string, error) {
	if podSpec.ServiceType != corev1.ServiceTypeLoadBalancer {
		return app.Service() + "." + namespace, nil
	}

	svc := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: app.Service()}, svc)
	if err != nil {
		return "", errors.Wrapf(err, "get %s service", app.Name())
	}

	var host string

	for _, i := range svc.Status.LoadBalancer.Ingress {
		host = i.IP
		if len(i.Hostname) > 0 {
			host = i.Hostname
		}
	}

	return host, nil
}
