package pxcrestore

import (
	"context"
	"strings"
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

	storageRestore, err := r.getStorageRestore(cr, bcp, cluster, false)
	if err != nil {
		return errors.Wrap(err, "failed to get storage")
	}
	job, err := storageRestore.Job()
	if err != nil {
		return errors.Wrap(err, "failed to get restore job")
	}
	if err = k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return err
	}

	if err = storageRestore.Init(ctx); err != nil {
		return errors.Wrap(err, "failed to init restore")
	}
	defer func() {
		if derr := storageRestore.Finalize(ctx); derr != nil {
			log.Error(derr, "failed to finalize restore")
		}
	}()

	return r.createJob(ctx, job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) pitr(ctx context.Context, cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	storageRestore, err := r.getStorageRestore(cr, bcp, cluster, true)
	if err != nil {
		return errors.Wrap(err, "failed to get storage")
	}
	job, err := storageRestore.Job()
	if err != nil {
		return errors.Wrap(err, "failed to create pitr restore job")
	}
	if err := k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return err
	}
	if err = storageRestore.Init(ctx); err != nil {
		return errors.Wrap(err, "failed to init storage")
	}
	defer func() {
		if derr := storageRestore.Finalize(ctx); derr != nil {
			log.Error(derr, "failed to finalize restore")
		}
	}()

	return r.createJob(ctx, job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) validate(ctx context.Context, cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) error {
	storageRestore, err := r.getStorageRestore(cr, bcp, cluster, false)
	if err != nil {
		return errors.Wrap(err, "failed to get storage")
	}
	job, err := storageRestore.Job()
	if err != nil {
		return errors.Wrap(err, "failed to create pitr restore job")
	}
	if err := r.validateJob(ctx, job); err != nil {
		return errors.Wrap(err, "failed to validate job")
	}

	if cr.Spec.PITR != nil {
		storageRestore, err := r.getStorageRestore(cr, bcp, cluster, true)
		if err != nil {
			return errors.Wrap(err, "failed to get storage")
		}
		job, err := storageRestore.Job()
		if err != nil {
			return errors.Wrap(err, "failed to create pitr restore job")
		}
		if err := r.validateJob(ctx, job); err != nil {
			return errors.Wrap(err, "failed to validate job")
		}
	}

	if err := storageRestore.Validate(ctx); err != nil {
		return errors.Wrap(err, "failed to validate backup existence")
	}
	return nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) validateJob(ctx context.Context, job *batchv1.Job) error {
	secrets := []string{}
	for _, container := range job.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name != "" {
				secrets = append(secrets, env.ValueFrom.SecretKeyRef.Name)
			}
		}
	}

	notExistingSecrets := []string{}
	for _, secret := range secrets {
		err := r.client.Get(ctx, types.NamespacedName{
			Name:      secret,
			Namespace: job.Namespace,
		}, new(corev1.Secret))
		if err != nil {
			if k8serrors.IsNotFound(err) {
				notExistingSecrets = append(notExistingSecrets, secret)
				continue
			}
			return err
		}
	}
	if len(notExistingSecrets) > 0 {
		return errors.Errorf("secrets %s not found", strings.Join(notExistingSecrets, ", "))
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
