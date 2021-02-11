package pxc

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
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
				Name: "mysql-admin"},
		)
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
				Name: "mysql-admin"},
		)
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
			Name:      ProxySQLUnreadyServiceNamespacedName(cr).Name,
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
				Name: "mysql-admin"},
		)
	}

	return obj
}

func ProxySQLUnreadyServiceNamespacedName(cr *api.PerconaXtraDBCluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-proxysql-unready",
		Namespace: cr.Namespace,
	}
}

func NewServiceProxySQL(cr *api.PerconaXtraDBCluster) *corev1.Service {
	svcType := corev1.ServiceTypeClusterIP
	if cr.Spec.ProxySQL != nil && len(cr.Spec.ProxySQL.ServiceType) > 0 {
		svcType = cr.Spec.ProxySQL.ServiceType
	}
	serviceAnnotations := make(map[string]string)
	loadBalancerSourceRanges := []string{}
	if cr.Spec.ProxySQL != nil {
		serviceAnnotations = cr.Spec.ProxySQL.ServiceAnnotations
		loadBalancerSourceRanges = cr.Spec.ProxySQL.LoadBalancerSourceRanges
	}
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProxySQLServiceNamespacedName(cr).Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "percona-xtradb-cluster",
				"app.kubernetes.io/instance": cr.Name,
			},
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
		},
	}

	if svcType == corev1.ServiceTypeLoadBalancer || svcType == corev1.ServiceTypeNodePort {
		svcTrafficPolicyType := corev1.ServiceExternalTrafficPolicyTypeCluster
		if cr.Spec.ProxySQL != nil && len(cr.Spec.ProxySQL.ExternalTrafficPolicy) > 0 {
			svcTrafficPolicyType = cr.Spec.ProxySQL.ExternalTrafficPolicy
		}

		obj.Spec.ExternalTrafficPolicy = svcTrafficPolicyType
	}

	if cr.CompareVersionWith("1.6.0") >= 0 {
		obj.Spec.Ports = append(
			obj.Spec.Ports,
			corev1.ServicePort{
				Port: 33062,
				Name: "mysql-admin"},
		)
	}

	return obj
}

func ProxySQLServiceNamespacedName(cr *api.PerconaXtraDBCluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-proxysql",
		Namespace: cr.Namespace,
	}
}

func NewServiceHAProxy(cr *api.PerconaXtraDBCluster) *corev1.Service {
	svcType := corev1.ServiceTypeClusterIP
	if cr.Spec.HAProxy != nil && len(cr.Spec.HAProxy.ServiceType) > 0 {
		svcType = cr.Spec.HAProxy.ServiceType
	}
	serviceAnnotations := make(map[string]string)
	loadBalancerSourceRanges := []string{}
	if cr.Spec.HAProxy != nil {
		serviceAnnotations = cr.Spec.HAProxy.ServiceAnnotations
		loadBalancerSourceRanges = cr.Spec.HAProxy.LoadBalancerSourceRanges
	}
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      HaproxyServiceNamespacedName(cr).Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "percona-xtradb-cluster",
				"app.kubernetes.io/instance":   cr.Name,
				"app.kubernetes.io/component":  "haproxy",
				"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
				"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
			},
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
		},
	}

	if svcType == corev1.ServiceTypeLoadBalancer || svcType == corev1.ServiceTypeNodePort {
		svcTrafficPolicyType := corev1.ServiceExternalTrafficPolicyTypeCluster
		if cr.Spec.HAProxy != nil && len(cr.Spec.HAProxy.ExternalTrafficPolicy) > 0 {
			svcTrafficPolicyType = cr.Spec.HAProxy.ExternalTrafficPolicy
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

	return obj
}

func HaproxyServiceNamespacedName(cr *api.PerconaXtraDBCluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-haproxy",
		Namespace: cr.Namespace,
	}
}

func NewServiceHAProxyReplicas(cr *api.PerconaXtraDBCluster) *corev1.Service {
	svcType := corev1.ServiceTypeClusterIP
	if cr.Spec.HAProxy != nil && len(cr.Spec.HAProxy.ReplicasServiceType) > 0 {
		svcType = cr.Spec.HAProxy.ReplicasServiceType
	}
	serviceAnnotations := make(map[string]string)
	loadBalancerSourceRanges := []string{}
	if cr.Spec.HAProxy != nil {
		serviceAnnotations = cr.Spec.HAProxy.ServiceAnnotations
		loadBalancerSourceRanges = cr.Spec.HAProxy.LoadBalancerSourceRanges
	}
	obj := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      HAProxyReplicasNamespacedName(cr).Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "percona-xtradb-cluster",
				"app.kubernetes.io/instance":   cr.Name,
				"app.kubernetes.io/component":  "haproxy",
				"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
				"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
			},
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
		},
	}

	if svcType == corev1.ServiceTypeLoadBalancer || svcType == corev1.ServiceTypeNodePort {
		svcTrafficPolicyType := corev1.ServiceExternalTrafficPolicyTypeCluster
		if cr.Spec.HAProxy != nil && len(cr.Spec.HAProxy.ReplicasExternalTrafficPolicy) > 0 {
			svcTrafficPolicyType = cr.Spec.HAProxy.ReplicasExternalTrafficPolicy
		}

		obj.Spec.ExternalTrafficPolicy = svcTrafficPolicyType
	}

	return obj
}

func HAProxyReplicasNamespacedName(cr *api.PerconaXtraDBCluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-haproxy-replicas",
		Namespace: cr.Namespace,
	}
}
