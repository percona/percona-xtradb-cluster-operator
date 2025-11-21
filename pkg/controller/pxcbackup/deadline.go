package pxcbackup

import (
	"context"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

var (
	errSuspendedDeadlineExceeded = errors.New("suspended deadline seconds exceeded")
	errStartingDeadlineExceeded  = errors.New("starting deadline seconds exceeded")
	errRunningDeadlineExceeded   = errors.New("running deadline seconds exceeded")
)

func (r *ReconcilePerconaXtraDBClusterBackup) checkDeadlines(ctx context.Context, cluster *api.PerconaXtraDBCluster, cr *api.PerconaXtraDBClusterBackup) error {
	if err := checkStartingDeadline(ctx, cluster, cr); err != nil {
		return err
	}

	if err := r.checkRunningDeadline(ctx, cluster, cr); err != nil {
		return err
	}

	if err := r.checkSuspendedDeadline(ctx, cluster, cr); err != nil {
		return err
	}

	return nil
}

func checkStartingDeadline(ctx context.Context, cluster *api.PerconaXtraDBCluster, cr *api.PerconaXtraDBClusterBackup) error {
	log := logf.FromContext(ctx)

	if cr.Status.State != api.BackupNew {
		return nil
	}

	var deadlineSeconds *int64
	if cr.Spec.StartingDeadlineSeconds != nil {
		deadlineSeconds = cr.Spec.StartingDeadlineSeconds
	} else if cluster.Spec.Backup.StartingDeadlineSeconds != nil {
		deadlineSeconds = cluster.Spec.Backup.StartingDeadlineSeconds
	}

	if deadlineSeconds == nil {
		return nil
	}

	since := time.Since(cr.CreationTimestamp.Time).Seconds()
	if since < float64(*deadlineSeconds) {
		return nil
	}

	log.Info("Backup didn't start in startingDeadlineSeconds, failing the backup",
		"startingDeadlineSeconds", *deadlineSeconds,
		"passedSeconds", since)

	return errStartingDeadlineExceeded
}

func (r *ReconcilePerconaXtraDBClusterBackup) checkRunningDeadline(ctx context.Context, cluster *api.PerconaXtraDBCluster, cr *api.PerconaXtraDBClusterBackup) error {
	log := logf.FromContext(ctx)

	// check only if the current state is 'Starting'
	if cr.Status.State != api.BackupStarting {
		return nil
	}

	var deadlineSeconds *int32
	if cr.Spec.RunningDeadlineSeconds != nil {
		deadlineSeconds = cr.Spec.RunningDeadlineSeconds
	} else if cluster.Spec.Backup.RunningDeadlineSeconds != nil {
		deadlineSeconds = cluster.Spec.Backup.RunningDeadlineSeconds
	}

	if deadlineSeconds == nil {
		return nil
	}

	job, err := r.getBackupJob(ctx, cluster, cr)
	if err != nil {
		return err
	}

	// The backup goes into 'Starting' typically around the same time as the job is created,
	// so we will compare against the creation time of the job.
	since := time.Since(job.CreationTimestamp.Time).Seconds()
	if since < float64(*deadlineSeconds) {
		return nil
	}

	log.Info("Backup didn't start running in runningDeadlineSeconds, failing the backup",
		"runningDeadlineSeconds", *deadlineSeconds,
		"passedSeconds", since)

	return errRunningDeadlineExceeded
}

func (r *ReconcilePerconaXtraDBClusterBackup) checkSuspendedDeadline(
	ctx context.Context,
	cluster *api.PerconaXtraDBCluster,
	cr *api.PerconaXtraDBClusterBackup,
) error {
	log := logf.FromContext(ctx)

	job, err := r.getBackupJob(ctx, cluster, cr)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	var deadlineSeconds *int64
	if cr.Spec.SuspendedDeadlineSeconds != nil {
		deadlineSeconds = cr.Spec.SuspendedDeadlineSeconds
	} else if cluster.Spec.Backup.SuspendedDeadlineSeconds != nil {
		deadlineSeconds = cluster.Spec.Backup.SuspendedDeadlineSeconds
	}

	if deadlineSeconds == nil {
		return nil
	}

	for _, cond := range job.Status.Conditions {
		if cond.Type != batchv1.JobSuspended || cond.Status != corev1.ConditionTrue {
			continue
		}

		if since := time.Since(cond.LastTransitionTime.Time).Seconds(); since > float64(*deadlineSeconds) {
			log.Info("Backup didn't resume in suspendedDeadlineSeconds, failing the backup",
				"suspendedDeadlineSeconds", *deadlineSeconds,
				"passedSeconds", since)
			return errSuspendedDeadlineExceeded
		}
	}

	return nil
}
