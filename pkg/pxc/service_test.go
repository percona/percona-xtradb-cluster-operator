package pxc

import (
	"testing"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewServiceProxySQL(t *testing.T) {
	lbClass := "custom-lb-class"

	tests := map[string]struct {
		crVersion                 string
		loadBalancerClass         *string
		loadBalancerType          corev1.ServiceType
		expectedLoadBalancerClass *string
	}{
		"version >= 1.18.0": {
			crVersion:                 "1.18.0",
			loadBalancerClass:         &lbClass,
			loadBalancerType:          "LoadBalancer",
			expectedLoadBalancerClass: &lbClass,
		},
		"version < 1.18.0 - lb class will be nil": {
			crVersion:         "1.17.0",
			loadBalancerClass: &lbClass,
			loadBalancerType:  "LoadBalancer",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cluster",
					Namespace: "my-namespace",
				},
				Spec: api.PerconaXtraDBClusterSpec{
					CRVersion: tt.crVersion,
					ProxySQL: &api.ProxySQLSpec{
						Expose: api.ServiceExpose{
							Type:              tt.loadBalancerType,
							LoadBalancerClass: tt.loadBalancerClass,
						},
					},
				},
			}

			svc := NewServiceProxySQL(cr)
			if svc.Spec.LoadBalancerClass != tt.expectedLoadBalancerClass {
				t.Fatalf("expected LoadBalancerClass to be %v, got %v", tt.expectedLoadBalancerClass, svc.Spec.LoadBalancerClass)
			}
		})
	}
}

func TestNewServiceHAProxy(t *testing.T) {
	lbClass := "custom-lb-class"
	lbClassTwo := "custom-lb-class-two"

	tests := map[string]struct {
		crVersion             string
		primaryServiceExpose  api.ServiceExpose
		replicasServiceExpose api.ServiceExpose
		expectedServiceSpec   corev1.ServiceSpec
	}{
		"version >= 1.18.0 - primary service expose configured - ensure that primary config is used": {
			crVersion: "1.18.0",
			primaryServiceExpose: api.ServiceExpose{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClass,
			},
			replicasServiceExpose: api.ServiceExpose{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClassTwo,
			},
			expectedServiceSpec: corev1.ServiceSpec{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClass,
			},
		},
		"version < 1.18.0 - primary service without lb class": {
			crVersion: "1.17.0",
			primaryServiceExpose: api.ServiceExpose{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClass,
			},
			expectedServiceSpec: corev1.ServiceSpec{
				Type: "LoadBalancer",
			},
		},
		"version >= 1.18.0 - primary service without lb class": {
			crVersion: "1.18.0",
			primaryServiceExpose: api.ServiceExpose{
				Type: "LoadBalancer",
			},
			expectedServiceSpec: corev1.ServiceSpec{
				Type: "LoadBalancer",
			},
		},
		"version < 1.18.0 - no lb expose configuration": {
			crVersion: "1.17.0",
			expectedServiceSpec: corev1.ServiceSpec{
				Type: "ClusterIP",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cluster",
					Namespace: "my-namespace",
				},
				Spec: api.PerconaXtraDBClusterSpec{
					CRVersion: tt.crVersion,
					HAProxy: &api.HAProxySpec{
						ExposePrimary: tt.primaryServiceExpose,
						ExposeReplicas: &api.ReplicasServiceExpose{
							ServiceExpose: tt.replicasServiceExpose,
						},
					},
				},
			}

			svc := NewServiceHAProxy(cr)
			if svc.Spec.Type != tt.expectedServiceSpec.Type {
				t.Fatalf("expected LoadBalancerClass to be %v, got %v", tt.expectedServiceSpec.Type, svc.Spec.Type)
			}
			if svc.Spec.LoadBalancerClass != tt.expectedServiceSpec.LoadBalancerClass {
				t.Fatalf("expected LoadBalancerClass to be %v, got %v", *tt.expectedServiceSpec.LoadBalancerClass, svc.Spec.LoadBalancerClass)
			}
		})
	}
}

func TestNewServiceHAProxyReplicas(t *testing.T) {
	lbClass := "custom-lb-class"
	lbClassTwo := "custom-lb-class-two"

	tests := map[string]struct {
		crVersion             string
		primaryServiceExpose  api.ServiceExpose
		replicasServiceExpose api.ServiceExpose
		expectedServiceSpec   corev1.ServiceSpec
	}{
		"version >= 1.18.0 - replica service expose configured - ensure replica config is used": {
			crVersion: "1.18.0",
			primaryServiceExpose: api.ServiceExpose{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClass,
			},
			replicasServiceExpose: api.ServiceExpose{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClassTwo,
			},
			expectedServiceSpec: corev1.ServiceSpec{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClassTwo,
			},
		},
		"version < 1.18.0 - replica service without lb class": {
			crVersion: "1.17.0",
			replicasServiceExpose: api.ServiceExpose{
				Type:              "LoadBalancer",
				LoadBalancerClass: &lbClass,
			},
			expectedServiceSpec: corev1.ServiceSpec{
				Type: "LoadBalancer",
			},
		},
		"version >= 1.18.0 - replica service without lb class": {
			crVersion: "1.18.0",
			replicasServiceExpose: api.ServiceExpose{
				Type: "LoadBalancer",
			},
			expectedServiceSpec: corev1.ServiceSpec{
				Type: "LoadBalancer",
			},
		},
		"version < 1.18.0 - no lb expose configuration": {
			crVersion: "1.17.0",
			expectedServiceSpec: corev1.ServiceSpec{
				Type: "ClusterIP",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cluster",
					Namespace: "my-namespace",
				},
				Spec: api.PerconaXtraDBClusterSpec{
					CRVersion: tt.crVersion,
					HAProxy: &api.HAProxySpec{
						ExposePrimary: tt.primaryServiceExpose,
						ExposeReplicas: &api.ReplicasServiceExpose{
							ServiceExpose: tt.replicasServiceExpose,
						},
					},
				},
			}

			svc := NewServiceHAProxyReplicas(cr)
			if svc.Spec.Type != tt.expectedServiceSpec.Type {
				t.Fatalf("expected LoadBalancerClass to be %v, got %v", tt.expectedServiceSpec.Type, svc.Spec.Type)
			}
			if svc.Spec.LoadBalancerClass != tt.expectedServiceSpec.LoadBalancerClass {
				t.Fatalf("expected LoadBalancerClass to be %v, got %v", *tt.expectedServiceSpec.LoadBalancerClass, svc.Spec.LoadBalancerClass)
			}
		})
	}
}
