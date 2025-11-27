package pxcbackup

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("PerconaXtraDBClusterBackup", Ordered, func() {
	ctx := context.Background()
	const ns = "pxc"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	clusterName := "cluster1"
	cluster := &pxcv1.PerconaXtraDBCluster{}

	reconciler := &ReconcilePerconaXtraDBClusterBackup{}
	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))

		By("Creating a PXC Cluster to perform the tests")
		cluster, err = readDefaultCR(clusterName, ns)
		Expect(err).To(Not(HaveOccurred()))

		err = k8sClient.Create(ctx, cluster)
		Expect(err).To(Not(HaveOccurred()))

		mockPXCReadyStatus(ctx, cluster)
		reconciler = newTestReconciler()
	})

	AfterAll(func() {
		// TODO(user): Attention if you improve this code by adding other context test you MUST
		// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)

		By("Deleting the PXC Cluster to perform the tests")
		cluster, err := readDefaultCR(clusterName, ns)
		Expect(err).To(Not(HaveOccurred()))
		err = k8sClient.Delete(ctx, cluster)
		Expect(err).To(Not(HaveOccurred()))
	})

	It("Should reconcile backup", func() {
		pxcBackup, err := newBackup("backup1", ns)
		Expect(err).To(Not(HaveOccurred()))

		err = k8sClient.Create(ctx, pxcBackup)
		Expect(err).To(Not(HaveOccurred()))

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      pxcBackup.Name,
			Namespace: pxcBackup.Namespace,
		}})
		Expect(err).To(Succeed())

		// Check that a job was created
		bcp := backup.New(cluster)
		job := bcp.Job(pxcBackup, cluster)
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      job.Name,
			Namespace: job.Namespace,
		}, job)
		Expect(err).To(Not(HaveOccurred()))
	})
})

var _ = Describe("Error checking deadlines", Ordered, func() {
	ctx := context.Background()
	const ns = "pxc-deadline-error"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	clusterName := "cluster1"
	cluster := &pxcv1.PerconaXtraDBCluster{}
	reconciler := &ReconcilePerconaXtraDBClusterBackup{}
	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))

		By("Creating a PXC Cluster to perform the tests")
		cluster, err = readDefaultCR(clusterName, ns)
		Expect(err).To(Not(HaveOccurred()))

		err = k8sClient.Create(ctx, cluster)
		Expect(err).To(Not(HaveOccurred()))

		mockPXCReadyStatus(ctx, cluster)
		reconciler = newTestReconciler()
	})

	AfterAll(func() {
		// TODO(user): Attention if you improve this code by adding other context test you MUST
		// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)

		By("Deleting the PXC Cluster to perform the tests")
		cluster, err := readDefaultCR(clusterName, ns)
		Expect(err).To(Not(HaveOccurred()))
		err = k8sClient.Delete(ctx, cluster)
		Expect(err).To(Not(HaveOccurred()))
	})

	It("Should not fail when error checking deadline", func() {
		pxcBackup, err := newBackup("backup1", ns)
		Expect(err).To(Not(HaveOccurred()))
		pxcBackup.Spec.RunningDeadlineSeconds = ptr.To(int64(300))

		pxcBackupReq := reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      pxcBackup.Name,
			Namespace: pxcBackup.Namespace,
		}}

		err = k8sClient.Create(ctx, pxcBackup)
		Expect(err).To(Not(HaveOccurred()))

		_, err = reconciler.Reconcile(ctx, pxcBackupReq)
		Expect(err).To(Succeed())

		err = k8sClient.Get(ctx, pxcBackupReq.NamespacedName, pxcBackup)
		Expect(err).To(Not(HaveOccurred()))
		Expect(pxcBackup.Status.State).To(Equal(pxcv1.BackupStarting))

		// We will delete the job, this will cause the deadline check to fail
		job := backup.New(cluster).Job(pxcBackup, cluster)
		err = k8sClient.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		Expect(err).To(Not(HaveOccurred()))

		_, err = reconciler.Reconcile(ctx, pxcBackupReq)
		Expect(err).To((HaveOccurred()))
		Expect(err.Error()).To(ContainSubstring("check deadlines"))

		// Make sure that the backup is not marked as failed
		err = k8sClient.Get(ctx, pxcBackupReq.NamespacedName, pxcBackup)
		Expect(pxcBackup.Status.State).To(Equal(pxcv1.BackupStarting))
	})
})

var _ = Describe("Backup Job deleted when running deadline is exceeded", Ordered, func() {
	ctx := context.Background()
	const ns = "pxc-job-running-deadline-exceeded"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	clusterName := "cluster1"
	cluster := &pxcv1.PerconaXtraDBCluster{}
	reconciler := &ReconcilePerconaXtraDBClusterBackup{}
	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))

		By("Creating a PXC Cluster to perform the tests")
		cluster, err = readDefaultCR(clusterName, ns)
		Expect(err).To(Not(HaveOccurred()))

		err = k8sClient.Create(ctx, cluster)
		Expect(err).To(Not(HaveOccurred()))

		mockPXCReadyStatus(ctx, cluster)
		reconciler = newTestReconciler()
	})

	AfterAll(func() {
		// TODO(user): Attention if you improve this code by adding other context test you MUST
		// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)

		By("Deleting the PXC Cluster to perform the tests")
		cluster, err := readDefaultCR(clusterName, ns)
		Expect(err).To(Not(HaveOccurred()))
		err = k8sClient.Delete(ctx, cluster)
		Expect(err).To(Not(HaveOccurred()))
	})

	It("Should delete the backup job when running deadline is exceeded", func() {
		pxcBackup, err := newBackup("backup1", ns)
		Expect(err).To(Not(HaveOccurred()))
		pxcBackup.Spec.RunningDeadlineSeconds = ptr.To(int64(5))

		pxcBackupReq := reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      pxcBackup.Name,
			Namespace: pxcBackup.Namespace,
		}}

		err = k8sClient.Create(ctx, pxcBackup)
		Expect(err).To(Not(HaveOccurred()))

		_, err = reconciler.Reconcile(ctx, pxcBackupReq)
		Expect(err).To(Succeed())

		err = k8sClient.Get(ctx, pxcBackupReq.NamespacedName, pxcBackup)
		Expect(err).To(Not(HaveOccurred()))
		Expect(pxcBackup.Status.State).To(Equal(pxcv1.BackupStarting))

		time.Sleep(6 * time.Second)

		_, err = reconciler.Reconcile(ctx, pxcBackupReq)
		Expect(err).To(Not(HaveOccurred()))

		// Make sure that the backup is marked as failed
		err = k8sClient.Get(ctx, pxcBackupReq.NamespacedName, pxcBackup)
		Expect(err).To(Not(HaveOccurred()))
		Expect(pxcBackup.Status.State).To(Equal(pxcv1.BackupFailed))
		Expect(pxcBackup.Status.Error).To(ContainSubstring("running deadline seconds exceeded"))

		// Make sure that the job is deleted
		job := backup.New(cluster).Job(pxcBackup, cluster)
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      job.Name,
			Namespace: job.Namespace,
		}, job)
		Expect(err).To(HaveOccurred())
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	})
})

func newBackup(name, ns string) (*pxcv1.PerconaXtraDBClusterBackup, error) {
	bkp, err := readDefaultBackup(name, ns)
	if err != nil {
		return nil, err
	}
	bkp.Spec.PXCCluster = "cluster1"
	return bkp, nil
}

func newTestReconciler() *ReconcilePerconaXtraDBClusterBackup {
	cli, _ := clientcmd.NewClient()
	return &ReconcilePerconaXtraDBClusterBackup{
		client: k8sClient,
		scheme: k8sClient.Scheme(),
		serverVersion: &version.ServerVersion{
			Platform: version.PlatformKubernetes,
		},
		clientcmd:           cli,
		chLimit:             make(chan struct{}, 10),
		bcpDeleteInProgress: new(sync.Map),
	}
}

func mockPXCReadyStatus(ctx context.Context, cluster *pxcv1.PerconaXtraDBCluster) {
	cluster.Status.Status = pxcv1.AppStateReady
	cluster.Status.PXC.Ready = cluster.Spec.PXC.Size
	err := k8sClient.Status().Update(ctx, cluster)
	Expect(err).To(Not(HaveOccurred()))
}
