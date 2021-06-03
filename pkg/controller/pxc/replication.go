package pxc

import (
	"context"
	"strconv"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcilePerconaXtraDBCluster) ensurePxcPodServices(cr *api.PerconaXtraDBCluster) error {
	for i := 0; i < int(cr.Spec.PXC.Size); i++ {
		svc := pxc.NewServicePXC(cr)

		svc.Name += "-" + strconv.Itoa(i)
		svc.Labels["app.kubernetes.io/component"] = "external-service"
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
		svc.Spec.ClusterIP = ""
		svc.Spec.Selector = map[string]string{"statefulset.kubernetes.io/pod-name": svc.Name}

		err := setControllerReference(cr, svc, r.scheme)
		if err != nil {
			return errors.Wrap(err, "failed to set owner to external service")
		}

		err = r.createOrUpdate(svc)
		if err != nil {
			return errors.Wrap(err, "failed to ensure pxc service")
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) removeOutdatedServices(cr *api.PerconaXtraDBCluster) error {
	//needed for labels
	svc := pxc.NewServicePXC(cr)

	svcNames := make(map[string]struct{}, cr.Spec.PXC.Size)
	for i := 0; i < int(cr.Spec.PXC.Size); i++ {
		svcNames[svc.Name+"-"+strconv.Itoa(i)] = struct{}{}
	}

	svc.Labels["app.kubernetes.io/component"] = "external-service"
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

func (r *ReconcilePerconaXtraDBCluster) removePxcPodServices(cr *api.PerconaXtraDBCluster) error {
	//needed for labels
	svc := pxc.NewServicePXC(cr)
	svc.Labels["app.kubernetes.io/component"] = "external-service"

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
