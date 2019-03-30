package perconaxtradbcluster

import (
	"context"
	"fmt"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func (r *ReconcilePerconaXtraDBCluster) updateStatus(cr *api.PerconaXtraDBCluster) (err error) {
	cr.Status.PXC, err = r.componentStatus(statefulset.NewNode(cr), cr.Spec.PXC, cr.Namespace)
	if err != nil {
		return fmt.Errorf("get pxc status: %v", err)
	}

	if cr.Status.PXC.Size == cr.Status.PXC.Ready {
		cr.Status.Status = api.ClusterStateReady
	}

	cr.Status.Host = cr.Name + "-" + "pxc"

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		cr.Status.ProxySQL, err = r.componentStatus(statefulset.NewProxy(cr), cr.Spec.ProxySQL, cr.Namespace)
		if err != nil {
			return fmt.Errorf("get proxysql status: %v", err)
		}
		if cr.Status.ProxySQL.Size != cr.Status.ProxySQL.Ready {
			cr.Status.Status = api.ClusterStateInit
		}

		cr.Status.Host = cr.Name + "-" + "proxysql"
	}

	err = r.client.Status().Update(context.TODO(), cr)
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

func (r *ReconcilePerconaXtraDBCluster) componentStatus(app api.App, podSpec *api.PodSpec, namespace string) (api.PodStatus, error) {
	list := corev1.PodList{}
	err := r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(app.Labels()),
		},
		&list,
	)
	if err != nil {
		return api.PodStatus{}, fmt.Errorf("get list: %v", err)
	}

	var status api.PodStatus
	status.Size = podSpec.Size

	for _, pod := range list.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == app.Labels()["app.kubernetes.io/component"] && cs.Ready {
				status.Ready++
			}
		}
	}

	return status, nil
}
