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

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
)

func (r *ReconcilePerconaXtraDBClusterRestore) restore(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) error {
	if cluster.Spec.Backup == nil {
		return errors.New("undefined backup section in a cluster spec")
	}
	destination := bcp.Status.Destination
	switch {
	case strings.HasPrefix(bcp.Status.Destination, "pvc/"):
		return errors.Wrap(r.restorePVC(cr, bcp, strings.TrimPrefix(destination, "pvc/"), cluster.Spec), "pvc")
	case strings.HasPrefix(bcp.Status.Destination, "s3://"):
		return errors.Wrap(r.restoreS3(cr, bcp, strings.TrimPrefix(destination, "s3://"), cluster, false), "s3")
	case bcp.Status.Azure != nil:
		return errors.Wrap(r.restoreAzure(cr, bcp, bcp.Status.Destination, cluster.Spec, false), "azure")
	default:
		return errors.Errorf("unknown backup storage type")
	}
}

func (r *ReconcilePerconaXtraDBClusterRestore) pitr(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) error {
	dest := bcp.Status.Destination
	switch {
	case strings.HasPrefix(dest, "s3://"):
		return errors.Wrap(r.restoreS3(cr, bcp, strings.TrimPrefix(dest, "s3://"), cluster, true), "PITR restore s3")
	case bcp.Status.Azure != nil:
		return errors.Wrap(r.restoreAzure(cr, bcp, bcp.Status.Destination, cluster.Spec, true), "PITR restore azure")
	}
	return errors.Errorf("unknown storage type")
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
func (r *ReconcilePerconaXtraDBClusterRestore) restoreAzure(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, dest string, cluster api.PerconaXtraDBClusterSpec, pitr bool) error {
	job, err := backup.AzureRestoreJob(cr, bcp, cluster, dest, pitr)
	if err != nil {
		return err
	}
	if err = k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return err
	}

	return r.createJob(job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) restoreS3(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster *api.PerconaXtraDBCluster, pitr bool) error {
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
