package pxc

import (
	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestVersionMeta(t *testing.T) {
	tests := []struct {
		name              string
		want              versionMeta
		namespace         string
		watchNamespaces   string
		expectError       bool
		setWatchNamespace bool
	}{
		{
			name: "Cluster-wide turn off",
			want: versionMeta{
				Apply:              "disabled",
				ClusterWideEnabled: false,
			},
			namespace:         "test-namespace",
			watchNamespaces:   "test-namespace",
			setWatchNamespace: true,
			expectError:       false,
		},
		{
			name:              "Cluster-wide unset (env var not set)",
			want:              versionMeta{},
			namespace:         "test-namespace",
			watchNamespaces:   "",
			setWatchNamespace: false,
			expectError:       true,
		},
		{
			name: "Cluster-wide with specified namespaces",
			want: versionMeta{
				Apply:              "disabled",
				ClusterWideEnabled: true,
			},
			namespace:         "test-namespace",
			watchNamespaces:   "test-namespace,another-namespace",
			setWatchNamespace: true,
			expectError:       false,
		},
		{
			name: "Cluster-wide with empty namespaces",
			want: versionMeta{
				Apply:              "disabled",
				ClusterWideEnabled: true,
			},
			namespace:         "test-namespace",
			watchNamespaces:   "",
			setWatchNamespace: true,
			expectError:       false,
		},
	}

	size := int32(1)
	operatorName := "percona-xtradb-cluster-operator"
	operatorDepl := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorName,
			Namespace: "pxc-operator",
			Labels:    make(map[string]string),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &size,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "percona-xtradb-cluster-operator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": "percona-xtradb-cluster-operator",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "percona-xtradb-cluster-operator",
					Containers: []corev1.Container{
						{
							Name: "percona-xtradb-cluster-operator",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setWatchNamespace {
				t.Setenv(k8s.WatchNamespaceEnvVar, tt.watchNamespaces)
			}

			cr, err := readDefaultCR("cluster1", tt.namespace)
			if err != nil {
				t.Fatalf("failed to read default CR: %v", err)
			}

			scheme := k8sruntime.NewScheme()
			if err := clientgoscheme.AddToScheme(scheme); err != nil {
				t.Fatal(err, "failed to add client-go scheme")
			}
			if err := apis.AddToScheme(scheme); err != nil {
				t.Fatal(err, "failed to add apis scheme")
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr, &operatorDepl).Build()
			sv := &version.ServerVersion{Platform: version.PlatformKubernetes}
			r := &ReconcilePerconaXtraDBCluster{
				client:        cl,
				scheme:        scheme,
				serverVersion: sv,
			}

			vm, err := r.getVersionMeta(cr)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if vm.Apply != tt.want.Apply || vm.ClusterWideEnabled != tt.want.ClusterWideEnabled {
				t.Fatalf("Have: %+v; Want: %+v", vm, tt.want)
			}
		})
	}
}
