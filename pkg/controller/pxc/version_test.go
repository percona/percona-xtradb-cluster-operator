package pxc

import (
	"context"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/version"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"testing"

	apiv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestVersionMeta(t *testing.T) {
	tests := []struct {
		name            string
		modify          func(cr *apiv1.PerconaXtraDBCluster)
		want            versionMeta
		clusterWide     bool
		namespace       string
		watchNamespaces string
	}{
		{
			name: "Cluster-wide turn off",
			modify: func(cr *apiv1.PerconaXtraDBCluster) {
			},
			want: versionMeta{
				Apply:              "disabled",
				Platform:           string(version.PlatformKubernetes),
				ClusterWideEnabled: false,
			},
			clusterWide:     false,
			namespace:       "test-namespace",
			watchNamespaces: "test-namespace",
		},
		{
			name:   "Cluster-wide with specified namespaces",
			modify: func(cr *apiv1.PerconaXtraDBCluster) {},
			want: versionMeta{
				Apply:              "disabled",
				Platform:           string(version.PlatformKubernetes),
				ClusterWideEnabled: true,
			},
			clusterWide:     true,
			namespace:       "test-namespace",
			watchNamespaces: "test-namespace,another-namespace",
		},
		{
			name:   "Cluster-wide with empty namespaces",
			modify: func(cr *apiv1.PerconaXtraDBCluster) {},
			want: versionMeta{
				Apply:              "disabled",
				Platform:           string(version.PlatformKubernetes),
				ClusterWideEnabled: true,
			},
			clusterWide:     true,
			namespace:       "test-namespace",
			watchNamespaces: "",
		},
	}

	size := int32(1)
	operatorName := "percona-xtradb-cluster-operator"
	operatorDepl := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorName,
			Namespace: "",
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
		ctx := context.Background()
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(k8s.WatchNamespaceEnvVar, tt.namespace)
			if tt.clusterWide {
				t.Setenv(k8s.WatchNamespaceEnvVar, tt.watchNamespaces)
			}

			cr, err := readDefaultCR("cluster1", "pxc-operator")

			tt.modify(cr)
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

			if err := r.setCRVersion(context.TODO(), cr); err != nil {
				t.Fatal(err, "set CR version")
			}
			if err := cr.CheckNSetDefaults(new(version.ServerVersion), logf.FromContext(ctx)); err != nil {
				t.Fatal(err)
			}

			vm, err := r.getVersionMeta(cr)

			if err != nil {
				t.Fatal(err)
			}
			if vm != tt.want {
				t.Fatalf("Have: %v; Want: %v", vm, tt.want)
			}
		})
	}
}
