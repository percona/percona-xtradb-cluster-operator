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
				Type:               api.AppStateError,
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

	type sfsstatus struct {
		app    api.StatefulApp
		status *api.AppStatus
		spec   *api.PodSpec
	}

	apps := []sfsstatus{
		{
			app:    statefulset.NewNode(cr),
			status: &cr.Status.PXC,
			spec:   cr.Spec.PXC.PodSpec,
		},
	}

	cr.Status.HAProxy = api.AppStatus{}
	if cr.HAProxyEnabled() {
		apps = append(apps, sfsstatus{
			app:    statefulset.NewHAProxy(cr),
			status: &cr.Status.HAProxy,
			spec:   cr.Spec.HAProxy,
		})
	}

	cr.Status.ProxySQL = api.AppStatus{}
	if cr.ProxySQLEnabled() {
		apps = append(apps, sfsstatus{
			app:    statefulset.NewProxy(cr),
			status: &cr.Status.ProxySQL,
			spec:   cr.Spec.ProxySQL,
		})
	}

	inProgress := false

	for _, a := range apps {
		*a.status, cr.Status.Host, err = r.componentStatus(a.app, cr, a.spec, *a.status)
		if err != nil {
			return errors.Wrapf(err, "get %s status", a.app.Name())
		}

		if a.status.Message != "" {
			cr.Status.Messages = append(cr.Status.Messages, a.app.Name()+": "+a.status.Message)
		}

		if !inProgress {
			inProgress, err = r.upgradeInProgress(cr, a.app.Name())
			if err != nil {
				return errors.Wrapf(err, "check %s upgrade progress", a.app.Name())
			}
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
		}

		for _, cond := range pod.Status.Conditions {
			switch cond.Type {
			case corev1.ContainersReady:
				if cond.Status != corev1.ConditionTrue {
					continue
				}

				if !isPXC(app) {
					status.Ready++
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
