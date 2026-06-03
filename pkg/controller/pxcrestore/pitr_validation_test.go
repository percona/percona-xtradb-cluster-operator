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
		// transaction: single UUID:N only (recoverer uses SplitN+ParseInt, tagged format UUID:tag:N not supported)
		Entry("type transaction with single gtid", "valid-transaction", &pxcv1.PITR{Type: "transaction", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:42"}),
		// skip: full GTID set syntax is allowed, including tags (MySQL 8.4)
		Entry("type skip with single gtid", "valid-skip-single", &pxcv1.PITR{Type: "skip", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:42"}),
		Entry("type skip with gtid range", "valid-skip-range", &pxcv1.PITR{Type: "skip", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:1-10"}),
		Entry("type skip with multiple intervals", "valid-skip-intervals", &pxcv1.PITR{Type: "skip", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:1-5:7-9"}),
		Entry("type skip with multi-source gtid set", "valid-skip-multi-source", &pxcv1.PITR{Type: "skip", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:1-10,bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee:1-5"}),
		Entry("type skip with tagged gtid", "valid-skip-tagged", &pxcv1.PITR{Type: "skip", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:Domain_1:1-10"}),
		Entry("type skip with multiple tags", "valid-skip-multi-tag", &pxcv1.PITR{Type: "skip", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:Domain_1:1-3:Domain_2:8-52"}),
	)

	DescribeTable("invalid PITR configurations",
		func(name string, pitr *pxcv1.PITR, errMsg string) {
			err := k8sClient.Create(ctx, newRestore(name, pitr))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errMsg))
		},
		// date type
		Entry("type date with empty date", "invalid-date-empty", &pxcv1.PITR{Type: "date"}, "Date is required"),
		Entry("type date with wrong format", "invalid-date-format", &pxcv1.PITR{Type: "date", Date: "15-01-2024 12:30:00"}, "format YYYY-MM-DD"),
		Entry("type date with invalid month", "invalid-date-month", &pxcv1.PITR{Type: "date", Date: "2024-27-30 12:30:00"}, "format YYYY-MM-DD"),
		Entry("type date with no time", "invalid-date-no-time", &pxcv1.PITR{Type: "date", Date: "2024-12-30"}, "format YYYY-MM-DD"),
		// latest type
		Entry("type latest with date set", "invalid-latest-date", &pxcv1.PITR{Type: "latest", Date: "2024-01-15 12:30:00"}, "Date and GTID should not be set"),
		Entry("type latest with gtid set", "invalid-latest-gtid", &pxcv1.PITR{Type: "latest", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:1"}, "Date and GTID should not be set"),
		// unknown type
		Entry("unknown type", "invalid-unknown-type", &pxcv1.PITR{Type: "unknown"}, "Unsupported value"),
		// transaction: GTID is required and must be UUID:N or UUID:tag:N (N >= 1)
		Entry("type transaction without gtid", "invalid-transaction-no-gtid", &pxcv1.PITR{Type: "transaction"}, "GTID is required"),
		Entry("type transaction with non-uuid gtid", "invalid-transaction-bad-uuid", &pxcv1.PITR{Type: "transaction", GTID: "notauuid:42"}, "single transaction identifier"),
		Entry("type transaction with gtid range", "invalid-transaction-range", &pxcv1.PITR{Type: "transaction", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:1-10"}, "single transaction identifier"),
		Entry("type transaction with multi-source gtid set", "invalid-transaction-multi-source", &pxcv1.PITR{Type: "transaction", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:1,bbbbbbbb-bbbb-cccc-dddd-eeeeeeeeeeee:1"}, "single transaction identifier"),
		Entry("type transaction with zero transaction id", "invalid-transaction-zero", &pxcv1.PITR{Type: "transaction", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:0"}, "single transaction identifier"),
		Entry("type transaction with tagged gtid", "invalid-transaction-tagged", &pxcv1.PITR{Type: "transaction", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:Domain_1:42"}, "single transaction identifier"),
		// skip: GTID is required and must be a valid MySQL GTID set
		Entry("type skip without gtid", "invalid-skip-no-gtid", &pxcv1.PITR{Type: "skip"}, "GTID is required"),
		Entry("type skip with non-uuid gtid", "invalid-skip-bad-uuid", &pxcv1.PITR{Type: "skip", GTID: "notauuid:1-10"}, "valid MySQL GTID set"),
		Entry("type skip with malformed gtid", "invalid-skip-malformed", &pxcv1.PITR{Type: "skip", GTID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}, "valid MySQL GTID set"),
	)
})
