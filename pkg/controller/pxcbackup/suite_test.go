package pxcbackup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" // nolint
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestPxcbackup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PerconaXtraDBClusterBackup Suite")
}

func readDefaultCR(name, namespace string) (*pxcv1.PerconaXtraDBCluster, error) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "cr.yaml"))
	if err != nil {
		return nil, err
	}

	cr := &pxcv1.PerconaXtraDBCluster{}

	if err := yaml.Unmarshal(data, cr); err != nil {
		return cr, err
	}

	cr.Name = name
	cr.Namespace = namespace
	cr.Spec.InitImage = "perconalab/percona-xtradb-cluster-operator:main"
	b := false
	cr.Spec.PXC.AutoRecovery = &b

	v := version.ServerVersion{
		Platform: version.PlatformKubernetes,
		Info:     k8sversion.Info{},
	}

	log := logf.FromContext(context.Background())
	if err := cr.CheckNSetDefaults(&v, log); err != nil {
		return cr, err
	}

	return cr, nil
}

func readDefaultBackup(name, namespace string) (*pxcv1.PerconaXtraDBClusterBackup, error) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "backup", "backup.yaml"))
	if err != nil {
		return nil, err
	}

	cr := &pxcv1.PerconaXtraDBClusterBackup{}

	if err := yaml.Unmarshal(data, cr); err != nil {
		return cr, err
	}

	cr.Name = name
	cr.Namespace = namespace

	return cr, nil
}

func reconciler(cl client.Client) *ReconcilePerconaXtraDBClusterBackup {
	return &ReconcilePerconaXtraDBClusterBackup{
		client: cl,
		scheme: cl.Scheme(),
	}
}

// buildFakeClient creates a fake client to mock API calls with the mock objects
func buildFakeClient(objs ...runtime.Object) client.Client {
	s := scheme.Scheme

	s.AddKnownTypes(pxcv1.SchemeGroupVersion, new(pxcv1.PerconaXtraDBClusterRestore))
	s.AddKnownTypes(pxcv1.SchemeGroupVersion, new(pxcv1.PerconaXtraDBClusterBackup))
	s.AddKnownTypes(pxcv1.SchemeGroupVersion, new(pxcv1.PerconaXtraDBCluster))

	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&pxcv1.PerconaXtraDBClusterRestore{}).
		Build()

	return cl
}
