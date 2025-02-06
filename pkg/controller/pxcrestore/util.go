package pxcrestore

import (
	"context"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func getBackup(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBClusterRestore) (*api.PerconaXtraDBClusterBackup, error) {
	if cr.Spec.BackupSource != nil {
		status := cr.Spec.BackupSource.DeepCopy()
		status.State = api.BackupSucceeded
		status.CompletedAt = nil
		status.LastScheduled = nil
		return &api.PerconaXtraDBClusterBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
			Spec: api.PXCBackupSpec{
				PXCCluster:  cr.Spec.PXCCluster,
				StorageName: cr.Spec.BackupSource.StorageName,
			},
			Status: *status,
		}, nil
	}

	bcp := &api.PerconaXtraDBClusterBackup{}
	if err := cl.Get(ctx, types.NamespacedName{Name: cr.Spec.BackupName, Namespace: cr.Namespace}, bcp); err != nil {
		return bcp, errors.Wrapf(err, "get backup %s", cr.Spec.BackupName)
	}
	if bcp.Status.State != api.BackupSucceeded {
		return bcp, errors.Errorf("backup %s didn't finished yet, current state: %s", bcp.Name, bcp.Status.State)
	}

	return bcp, nil
}

func setStatus(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBClusterRestore) error {
	if cr.Status.State == api.RestoreSucceeded {
		tm := metav1.NewTime(time.Now())
		cr.Status.CompletedAt = &tm
	}

	err := cl.Status().Update(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "send update")
	}

	return nil
}

func isOtherRestoreInProgress(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBClusterRestore) (*api.PerconaXtraDBClusterRestore, error) {
	rJobsList := &api.PerconaXtraDBClusterRestoreList{}
	if err := cl.List(
		ctx,
		rJobsList,
		&client.ListOptions{
			Namespace: cr.Namespace,
		},
	); err != nil {
		return nil, errors.Wrap(err, "get restore jobs list")
	}

	for _, j := range rJobsList.Items {
		if j.Spec.PXCCluster == cr.Spec.PXCCluster &&
			j.Name != cr.Name && j.Status.State != api.RestoreFailed &&
			j.Status.State != api.RestoreSucceeded {
			return &j, nil
		}
	}
	return nil, nil
}

func isJobFinished(checkJob *batchv1.Job) (bool, error) {
	for _, c := range checkJob.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}

		switch c.Type {
		case batchv1.JobComplete:
			return true, nil
		case batchv1.JobFailed:
			return false, errors.Errorf("job %s failed: %s", checkJob.Name, c.Message)
		}
	}
	return false, nil
}
