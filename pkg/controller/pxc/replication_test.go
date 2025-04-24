package pxc

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
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
		externalTrafficPolicy      corev1.ServiceExternalTrafficPolicyType
		expectedExternalPolicy     corev1.ServiceExternalTrafficPolicyType
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

			if svc.Spec.Type != tt.expectedServiceType {
				t.Errorf("expected Service Type %v, got %v", tt.expectedServiceType, svc.Spec.Type)
			}

			if svc.Spec.ExternalTrafficPolicy != tt.expectedExternalPolicy {
				t.Errorf("expected ExternalTrafficPolicy %v, got %v", tt.expectedExternalPolicy, svc.Spec.ExternalTrafficPolicy)
			}

			if !reflect.DeepEqual(svc.Annotations, customAnnotations) {
				t.Errorf("expected Annotations %v, got %v", customAnnotations, svc.Annotations)
			}

			if !reflect.DeepEqual(svc.Spec.LoadBalancerSourceRanges, sourceRanges) {
				t.Errorf("expected LoadBalancerSourceRanges %v, got %v", sourceRanges, svc.Spec.LoadBalancerSourceRanges)
			}

			if tt.expectLoadBalancerClassSet && svc.Spec.LoadBalancerClass == nil {
				t.Errorf("expected LoadBalancerClass to be set")
			}

			if !tt.expectLoadBalancerClassSet && svc.Spec.LoadBalancerClass != nil {
				t.Errorf("expected LoadBalancerClass to be nil, got %v", *svc.Spec.LoadBalancerClass)
			}
		})
	}
}
