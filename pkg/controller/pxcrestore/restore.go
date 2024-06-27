package pxcrestore

import (
	"context"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
)

func (r *ReconcilePerconaXtraDBClusterRestore) restore(ctx context.Context, cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	if cluster.Spec.Backup == nil {
		return errors.New("undefined backup section in a cluster spec")
	}

	restorer, err := r.getRestorer(ctx, cr, bcp, cluster)
	if err != nil {
		return errors.Wrap(err, "failed to get restorer")
	}
	job, err := restorer.Job()
	if err != nil {
		return errors.Wrap(err, "failed to get restore job")
	}
	if err = k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return err
	}

	if err = restorer.Init(ctx); err != nil {
		return errors.Wrap(err, "failed to init restore")
	}
	defer func() {
		if derr := restorer.Finalize(ctx); derr != nil {
			log.Error(derr, "failed to finalize restore")
		}
	}()

	return r.createJob(ctx, job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) pitr(ctx context.Context, cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	restorer, err := r.getRestorer(ctx, cr, bcp, cluster)
	if err != nil {
		return errors.Wrap(err, "failed to get restorer")
	}
	job, err := restorer.PITRJob()
	if err != nil {
		return errors.Wrap(err, "failed to create pitr restore job")
	}
	if err := k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return err
	}
	if err = restorer.Init(ctx); err != nil {
		return errors.Wrap(err, "failed to init restore")
	}
	defer func() {
		if derr := restorer.Finalize(ctx); derr != nil {
			log.Error(derr, "failed to finalize restore")
		}
	}()

	return r.createJob(ctx, job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) validate(ctx context.Context, cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) error {
	restorer, err := r.getRestorer(ctx, cr, bcp, cluster)
	if err != nil {
		return errors.Wrap(err, "failed to get restorer")
	}
	job, err := restorer.Job()
	if err != nil {
		return errors.Wrap(err, "failed to create restore job")
	}
	if err := restorer.ValidateJob(ctx, job); err != nil {
		return errors.Wrap(err, "failed to validate job")
	}

	if cr.Spec.PITR != nil {
		job, err := restorer.PITRJob()
		if err != nil {
			return errors.Wrap(err, "failed to create pitr restore job")
		}
		if err := restorer.ValidateJob(ctx, job); err != nil {
			return errors.Wrap(err, "failed to validate job")
		}
	}
	if err := restorer.Validate(ctx); err != nil {
		return errors.Wrap(err, "failed to validate backup existence")
	}
	return nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) createJob(ctx context.Context, job *batchv1.Job) error {
	err := r.client.Create(ctx, job)
	if err != nil {
		return errors.Wrap(err, "create job")
	}

	for {
		time.Sleep(time.Second * 1)

		checkJob := batchv1.Job{}
		err := r.client.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, &checkJob)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrap(err, "get job status")
		}
		for _, cond := range checkJob.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case batchv1.JobComplete:
				return nil
			case batchv1.JobFailed:
				return errors.New(cond.Message)
			}
		}
	}
}
