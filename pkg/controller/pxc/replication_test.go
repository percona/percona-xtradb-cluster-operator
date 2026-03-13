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

func TestNewExposedPXCService(t *testing.T) {
	customAnnotations := map[string]string{
		"test-annotation": "value",
	}
	sourceRanges := []string{"192.168.0.0/16"}
	loadBalancerClass := "custom-lb-class"

	tests := map[string]struct {
		crVersion                  string
		serviceType                corev1.ServiceType
		externalTrafficPolicy      corev1.ServiceExternalTrafficPolicy
		expectedExternalPolicy     corev1.ServiceExternalTrafficPolicy
		expectedServiceType        corev1.ServiceType
		expectLoadBalancerClassSet bool
	}{
		"LoadBalancer with LB class on version >= 1.18.0": {
			crVersion:                  "1.18.0",
			serviceType:                corev1.ServiceTypeLoadBalancer,
			externalTrafficPolicy:      corev1.ServiceExternalTrafficPolicyTypeCluster,
			expectedExternalPolicy:     corev1.ServiceExternalTrafficPolicyTypeCluster,
			expectedServiceType:        corev1.ServiceTypeLoadBalancer,
			expectLoadBalancerClassSet: true,
		},
		"LoadBalancer without LB class on version < 1.18.0": {
			crVersion:              "1.17.0",
			serviceType:            corev1.ServiceTypeLoadBalancer,
			externalTrafficPolicy:  corev1.ServiceExternalTrafficPolicyTypeCluster,
			expectedExternalPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
			expectedServiceType:    corev1.ServiceTypeLoadBalancer,
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
					PXC: &api.PXCSpec{
						Expose: api.ServiceExpose{
							Type:                     tt.serviceType,
							ExternalTrafficPolicy:    tt.externalTrafficPolicy,
							Annotations:              customAnnotations,
							LoadBalancerSourceRanges: sourceRanges,
							LoadBalancerClass:        &loadBalancerClass,
						},
					},
				},
			}

			svc := NewExposedPXCService("pxc-0", cr)

			assert.Equal(t, tt.expectedServiceType, svc.Spec.Type)
			assert.Equal(t, tt.expectedExternalPolicy, svc.Spec.ExternalTrafficPolicy)
			assert.Equal(t, customAnnotations, svc.Annotations)
			assert.Equal(t, sourceRanges, svc.Spec.LoadBalancerSourceRanges)

			if tt.expectLoadBalancerClassSet {
				require.NotNil(t, svc.Spec.LoadBalancerClass)
			} else {
				assert.Nil(t, svc.Spec.LoadBalancerClass)
			}
		})
	}
}

func TestNewExposedPXCServiceInternalTrafficPolicy(t *testing.T) {
	tests := map[string]struct {
		serviceType    corev1.ServiceType
		internalPolicy corev1.ServiceInternalTrafficPolicy
		expectedPolicy corev1.ServiceInternalTrafficPolicy
	}{
		"ClusterIP defaults to Cluster": {
			serviceType:    corev1.ServiceTypeClusterIP,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyCluster,
		},
		"NodePort uses Local when configured": {
			serviceType:    corev1.ServiceTypeNodePort,
			internalPolicy: corev1.ServiceInternalTrafficPolicyLocal,
			expectedPolicy: corev1.ServiceInternalTrafficPolicyLocal,
		},
		"LoadBalancer falls back to Cluster for invalid value": {
			serviceType:    corev1.ServiceTypeLoadBalancer,
			internalPolicy: corev1.ServiceInternalTrafficPolicy("invalid"),
			expectedPolicy: corev1.ServiceInternalTrafficPolicyCluster,
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
					PXC: &api.PXCSpec{
						Expose: api.ServiceExpose{
							Type:                  tt.serviceType,
							InternalTrafficPolicy: tt.internalPolicy,
						},
					},
				},
			}

			svc := NewExposedPXCService("pxc-0", cr)

			require.NotNil(t, svc.Spec.InternalTrafficPolicy)
			assert.Equal(t, tt.expectedPolicy, *svc.Spec.InternalTrafficPolicy)
		})
	}
}
