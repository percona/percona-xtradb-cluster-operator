package pxcbackup

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
)

var _ = Describe("Starting deadline", func() {
	It("should be optional", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		cluster.Spec.Backup.StartingDeadlineSeconds = nil

		bcp.Spec.StartingDeadlineSeconds = nil
		bcp.Status.State = pxcv1.BackupNew

		err = checkStartingDeadline(context.Background(), cluster, bcp)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should use universal value if defined", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		cluster.Spec.Backup.StartingDeadlineSeconds = ptr.To(int64(60))

		bcp.Status.State = pxcv1.BackupNew
		bcp.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Minute))

		err = checkStartingDeadline(context.Background(), cluster, bcp)
		Expect(err).To(HaveOccurred())
	})

	It("should use particular value if defined", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		cluster.Spec.Backup.StartingDeadlineSeconds = ptr.To(int64(600))

		bcp.Status.State = pxcv1.BackupNew
		bcp.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Minute))
		bcp.Spec.StartingDeadlineSeconds = ptr.To(int64(60))

		err = checkStartingDeadline(context.Background(), cluster, bcp)
		Expect(err).To(HaveOccurred())
	})

	It("should not return an error", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		cluster.Spec.Backup.StartingDeadlineSeconds = ptr.To(int64(600))

		bcp.Status.State = pxcv1.BackupNew
		bcp.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Minute))
		bcp.Spec.StartingDeadlineSeconds = ptr.To(int64(300))

		err = checkStartingDeadline(context.Background(), cluster, bcp)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("Running deadline", func() {
	It("should be optional", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		cluster.Spec.Backup.RunningDeadlineSeconds = nil
		bcp.Spec.RunningDeadlineSeconds = nil
		bcp.Status.State = pxcv1.BackupStarting

		r := reconciler(buildFakeClient())

		err = r.checkRunningDeadline(context.Background(), cluster, bcp)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return early if not in 'Starting' state", func() {
		r := reconciler(buildFakeClient())

		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		states := []pxcv1.PXCBackupState{
			pxcv1.BackupNew,
			pxcv1.BackupSucceeded,
			pxcv1.BackupFailed,
			pxcv1.BackupSuspended,
		}

		for _, state := range states {
			bcp.Status.State = state
			err = r.checkRunningDeadline(context.Background(), cluster, bcp)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should use universal value if defined", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		cr, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())
		cr.Status.State = pxcv1.BackupStarting

		bcp := backup.New(cluster)
		job := bcp.Job(cr, cluster)

		job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, "")
		Expect(err).ToNot(HaveOccurred())
		creationTs := metav1.NewTime(time.Now().Add(-2 * time.Minute))
		job.CreationTimestamp = creationTs

		r := reconciler(buildFakeClient(job))

		cluster.Spec.Backup.RunningDeadlineSeconds = ptr.To(int32(60))
		cr.Spec.RunningDeadlineSeconds = nil

		err = r.checkRunningDeadline(context.Background(), cluster, cr)
		Expect(err).To(HaveOccurred())
	})

	It("should use particular value if defined", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		cr, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())
		cr.Status.State = pxcv1.BackupStarting

		bcp := backup.New(cluster)
		job := bcp.Job(cr, cluster)

		job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, "")
		Expect(err).ToNot(HaveOccurred())
		creationTs := metav1.NewTime(time.Now().Add(-2 * time.Minute))
		job.CreationTimestamp = creationTs

		r := reconciler(buildFakeClient(job))

		cluster.Spec.Backup.RunningDeadlineSeconds = ptr.To(int32(60)) // this one is ignored
		cr.Spec.RunningDeadlineSeconds = ptr.To(int32(300))

		err = r.checkRunningDeadline(context.Background(), cluster, cr)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("Suspended deadline", func() {
	It("should do an early return without a job", func() {
		r := reconciler(buildFakeClient())

		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		err = r.checkSuspendedDeadline(context.Background(), cluster, bcp)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should be optional", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		cr, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp := backup.New(cluster)
		job := bcp.Job(cr, cluster)

		job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, "")
		Expect(err).ToNot(HaveOccurred())

		job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
			Type:               batchv1.JobSuspended,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
		})

		r := reconciler(buildFakeClient(job))

		cluster.Spec.Backup.SuspendedDeadlineSeconds = nil
		cr.Spec.SuspendedDeadlineSeconds = nil

		err = r.checkSuspendedDeadline(context.Background(), cluster, cr)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should use universal value if defined", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		cr, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp := backup.New(cluster)
		job := bcp.Job(cr, cluster)

		job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, "")
		Expect(err).ToNot(HaveOccurred())

		job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
			Type:               batchv1.JobSuspended,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
		})

		r := reconciler(buildFakeClient(job))

		cluster.Spec.Backup.SuspendedDeadlineSeconds = ptr.To(int64(60))
		cr.Spec.SuspendedDeadlineSeconds = nil

		err = r.checkSuspendedDeadline(context.Background(), cluster, cr)
		Expect(err).To(HaveOccurred())
	})

	It("should use particular value if defined", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		cr, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp := backup.New(cluster)
		job := bcp.Job(cr, cluster)

		job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, "")
		Expect(err).ToNot(HaveOccurred())

		job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
			Type:               batchv1.JobSuspended,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
		})

		r := reconciler(buildFakeClient(job))

		cluster.Spec.Backup.SuspendedDeadlineSeconds = ptr.To(int64(600))
		cr.Spec.SuspendedDeadlineSeconds = ptr.To(int64(60))

		err = r.checkSuspendedDeadline(context.Background(), cluster, cr)
		Expect(err).To(HaveOccurred())
	})

	It("should clean up suspended job", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		cr, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp := backup.New(cluster)
		job := bcp.Job(cr, cluster)

		job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, "")
		Expect(err).ToNot(HaveOccurred())

		job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
			Type:               batchv1.JobSuspended,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
		})

		cl := buildFakeClient(job)
		r := reconciler(cl)

		cr.Spec.SuspendedDeadlineSeconds = ptr.To(int64(60))

		err = r.checkSuspendedDeadline(context.Background(), cluster, cr)
		Expect(err).To(HaveOccurred())

		err = r.cleanUpSuspendedJob(context.Background(), cluster, cr)
		Expect(err).NotTo(HaveOccurred())

		j := new(batchv1.Job)
		err = cl.Get(context.Background(), client.ObjectKeyFromObject(job), j)
		Expect(err).To(HaveOccurred())
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	})

	It("should not return an error", func() {
		cluster, err := readDefaultCR("cluster1", "test")
		Expect(err).ToNot(HaveOccurred())

		cr, err := readDefaultBackup("backup1", "test")
		Expect(err).ToNot(HaveOccurred())

		bcp := backup.New(cluster)
		job := bcp.Job(cr, cluster)

		job.Spec, err = bcp.JobSpec(cr.Spec, cluster, job, "")
		Expect(err).ToNot(HaveOccurred())

		job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
			Type:               batchv1.JobSuspended,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
		})

		r := reconciler(buildFakeClient(job))

		cluster.Spec.Backup.SuspendedDeadlineSeconds = ptr.To(int64(600))
		cr.Spec.SuspendedDeadlineSeconds = ptr.To(int64(300))

		err = r.checkSuspendedDeadline(context.Background(), cluster, cr)
		Expect(err).ToNot(HaveOccurred())
	})
})
