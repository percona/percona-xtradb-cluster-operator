package pxcrestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
)

func TestPxcrestore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PerconaXtraDBClusterRestore Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	Expect(os.Setenv("WATCH_NAMESPACE", "default")).NotTo(HaveOccurred())

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
	Expect(os.Unsetenv("WATCH_NAMESPACE")).NotTo(HaveOccurred())
	Expect(testEnv.Stop()).NotTo(HaveOccurred())
})

var _ = Describe("PerconaXtraDBClusterRestore PITR CRD validation", Ordered, func() {
	ctx := context.Background()
	const ns = "pitr-validation"

	BeforeAll(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	AfterAll(func() {
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	})

	newRestore := func(name string, pitr *pxcv1.PITR) *pxcv1.PerconaXtraDBClusterRestore {
		return &pxcv1.PerconaXtraDBClusterRestore{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: pxcv1.PerconaXtraDBClusterRestoreSpec{
				PXCCluster: "cluster1",
				BackupName: "backup1",
				PITR:       pitr,
			},
		}
	}

	DescribeTable("valid PITR configurations",
		func(name string, pitr *pxcv1.PITR) {
			Expect(k8sClient.Create(ctx, newRestore(name, pitr))).To(Succeed())
		},
		Entry("type latest", "valid-latest", &pxcv1.PITR{Type: "latest"}),
		Entry("type date with valid format", "valid-date", &pxcv1.PITR{Type: "date", Date: "2024-01-15 12:30:00"}),
		Entry("type transaction with gtid", "valid-transaction", &pxcv1.PITR{Type: "transaction", GTID: "abc123:1-10"}),
	)

	DescribeTable("invalid PITR configurations",
		func(name string, pitr *pxcv1.PITR, errMsg string) {
			err := k8sClient.Create(ctx, newRestore(name, pitr))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errMsg))
		},
		Entry("type date with empty date", "invalid-date-empty", &pxcv1.PITR{Type: "date"}, "Date is required"),
		Entry("type date with wrong format", "invalid-date-format", &pxcv1.PITR{Type: "date", Date: "15-01-2024 12:30:00"}, "format YYYY-MM-DD"),
		Entry("type latest with date set", "invalid-latest-date", &pxcv1.PITR{Type: "latest", Date: "2024-01-15 12:30:00"}, "Date and GTID should not be set"),
		Entry("type transaction without gtid", "invalid-transaction-no-gtid", &pxcv1.PITR{Type: "transaction"}, "GTID is required"),
		Entry("unknown type", "invalid-unknown-type", &pxcv1.PITR{Type: "unknown"}, "Unsupported value"),
	)
})
