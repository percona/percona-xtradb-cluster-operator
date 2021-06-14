package pxc

import (
	"context"
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
