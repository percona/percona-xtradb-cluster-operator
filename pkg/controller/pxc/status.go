package pxc

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/pkg/errors"
)

func (r *ReconcilePerconaXtraDBCluster) updateStatus(cr *api.PerconaXtraDBCluster, inProgress bool, reconcileErr error) (err error) {
	clusterCondition := api.ClusterCondition{
		Status:             api.ConditionTrue,
		Type:               api.AppStateInit,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}

	if reconcileErr != nil {
		if cr.Status.Status != api.AppStateError {
			clusterCondition := api.ClusterCondition{
				Status:             api.ConditionTrue,
				Type:               api.AppStateError,
				Message:            reconcileErr.Error(),
				Reason:             "ErrorReconcile",
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
			cr.Status.AddCondition(clusterCondition)

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

	// Maintaining the order of this slice is important!
	// PXC has to be the first object in the slice for cr.Status.Host to be correct.
	// HAProxy and ProxySQL are mutually exclusive and their order shouldn't be important.
	apps := []sfsstatus{
		{
			app:    statefulset.NewNode(cr),
			status: &cr.Status.PXC,
			spec:   cr.Spec.PXC.PodSpec,
		},
	}

	cr.Status.HAProxy = api.AppStatus{
		ComponentStatus: api.ComponentStatus{
			Version: cr.Status.HAProxy.Version,
		},
	}
	if cr.HAProxyEnabled() {
		apps = append(apps, sfsstatus{
			app:    statefulset.NewHAProxy(cr),
			status: &cr.Status.HAProxy,
			spec:   &cr.Spec.HAProxy.PodSpec,
		})
	}

	cr.Status.ProxySQL = api.AppStatus{
		ComponentStatus: api.ComponentStatus{
			Version: cr.Status.ProxySQL.Version,
		},
	}
	if cr.ProxySQLEnabled() {
		apps = append(apps, sfsstatus{
			app:    statefulset.NewProxy(cr),
			status: &cr.Status.ProxySQL,
			spec:   cr.Spec.ProxySQL,
		})
	}

	cr.Status.Size = 0
	cr.Status.Ready = 0
	for _, a := range apps {
		status, err := r.appStatus(a.app, cr.Namespace, a.spec, cr.CompareVersionWith("1.7.0") == -1, cr.Spec.Pause)
		if err != nil {
			return errors.Wrapf(err, "get %s status", a.app.Name())
		}
		status.Version = a.status.Version
		status.Image = a.status.Image
		// Ready count can be greater than total size in case of downscale
		if status.Ready > status.Size {
			status.Ready = status.Size
		}
		*a.status = status

		host, err := r.appHost(a.app, cr.Namespace, a.spec)
		if err != nil {
			return errors.Wrapf(err, "get %s host", a.app.Name())
		}
		cr.Status.Host = host

		if a.status.Message != "" {
			cr.Status.Messages = append(cr.Status.Messages, a.app.Name()+": "+a.status.Message)
		}

		cr.Status.Size += status.Size
		cr.Status.Ready += status.Ready

		if !inProgress {
			inProgress, err = r.upgradeInProgress(cr, a.app.Name())
			if err != nil {
				return errors.Wrapf(err, "check %s upgrade progress", a.app.Name())
			}
		}
	}

	cr.Status.Status = cr.Status.ClusterStatus(inProgress, cr.ObjectMeta.DeletionTimestamp != nil)
	clusterCondition.Type = cr.Status.Status
	cr.Status.AddCondition(clusterCondition)
	cr.Status.ObservedGeneration = cr.ObjectMeta.Generation

	return r.writeStatus(cr)
}

func (r *ReconcilePerconaXtraDBCluster) writeStatus(cr *api.PerconaXtraDBCluster) error {
	err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		c := &api.PerconaXtraDBCluster{}

		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, c)
		if err != nil {
			return err
		}

		c.Status = cr.Status

		return r.client.Status().Update(context.TODO(), c)
	})

	return errors.Wrap(err, "write status")
}

func (r *ReconcilePerconaXtraDBCluster) upgradeInProgress(cr *api.PerconaXtraDBCluster, appName string) (bool, error) {
	sfsObj := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name + "-" + appName, Namespace: cr.Namespace}, sfsObj)
	if err != nil {
		return false, err
	}
	return sfsObj.Status.Replicas > sfsObj.Status.UpdatedReplicas, nil
}

// appStatus counts the ready pods in statefulset (PXC, HAProxy, ProxySQL).
// If ready pods are equal to the size of the statefulset, we consider them ready.
// If a pod is in the unschedulable state for more than 1 min, we consider the statefulset in an error state.
// Otherwise, we consider the statefulset is initializing.
func (r *ReconcilePerconaXtraDBCluster) appStatus(app api.StatefulApp, namespace string, podSpec *api.PodSpec, crLt170, paused bool) (api.AppStatus, error) {
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
		Size: podSpec.Size,
		ComponentStatus: api.ComponentStatus{
			Status:            api.AppStateInit,
			LabelSelectorPath: labels.SelectorFromSet(app.Labels()).String(),
		},
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

				if !isPXC(app) || crLt170 {
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
					status.Message = cond.Message
				}
			}
		}
	}

	switch {
	case paused && status.Ready > 0:
		status.Status = api.AppStateStopping
	case paused:
		status.Status = api.AppStatePaused
	case status.Size == status.Ready:
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
