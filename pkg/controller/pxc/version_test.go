package pxc

import (
	"context"
	"fmt"
	gwRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/version"
	"go.nhat.io/grpcmock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"net"
	"net/http"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"testing"

	pbVersion "github.com/Percona-Lab/percona-version-service/versionpb"
	apiv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
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

func (vs *fakeVS) Apply(_ context.Context, req any) (any, error) {
	if vs.unimplemented {
		return nil, errors.New("unimplemented")
	}
	r := req.(*pbVersion.ApplyRequest)
	switch r.Apply {
	case string(apiv1.UpgradeStrategyDisabled), string(apiv1.UpgradeStrategyNever):
		return &pbVersion.VersionResponse{}, nil
	}

	have := &pbVersion.ApplyRequest{
		BackupVersion:     r.GetBackupVersion(),
		CustomResourceUid: r.GetCustomResourceUid(),
		DatabaseVersion:   r.GetDatabaseVersion(),
		KubeVersion:       r.GetKubeVersion(),
		NamespaceUid:      r.GetNamespaceUid(),
		OperatorVersion:   r.GetOperatorVersion(),
		Platform:          r.GetPlatform(),
		Product:           r.GetProduct(),
		HaproxyVersion:    r.GetHaproxyVersion(),
		PmmVersion:        r.GetPmmVersion(),
	}
	want := &pbVersion.ApplyRequest{
		BackupVersion:     "backup-version",
		CustomResourceUid: "custom-resource-uid",
		DatabaseVersion:   "database-version",
		KubeVersion:       "kube-version",
		OperatorVersion:   version.Version,
		Product:           "ps-operator",
		Platform:          string(version.PlatformKubernetes),
		HaproxyVersion:    "haproxy-version",
		PmmVersion:        "pmm-version",
	}

	if !reflect.DeepEqual(have, want) {
		return nil, errors.Errorf("Have: %v; Want: %v", have, want)
	}

	return &pbVersion.VersionResponse{
		Versions: []*pbVersion.OperatorVersion{
			{
				Matrix: &pbVersion.VersionMatrix{
					Mysql: map[string]*pbVersion.Version{
						"mysql-version": {
							ImagePath: "mysql-image",
						},
					},
					Backup: map[string]*pbVersion.Version{
						"backup-version": {
							ImagePath: "backup-image",
						},
					},
					Pmm: map[string]*pbVersion.Version{
						"pmm-version": {
							ImagePath: "pmm-image",
						},
					},
					Orchestrator: map[string]*pbVersion.Version{
						"orchestrator-version": {
							ImagePath: "orchestrator-image",
						},
					},
					Router: map[string]*pbVersion.Version{
						"router-version": {
							ImagePath: "router-image",
						},
					},
					Haproxy: map[string]*pbVersion.Version{
						"haproxy-version": {
							ImagePath: "haproxy-image",
						},
					},
				},
			},
		},
	}, nil
}

type fakeVS struct {
	addr          string
	gwPort        int
	unimplemented bool
}

func fakeVersionService(addr string, gwport int, unimplemented bool) *fakeVS {
	return &fakeVS{
		addr:          addr,
		gwPort:        gwport,
		unimplemented: unimplemented,
	}
}

type mockClientConn struct {
	dialer grpcmock.ContextDialer
}

func (m *mockClientConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	return grpcmock.InvokeUnary(ctx, method, args, reply, grpcmock.WithInsecure(), grpcmock.WithCallOptions(opts...), grpcmock.WithContextDialer(m.dialer))
}

func (m *mockClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("unimplemented")
}

func (vs *fakeVS) Start(t *testing.T) error {
	_, d := grpcmock.MockServerWithBufConn(
		grpcmock.RegisterServiceFromInstance("version.VersionService", (*pbVersion.VersionServiceServer)(nil)),
		func(s *grpcmock.Server) {
			s.ExpectUnary("/version.VersionService/Apply").Run(vs.Apply)
		},
	)(t)

	gwmux := gwRuntime.NewServeMux()
	err := pbVersion.RegisterVersionServiceHandlerClient(context.Background(), gwmux, pbVersion.NewVersionServiceClient(&mockClientConn{d}))
	if err != nil {
		return errors.Wrap(err, "failed to register gateway")
	}
	gwServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", vs.addr, vs.gwPort),
		Handler: gwmux,
	}
	gwLis, err := net.Listen("tcp", gwServer.Addr)
	if err != nil {
		return errors.Wrap(err, "failed to listen gateway")
	}
	go func() {
		if err := gwServer.Serve(gwLis); err != nil {
			t.Error("failed to serve gRPC-Gateway", err)
		}
	}()

	return nil
}
