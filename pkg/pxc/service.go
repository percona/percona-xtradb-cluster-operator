package pxc

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func (h *PXC) newServiceNodes(cr *api.PerconaXtraDBCluster) *corev1.Service {
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + appName + "-nodes",
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
			Labels: map[string]string{
				"app":     appName,
				"cluster": cr.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql",
				},
			},
			ClusterIP: "None",
			Selector: map[string]string{
				"component": cr.Name + "-" + appName + "-nodes",
			},
		},
	}
	addOwnerRefToObject(obj, cr.OwnerRef())
	return obj
}

func (h *PXC) newServiceProxySQL(cr *api.PerconaXtraDBCluster) *corev1.Service {
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + appName + "-proxysql",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":     appName,
				"cluster": cr.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:     3306,
					Name:     "mysql",
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 3306,
					},
				},
				{
					Port:     6032,
					Name:     "proxyadm",
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 6032,
					},
				},
			},
			Selector: map[string]string{
				"component": cr.Name + "-" + appName + "-proxysql",
			},
		},
	}
	addOwnerRefToObject(obj, cr.OwnerRef())
	return obj
}
