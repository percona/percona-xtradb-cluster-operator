package pxcrestore

import (
	"context"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
)

func (r *ReconcilePerconaXtraDBClusterRestore) restore(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster api.PerconaXtraDBClusterSpec) error {
	if cluster.Backup == nil {
		return errors.New("undefined backup section in a cluster spec")
	}
	if len(bcp.Status.Destination) > 6 {
		switch {
		case bcp.Status.Destination[:4] == "pvc/":
			return errors.Wrap(r.restorePVC(cr, bcp, bcp.Status.Destination[4:], cluster), "pvc")
		case bcp.Status.Destination[:5] == "s3://":
			return errors.Wrap(r.restoreS3(cr, bcp, bcp.Status.Destination[5:], cluster, false), "s3")
		}
	}

	return errors.Errorf("unknown destination %s", bcp.Status.Destination)
}

func (r *ReconcilePerconaXtraDBClusterRestore) pitr(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster api.PerconaXtraDBClusterSpec) error {
	if cr.Spec.PITR != nil {
		return errors.Wrap(r.restoreS3(cr, bcp, bcp.Status.Destination[5:], cluster, true), "PITR restore")
	}

	return nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) restorePVC(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, pvcName string, cluster api.PerconaXtraDBClusterSpec) error {
	svc := backup.PVCRestoreService(cr)
	k8s.SetControllerReference(cr, svc, r.scheme)
	pod, err := backup.PVCRestorePod(cr, bcp.Status.StorageName, pvcName, cluster)
	if err != nil {
		return errors.Wrap(err, "restore pod")
	}
	k8s.SetControllerReference(cr, pod, r.scheme)

	job, err := backup.PVCRestoreJob(cr, cluster)
	if err != nil {
		return errors.Wrap(err, "restore job")
	}
	k8s.SetControllerReference(cr, job, r.scheme)

	r.client.Delete(context.TODO(), svc)
	r.client.Delete(context.TODO(), pod)

	err = r.client.Create(context.TODO(), svc)
	if err != nil {
		return errors.Wrap(err, "create service")
	}
	err = r.client.Create(context.TODO(), pod)
	if err != nil {
		return errors.Wrap(err, "create pod")
	}

	for {
		time.Sleep(time.Second * 1)

		err := r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil {
			return errors.Wrap(err, "get pod status")
		}
		if pod.Status.Phase == corev1.PodRunning {
			break
		}
	}

	defer func() {
		r.client.Delete(context.TODO(), svc)
		r.client.Delete(context.TODO(), pod)
	}()

	return r.createJob(job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) restoreS3(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster api.PerconaXtraDBClusterSpec, pitr bool) error {
	job, err := backup.S3RestoreJob(cr, bcp, s3dest, cluster, pitr)
	if err != nil {
		return err
	}
	k8s.SetControllerReference(cr, job, r.scheme)

	return r.createJob(job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) createJob(job *batchv1.Job) error {
	err := r.client.Create(context.TODO(), job)
	if err != nil {
		return errors.Wrap(err, "create job")
	}

	for {
		time.Sleep(time.Second * 1)

		checkJob := batchv1.Job{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, &checkJob)
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Wrap(err, "get job status")
		}
		for _, cond := range checkJob.Status.Conditions {
			if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
				return nil
			}
		}
	}
}
