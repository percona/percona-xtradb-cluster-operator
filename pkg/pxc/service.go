package pxc

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

const (
	// headlessServiceAnnotation is the annotation key for headless service
	headlessServiceAnnotation = "percona.com/headless-service"
)

// NewServicePXC creates a headless service for pxc pods.
func NewServicePXC(cr *api.PerconaXtraDBCluster) *corev1.Service {
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + appName,
			Namespace: cr.Namespace,
			Labels:    naming.LabelsPXC(cr),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql",
				},
			},
			ClusterIP: "None",
			Selector:  naming.SelectorPXC(cr),
		},
	}

	if cr.CompareVersionWith("1.6.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port: 33062,
				Name: "mysql-admin",
			},
		)
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port: 33060,
				Name: "mysqlx",
			},
		)
	}

	if cr.CompareVersionWith("1.14.0") >= 0 {
		if cr.Spec.PXC != nil {
			obj.Annotations = cr.Spec.PXC.Expose.Annotations
			obj.Labels = fillServiceLabels(obj.Labels, cr.Spec.PXC.Expose.Labels)
		}
	}

	return obj
}

// NewServicePXCUnready creates a headless service with a "tolerate-unready-endpoints"
// annotation to allow unready pods to still be included in the DNS resolution.
func NewServicePXCUnready(cr *api.PerconaXtraDBCluster) *corev1.Service {
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + appName + "-unready",
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
			Labels: naming.LabelsPXC(cr),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql",
				},
			},
			ClusterIP: "None",
			Selector:  naming.SelectorPXC(cr),
		},
	}

	if cr.CompareVersionWith("1.6.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port: 33062,
				Name: "mysql-admin",
			},
		)
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port: 33060,
				Name: "mysqlx",
			},
		)
	}

	if cr.CompareVersionWith("1.10.0") >= 0 {
		obj.Spec.PublishNotReadyAddresses = true
		delete(obj.ObjectMeta.Annotations, "service.alpha.kubernetes.io/tolerate-unready-endpoints")
	}

	if cr.CompareVersionWith("1.14.0") >= 0 {
		if cr.Spec.PXC != nil {
			obj.Annotations = cr.Spec.PXC.Expose.Annotations
			obj.Labels = fillServiceLabels(obj.Labels, cr.Spec.PXC.Expose.Labels)
		}
	}

	return obj
}

// NewServiceProxySQLUnready creates a headless service with a "tolerate-unready-endpoints"
// annotation to allow unready pods to still be included in the DNS resolution.
func NewServiceProxySQLUnready(cr *api.PerconaXtraDBCluster) *corev1.Service {
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.ProxySQLUnreadyServiceNamespacedName().Name,
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
			Labels: naming.LabelsProxySQL(cr),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql",
				},
				{
					Port: 6032,
					Name: "proxyadm",
				},
			},
			ClusterIP: "None",
			Selector:  naming.SelectorProxySQL(cr),
		},
	}

	if cr.CompareVersionWith("1.6.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port: 33062,
				Name: "mysql-admin",
			},
		)
	}

	if cr.CompareVersionWith("1.10.0") >= 0 {
		obj.Spec.PublishNotReadyAddresses = true
		delete(obj.ObjectMeta.Annotations, "service.alpha.kubernetes.io/tolerate-unready-endpoints")
	}

	return obj
}

// NewServiceProxySQL creates the proxysql service.
func NewServiceProxySQL(cr *api.PerconaXtraDBCluster) *corev1.Service {
	svcType := corev1.ServiceTypeClusterIP

	if cr.Spec.ProxySQL != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.ProxySQL.Expose.Type) > 0 {
			svcType = cr.Spec.ProxySQL.Expose.Type
		} else if len(cr.Spec.ProxySQL.ServiceType) > 0 {
			svcType = cr.Spec.ProxySQL.ServiceType
		}
	}

	serviceAnnotations := make(map[string]string)
	serviceLabels := naming.LabelsProxySQL(cr)
	loadBalancerSourceRanges := []string{}
	loadBalancerIP := ""

	if cr.Spec.ProxySQL != nil {

		serviceAnnotations = cr.Spec.ProxySQL.Expose.Annotations
		serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.ProxySQL.Expose.Labels)
		loadBalancerSourceRanges = cr.Spec.ProxySQL.Expose.LoadBalancerSourceRanges
		loadBalancerIP = cr.Spec.ProxySQL.Expose.LoadBalancerIP

	}

	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.ProxySQLServiceNamespacedName().Name,
			Namespace:   cr.Namespace,
			Labels:      serviceLabels,
			Annotations: serviceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type: svcType,
			Ports: []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql",
				},
				{
					Port: 33062,
					Name: "mysql-admin",
				},
			},
			Selector:                 naming.SelectorProxySQL(cr),
			LoadBalancerSourceRanges: loadBalancerSourceRanges,
			LoadBalancerIP:           loadBalancerIP,
		},
	}

	if svcType == corev1.ServiceTypeLoadBalancer || svcType == corev1.ServiceTypeNodePort {
		svcTrafficPolicyType := corev1.ServiceExternalTrafficPolicyTypeCluster

		if cr.Spec.ProxySQL != nil {
			if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.ProxySQL.Expose.ExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.ProxySQL.Expose.ExternalTrafficPolicy
			} else if len(cr.Spec.ProxySQL.ExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.ProxySQL.ExternalTrafficPolicy
			}
		}

		obj.Spec.ExternalTrafficPolicy = svcTrafficPolicyType
	}

	if cr.Spec.ProxySQL != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 && cr.Spec.ProxySQL.Expose.Annotations != nil {
			if cr.Spec.ProxySQL.Expose.Annotations[headlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[headlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
			}
		} else if cr.Spec.ProxySQL.ServiceAnnotations != nil {
			if cr.Spec.ProxySQL.ServiceAnnotations[headlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[headlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
			}
		}
	}

	if cr.CompareVersionWith("1.17.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port: 6070,
				Name: "stats",
			},
		)
	}

	if cr.Spec.ProxySQL != nil {
		if cr.CompareVersionWith("1.18.0") >= 0 {
			loadBalancerClass, err := cr.Spec.ProxySQL.Expose.GetLoadBalancerClass()
			if err == nil {
				obj.Spec.LoadBalancerClass = loadBalancerClass
			}
		}
	}

	return obj
}

// NewServiceHAProxy creates the haproxy service using the primary expose configuration.
func NewServiceHAProxy(cr *api.PerconaXtraDBCluster) *corev1.Service {
	svcType := corev1.ServiceTypeClusterIP

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposePrimary.Type) > 0 {
			svcType = cr.Spec.HAProxy.ExposePrimary.Type
		} else if len(cr.Spec.HAProxy.ServiceType) > 0 {
			svcType = cr.Spec.HAProxy.ServiceType
		}
	}

	serviceAnnotations := make(map[string]string)
	serviceLabels := naming.LabelsHAProxy(cr)
	loadBalancerSourceRanges := []string{}
	loadBalancerIP := ""

	if cr.Spec.HAProxy != nil {
		serviceAnnotations = cr.Spec.HAProxy.ExposePrimary.Annotations
		serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.HAProxy.ExposePrimary.Labels)
		loadBalancerSourceRanges = cr.Spec.HAProxy.ExposePrimary.LoadBalancerSourceRanges
		loadBalancerIP = cr.Spec.HAProxy.ExposePrimary.LoadBalancerIP

	}

	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.HaproxyServiceNamespacedName().Name,
			Namespace:   cr.Namespace,
			Labels:      serviceLabels,
			Annotations: serviceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type: svcType,
			Ports: []corev1.ServicePort{
				{
					Port:       3306,
					TargetPort: intstr.FromInt(3306),
					Name:       "mysql",
				},
				{
					Port:       3309,
					TargetPort: intstr.FromInt(3309),
					Name:       "proxy-protocol",
				},
				{
					Port:       33062,
					TargetPort: intstr.FromInt(33062),
					Name:       "mysql-admin",
				},
				{
					Port:       33060,
					TargetPort: intstr.FromInt(33060),
					Name:       "mysqlx",
				},
			},
			Selector:                 naming.SelectorHAProxy(cr),
			LoadBalancerSourceRanges: loadBalancerSourceRanges,
			LoadBalancerIP:           loadBalancerIP,
		},
	}

	if svcType == corev1.ServiceTypeLoadBalancer || svcType == corev1.ServiceTypeNodePort {
		svcTrafficPolicyType := corev1.ServiceExternalTrafficPolicyTypeCluster

		if cr.Spec.HAProxy != nil {
			if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposePrimary.ExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.HAProxy.ExposePrimary.ExternalTrafficPolicy
			} else if len(cr.Spec.HAProxy.ExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.HAProxy.ExternalTrafficPolicy
			}
		}

		obj.Spec.ExternalTrafficPolicy = svcTrafficPolicyType
	}

	if cr.CompareVersionWith("1.17.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port:       8404,
				TargetPort: intstr.FromInt(8404),
				Name:       "stats",
			},
		)
	}

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 {
			if cr.Spec.HAProxy.ExposePrimary.Annotations != nil {
				if cr.Spec.HAProxy.ExposePrimary.Annotations[headlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
					obj.Annotations[headlessServiceAnnotation] = "true"
					obj.Spec.ClusterIP = corev1.ClusterIPNone
				}
			}
		} else {
			if cr.Spec.HAProxy.ServiceAnnotations != nil {
				if cr.Spec.HAProxy.ServiceAnnotations[headlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
					obj.Annotations[headlessServiceAnnotation] = "true"
					obj.Spec.ClusterIP = corev1.ClusterIPNone
				}
			}
		}
	}

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.18.0") >= 0 {
			loadBalancerClass, err := cr.Spec.HAProxy.ExposePrimary.GetLoadBalancerClass()
			if err == nil {
				obj.Spec.LoadBalancerClass = loadBalancerClass
			}
		}
	}

	return obj
}

// NewServiceHAProxyReplicas creates the haproxy service using the replicas expose configuration.
func NewServiceHAProxyReplicas(cr *api.PerconaXtraDBCluster) *corev1.Service {
	if cr.CompareVersionWith("1.14.0") >= 0 && cr.Spec.HAProxy != nil {
		if cr.Spec.HAProxy.ExposeReplicas == nil {
			cr.Spec.HAProxy.ExposeReplicas = &api.ReplicasServiceExpose{
				ServiceExpose: api.ServiceExpose{
					Enabled: true,
				},
			}
		}
	}

	svcType := corev1.ServiceTypeClusterIP
	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Type) > 0 {
			svcType = cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Type
		} else if len(cr.Spec.HAProxy.ReplicasServiceType) > 0 {
			svcType = cr.Spec.HAProxy.ReplicasServiceType
		}
	}

	serviceAnnotations := make(map[string]string)
	serviceLabels := naming.LabelsHAProxy(cr)
	loadBalancerSourceRanges := []string{}
	loadBalancerIP := ""
	if cr.Spec.HAProxy != nil {

		serviceAnnotations = cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Annotations
		serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Labels)
		loadBalancerIP = cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.LoadBalancerIP

		if cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.LoadBalancerSourceRanges != nil {
			loadBalancerSourceRanges = cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.LoadBalancerSourceRanges
		} else if cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.LoadBalancerSourceRanges == nil && cr.Spec.HAProxy.ExposeReplicas.Type == corev1.ServiceTypeLoadBalancer {
			loadBalancerSourceRanges = cr.Spec.HAProxy.ExposePrimary.LoadBalancerSourceRanges
		}

	}

	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.HAProxyReplicasNamespacedName().Name,
			Namespace:   cr.Namespace,
			Labels:      serviceLabels,
			Annotations: serviceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type: svcType,
			Ports: []corev1.ServicePort{
				{
					Port:       3306,
					TargetPort: intstr.FromInt(3307),
					Name:       "mysql-replicas",
				},
			},
			Selector:                 naming.SelectorHAProxy(cr),
			LoadBalancerSourceRanges: loadBalancerSourceRanges,
			LoadBalancerIP:           loadBalancerIP,
		},
	}

	if svcType == corev1.ServiceTypeLoadBalancer || svcType == corev1.ServiceTypeNodePort {
		svcTrafficPolicyType := corev1.ServiceExternalTrafficPolicyTypeCluster

		if cr.Spec.HAProxy != nil {
			if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.ExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.ExternalTrafficPolicy
			} else if len(cr.Spec.HAProxy.ReplicasExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.HAProxy.ReplicasExternalTrafficPolicy
			}
		}

		obj.Spec.ExternalTrafficPolicy = svcTrafficPolicyType
	}

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 && cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Annotations != nil {
			if cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Annotations[headlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[headlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
			}
		} else if cr.Spec.HAProxy.ReplicasServiceAnnotations != nil {
			if cr.Spec.HAProxy.ReplicasServiceAnnotations[headlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[headlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
			}
		}
	}

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.18.0") >= 0 {
			loadBalancerClass, err := cr.Spec.HAProxy.ExposeReplicas.GetLoadBalancerClass()
			if err == nil {
				obj.Spec.LoadBalancerClass = loadBalancerClass
			}
		}
	}

	return obj
}

func fillServiceLabels(labels map[string]string, serviceLabels map[string]string) map[string]string {
	for k, v := range serviceLabels {
		if _, ok := labels[k]; ok {
			continue
		}
		labels[k] = v
	}
	return labels
}
