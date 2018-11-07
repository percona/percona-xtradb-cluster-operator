package pxc

import (
	"context"
	"fmt"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

type PXC struct {
	serverVersion api.ServerVersion
}

func New(sv api.ServerVersion) *PXC {
	return &PXC{
		serverVersion: sv,
	}
}

func (h *PXC) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *api.PerconaXtraDBCluster:
		// Just ignore it for now.
		// All resources should be released by the k8s GC
		if event.Deleted {
			return nil
		}

		// TODO (ap): the status checking now is fake. Just a stub for further work
		if o.Status.State == api.ClusterStateInit {
			err := h.deploy(o)
			if err != nil {
				logrus.Errorf("cluster deploy error: %v", err)
				return err
			}
		}

		err := h.upgradePods(o.Spec.PXC, h.NewStatefulSet("node", o))
		if err != nil {
			logrus.Errorf("pxc upgrade error: %v", err)
		}

		if o.Spec.ProxySQL.Enabled {
			err = h.upgradePods(o.Spec.ProxySQL, h.NewStatefulSet("proxysql", o))
			if err != nil {
				logrus.Errorf("proxySQL upgrade error: %v", err)
			}
		}
	}

	return nil
}

func (h *PXC) deploy(cr *api.PerconaXtraDBCluster) error {
	nodeSet, err := h.newStatefulSetNode(cr)
	if err != nil {
		return err
	}
	err = sdk.Create(nodeSet)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create newStatefulSetNode: %v", err)
	}

	nodesService := h.newServiceNodes(cr)
	err = sdk.Create(nodesService)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create PXC Service: %v", err)
	}

	if cr.Spec.ProxySQL.Enabled {
		proxySet, err := h.newStatefulSetProxySQL(cr)
		if err != nil {
			return fmt.Errorf("failed to create ProxySQL Service: %v", err)
		}
		err = sdk.Create(proxySet)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create newStatefulSetProxySQL: %v", err)
		}

		err = sdk.Create(h.newServiceProxySQL(cr))
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create PXC Service: %v", err)
		}
	}

	return nil
}

// addOwnerRefToObject appends the desired OwnerReference to the object
func addOwnerRefToObject(obj metav1.Object, ownerRef metav1.OwnerReference) {
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ownerRef))
}

func asOwner(cr *api.PerconaXtraDBCluster) metav1.OwnerReference {
	trueVar := true

	return metav1.OwnerReference{
		APIVersion: api.SchemeGroupVersion.String(),
		Kind:       cr.Kind,
		Name:       cr.Name,
		UID:        cr.UID,
		Controller: &trueVar,
	}
}
