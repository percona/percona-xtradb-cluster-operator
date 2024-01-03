package pxc

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

const (
	// HeadlessServiceAnnotation is the annotation key for headless service
	HeadlessServiceAnnotation = "percona.com/headless-service"
)

func NewServicePXC(cr *api.PerconaXtraDBCluster) *corev1.Service {
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + appName,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "percona-xtradb-cluster",
				"app.kubernetes.io/instance": cr.Name,
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
				"app.kubernetes.io/name":      "percona-xtradb-cluster",
				"app.kubernetes.io/instance":  cr.Name,
				"app.kubernetes.io/component": appName,
			},
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
		obj.ObjectMeta.Labels["app.kubernetes.io/component"] = appName
		obj.ObjectMeta.Labels["app.kubernetes.io/managed-by"] = "percona-xtradb-cluster-operator"
		obj.ObjectMeta.Labels["app.kubernetes.io/part-of"] = "percona-xtradb-cluster"

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
			Labels: map[string]string{
				"app.kubernetes.io/name":     "percona-xtradb-cluster",
				"app.kubernetes.io/instance": cr.Name,
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
				"app.kubernetes.io/name":      "percona-xtradb-cluster",
				"app.kubernetes.io/instance":  cr.Name,
				"app.kubernetes.io/component": appName,
			},
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
		obj.ObjectMeta.Labels["app.kubernetes.io/component"] = appName
		obj.ObjectMeta.Labels["app.kubernetes.io/managed-by"] = "percona-xtradb-cluster-operator"
		obj.ObjectMeta.Labels["app.kubernetes.io/part-of"] = "percona-xtradb-cluster"

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
			Labels: map[string]string{
				"app.kubernetes.io/name":     "percona-xtradb-cluster",
				"app.kubernetes.io/instance": cr.Name,
			},
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
			Selector: map[string]string{
				"app.kubernetes.io/name":      "percona-xtradb-cluster",
				"app.kubernetes.io/instance":  cr.Name,
				"app.kubernetes.io/component": "proxysql",
			},
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
		obj.ObjectMeta.Labels["app.kubernetes.io/component"] = "proxysql"
		obj.ObjectMeta.Labels["app.kubernetes.io/managed-by"] = "percona-xtradb-cluster-operator"
		obj.ObjectMeta.Labels["app.kubernetes.io/part-of"] = "percona-xtradb-cluster"
	}

	if cr.CompareVersionWith("1.10.0") >= 0 {
		obj.Spec.PublishNotReadyAddresses = true
		delete(obj.ObjectMeta.Annotations, "service.alpha.kubernetes.io/tolerate-unready-endpoints")
	}

	return obj
}

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
	serviceLabels := map[string]string{
		"app.kubernetes.io/name":     "percona-xtradb-cluster",
		"app.kubernetes.io/instance": cr.Name,
	}
	loadBalancerSourceRanges := []string{}
	loadBalancerIP := ""

	if cr.Spec.ProxySQL != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 {
			serviceAnnotations = cr.Spec.ProxySQL.Expose.Annotations
			serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.ProxySQL.Expose.Labels)
			loadBalancerSourceRanges = cr.Spec.ProxySQL.Expose.LoadBalancerSourceRanges
			loadBalancerIP = cr.Spec.ProxySQL.Expose.LoadBalancerIP
		} else {
			serviceAnnotations = cr.Spec.ProxySQL.ServiceAnnotations
			serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.ProxySQL.ServiceLabels)
			loadBalancerSourceRanges = cr.Spec.ProxySQL.LoadBalancerSourceRanges
			loadBalancerIP = cr.Spec.ProxySQL.LoadBalancerIP
		}
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
			},
			Selector: map[string]string{
				"app.kubernetes.io/name":      "percona-xtradb-cluster",
				"app.kubernetes.io/instance":  cr.Name,
				"app.kubernetes.io/component": "proxysql",
			},
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
			if cr.Spec.ProxySQL.Expose.Annotations[HeadlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[HeadlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
			}
		} else if cr.Spec.ProxySQL.ServiceAnnotations != nil {
			if cr.Spec.ProxySQL.ServiceAnnotations[HeadlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[HeadlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
			}
		}
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
		obj.ObjectMeta.Labels["app.kubernetes.io/component"] = "proxysql"
		obj.ObjectMeta.Labels["app.kubernetes.io/managed-by"] = "percona-xtradb-cluster-operator"
		obj.ObjectMeta.Labels["app.kubernetes.io/part-of"] = "percona-xtradb-cluster"
	}

	return obj
}

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
	serviceLabels := map[string]string{
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/component":  "haproxy",
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}
	loadBalancerSourceRanges := []string{}
	loadBalancerIP := ""

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 {
			serviceAnnotations = cr.Spec.HAProxy.ExposePrimary.Annotations
			serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.HAProxy.ExposePrimary.Labels)
			loadBalancerSourceRanges = cr.Spec.HAProxy.ExposePrimary.LoadBalancerSourceRanges
			loadBalancerIP = cr.Spec.HAProxy.ExposePrimary.LoadBalancerIP
		} else {
			serviceAnnotations = cr.Spec.HAProxy.ServiceAnnotations
			serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.HAProxy.PodSpec.ServiceLabels)
			loadBalancerSourceRanges = cr.Spec.HAProxy.LoadBalancerSourceRanges
			loadBalancerIP = cr.Spec.HAProxy.LoadBalancerIP
		}
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
			},
			Selector: map[string]string{
				"app.kubernetes.io/name":      "percona-xtradb-cluster",
				"app.kubernetes.io/instance":  cr.Name,
				"app.kubernetes.io/component": "haproxy",
			},
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

	if cr.CompareVersionWith("1.6.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port:       33062,
				TargetPort: intstr.FromInt(33062),
				Name:       "mysql-admin",
			},
		)
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port:       33060,
				TargetPort: intstr.FromInt(33060),
				Name:       "mysqlx",
			},
		)
	}

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 {
			if cr.Spec.HAProxy.ExposePrimary.Annotations != nil {
				if cr.Spec.HAProxy.ExposePrimary.Annotations[HeadlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
					obj.Annotations[HeadlessServiceAnnotation] = "true"
					obj.Spec.ClusterIP = corev1.ClusterIPNone
				}
			}
		} else {
			if cr.Spec.HAProxy.ServiceAnnotations != nil {
				if cr.Spec.HAProxy.ServiceAnnotations[HeadlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
					obj.Annotations[HeadlessServiceAnnotation] = "true"
					obj.Spec.ClusterIP = corev1.ClusterIPNone
				}
			}
		}
	}

	return obj
}

func NewServiceHAProxyReplicas(cr *api.PerconaXtraDBCluster) *corev1.Service {
	if cr.CompareVersionWith("1.14.0") >= 0 && cr.Spec.HAProxy != nil {
		if cr.Spec.HAProxy.ExposeReplicas == nil {
			cr.Spec.HAProxy.ExposeReplicas = &api.ServiceExpose{
				Enabled: true,
			}
		}
	}

	svcType := corev1.ServiceTypeClusterIP
	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposeReplicas.Type) > 0 {
			svcType = cr.Spec.HAProxy.ExposeReplicas.Type
		} else if len(cr.Spec.HAProxy.ReplicasServiceType) > 0 {
			svcType = cr.Spec.HAProxy.ReplicasServiceType
		}
	}

	serviceAnnotations := make(map[string]string)
	serviceLabels := map[string]string{
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/component":  "haproxy",
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}
	loadBalancerSourceRanges := []string{}
	loadBalancerIP := ""
	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 {
			serviceAnnotations = cr.Spec.HAProxy.ExposeReplicas.Annotations
			serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.HAProxy.ExposeReplicas.Labels)
		} else if cr.CompareVersionWith("1.12.0") >= 0 {
			serviceAnnotations = cr.Spec.HAProxy.ReplicasServiceAnnotations
			serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.HAProxy.PodSpec.ReplicasServiceLabels)
		} else {
			serviceAnnotations = cr.Spec.HAProxy.ServiceAnnotations
			serviceLabels = fillServiceLabels(serviceLabels, cr.Spec.HAProxy.PodSpec.ServiceLabels)
		}

		if cr.CompareVersionWith("1.14.0") >= 0 {
			if cr.Spec.HAProxy.ExposeReplicas.LoadBalancerSourceRanges != nil {
				loadBalancerSourceRanges = cr.Spec.HAProxy.ExposeReplicas.LoadBalancerSourceRanges
			} else {
				loadBalancerSourceRanges = cr.Spec.HAProxy.ExposePrimary.LoadBalancerSourceRanges
			}
			loadBalancerIP = cr.Spec.HAProxy.ExposeReplicas.LoadBalancerIP
		} else {
			if cr.Spec.HAProxy.ReplicasLoadBalancerSourceRanges != nil {
				loadBalancerSourceRanges = cr.Spec.HAProxy.ReplicasLoadBalancerSourceRanges
			} else {
				loadBalancerSourceRanges = cr.Spec.HAProxy.LoadBalancerSourceRanges
			}
			loadBalancerIP = cr.Spec.HAProxy.ReplicasLoadBalancerIP
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
			Selector: map[string]string{
				"app.kubernetes.io/name":      "percona-xtradb-cluster",
				"app.kubernetes.io/instance":  cr.Name,
				"app.kubernetes.io/component": "haproxy",
			},
			LoadBalancerSourceRanges: loadBalancerSourceRanges,
			LoadBalancerIP:           loadBalancerIP,
		},
	}

	if svcType == corev1.ServiceTypeLoadBalancer || svcType == corev1.ServiceTypeNodePort {
		svcTrafficPolicyType := corev1.ServiceExternalTrafficPolicyTypeCluster

		if cr.Spec.HAProxy != nil {
			if cr.CompareVersionWith("1.14.0") >= 0 && len(cr.Spec.HAProxy.ExposeReplicas.ExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.HAProxy.ExposeReplicas.ExternalTrafficPolicy
			} else if len(cr.Spec.HAProxy.ReplicasExternalTrafficPolicy) > 0 {
				svcTrafficPolicyType = cr.Spec.HAProxy.ReplicasExternalTrafficPolicy
			}
		}

		obj.Spec.ExternalTrafficPolicy = svcTrafficPolicyType
	}

	if cr.Spec.HAProxy != nil {
		if cr.CompareVersionWith("1.14.0") >= 0 && cr.Spec.HAProxy.ExposeReplicas.Annotations != nil {
			if cr.Spec.HAProxy.ExposeReplicas.Annotations[HeadlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[HeadlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
			}
		} else if cr.Spec.HAProxy.ReplicasServiceAnnotations != nil {
			if cr.Spec.HAProxy.ReplicasServiceAnnotations[HeadlessServiceAnnotation] == "true" && svcType == corev1.ServiceTypeClusterIP {
				obj.Annotations[HeadlessServiceAnnotation] = "true"
				obj.Spec.ClusterIP = corev1.ClusterIPNone
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
