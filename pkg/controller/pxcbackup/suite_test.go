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
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" // nolint
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
)

func TestPxcbackup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PerconaXtraDBClusterBackup Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	errEnv := os.Setenv("WATCH_NAMESPACE", "default")
	Expect(errEnv).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = apis.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	errEnv := os.Unsetenv("WATCH_NAMESPACE")
	Expect(errEnv).NotTo(HaveOccurred())

	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

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
