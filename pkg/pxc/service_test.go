package pxc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
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
			assert.Equal(t, tt.expectedLoadBalancerClass, svc.Spec.LoadBalancerClass)
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
			assert.Equal(t, tt.expectedServiceSpec.Type, svc.Spec.Type)
			assert.Equal(t, tt.expectedServiceSpec.LoadBalancerClass, svc.Spec.LoadBalancerClass)
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
			assert.Equal(t, tt.expectedServiceSpec.Type, svc.Spec.Type)
			assert.Equal(t, tt.expectedServiceSpec.LoadBalancerClass, svc.Spec.LoadBalancerClass)
		})
	}
}

func TestNewServiceInternalTrafficPolicy(t *testing.T) {
	tests := map[string]struct {
		serviceName    string
		serviceType    corev1.ServiceType
		configureCR    func(cr *api.PerconaXtraDBCluster)
		buildService   func(cr *api.PerconaXtraDBCluster) *corev1.Service
		expectedPolicy corev1.ServiceInternalTrafficPolicy
	}{
		"ProxySQL defaults to Cluster": {
			serviceName: "ProxySQL",
			configureCR: func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.ProxySQL = &api.ProxySQLSpec{}
			},
			buildService:   NewServiceProxySQL,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyCluster,
		},
		"ProxySQL uses configured Local policy": {
			serviceName: "ProxySQL",
			serviceType: corev1.ServiceTypeNodePort,
			configureCR: func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.ProxySQL = &api.ProxySQLSpec{
					Expose: api.ServiceExpose{
						Type:                  corev1.ServiceTypeNodePort,
						InternalTrafficPolicy: corev1.ServiceInternalTrafficPolicyLocal,
					},
				}
			},
			buildService:   NewServiceProxySQL,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyLocal,
		},
		"HAProxy primary defaults to Cluster": {
			serviceName: "HAProxy primary",
			configureCR: func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.HAProxy = &api.HAProxySpec{}
			},
			buildService:   NewServiceHAProxy,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyCluster,
		},
		"HAProxy primary uses configured Local policy": {
			serviceName: "HAProxy primary",
			serviceType: corev1.ServiceTypeLoadBalancer,
			configureCR: func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.HAProxy = &api.HAProxySpec{
					ExposePrimary: api.ServiceExpose{
						Type:                  corev1.ServiceTypeLoadBalancer,
						InternalTrafficPolicy: corev1.ServiceInternalTrafficPolicyLocal,
					},
				}
			},
			buildService:   NewServiceHAProxy,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyLocal,
		},
		"HAProxy replicas defaults to Cluster": {
			serviceName: "HAProxy replicas",
			configureCR: func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.HAProxy = &api.HAProxySpec{
					ExposeReplicas: &api.ReplicasServiceExpose{},
				}
			},
			buildService:   NewServiceHAProxyReplicas,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyCluster,
		},
		"HAProxy replicas uses configured Local policy": {
			serviceName: "HAProxy replicas",
			serviceType: corev1.ServiceTypeNodePort,
			configureCR: func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.HAProxy = &api.HAProxySpec{
					ExposeReplicas: &api.ReplicasServiceExpose{
						ServiceExpose: api.ServiceExpose{
							Type:                  corev1.ServiceTypeNodePort,
							InternalTrafficPolicy: corev1.ServiceInternalTrafficPolicyLocal,
						},
					},
				}
			},
			buildService:   NewServiceHAProxyReplicas,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyLocal,
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
					CRVersion: version.Version(),
				},
			}
			tt.configureCR(cr)

			svc := tt.buildService(cr)

			require.NotNil(t, svc.Spec.InternalTrafficPolicy, "%s service InternalTrafficPolicy is nil", tt.serviceName)
			assert.Equal(t, tt.expectedPolicy, *svc.Spec.InternalTrafficPolicy)

			if tt.serviceType != "" {
				assert.Equal(t, tt.serviceType, svc.Spec.Type)
			}
		})
	}
}
