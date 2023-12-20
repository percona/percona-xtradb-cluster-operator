package pxcrestore

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
)

func (r *ReconcilePerconaXtraDBClusterRestore) restore(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) (*batchv1.Job, error) {
	if cluster.Spec.Backup == nil {
		return nil, errors.New("undefined backup section in a cluster spec")
	}
	destination := bcp.Status.Destination
	switch {
	case strings.HasPrefix(bcp.Status.Destination, "pvc/"):
		job, err := r.restorePVC(cr, bcp, strings.TrimPrefix(destination, "pvc/"), cluster)
		return job, errors.Wrap(err, "pvc")
	case strings.HasPrefix(bcp.Status.Destination, api.AwsBlobStoragePrefix):
		job, err := r.restoreS3(cr, bcp, strings.TrimPrefix(destination, api.AwsBlobStoragePrefix), cluster, false)
		return job, errors.Wrap(err, "s3")
	case bcp.Status.Azure != nil:
		job, err := r.restoreAzure(cr, bcp, bcp.Status.Destination, cluster, false)
		return job, errors.Wrap(err, "azure")
	default:
		return nil, errors.Errorf("unknown backup storage type")
	}
}

func (r *ReconcilePerconaXtraDBClusterRestore) pitr(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) (*batchv1.Job, error) {
	dest := bcp.Status.Destination
	switch {
	case strings.HasPrefix(dest, api.AwsBlobStoragePrefix):
		job, err := r.restoreS3(cr, bcp, strings.TrimPrefix(dest, api.AwsBlobStoragePrefix), cluster, true)
		return job, errors.Wrap(err, "PITR restore s3")
	case bcp.Status.Azure != nil:
		job, err := r.restoreAzure(cr, bcp, bcp.Status.Destination, cluster, true)
		return job, errors.Wrap(err, "PITR restore azure")
	}
	return nil, errors.Errorf("unknown storage type")
}

func (r *ReconcilePerconaXtraDBClusterRestore) restorePVC(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, pvcName string, cluster *api.PerconaXtraDBCluster) (*batchv1.Job, error) {
	svc := backup.PVCRestoreService(cr)
	k8s.SetControllerReference(cr, svc, r.scheme)
	pod, err := backup.PVCRestorePod(cr, bcp.Status.StorageName, pvcName, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "restore pod")
	}
	k8s.SetControllerReference(cr, pod, r.scheme)

	job, err := backup.RestoreJob(cr, bcp, cluster, "", false)
	if err != nil {
		return nil, errors.Wrap(err, "restore job")
	}
	k8s.SetControllerReference(cr, job, r.scheme)

	r.client.Delete(context.TODO(), svc)
	r.client.Delete(context.TODO(), pod)

	err = r.client.Create(context.TODO(), svc)
	if err != nil {
		return nil, errors.Wrap(err, "create service")
	}
	err = r.client.Create(context.TODO(), pod)
	if err != nil {
		return nil, errors.Wrap(err, "create pod")
	}

	for {
		time.Sleep(time.Second * 1)

		err := r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil {
			return nil, errors.Wrap(err, "get pod status")
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
func (r *ReconcilePerconaXtraDBClusterRestore) restoreAzure(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, dest string, cluster *api.PerconaXtraDBCluster, pitr bool) (*batchv1.Job, error) {
	job, err := backup.RestoreJob(cr, bcp, cluster, dest, pitr)
	if err != nil {
		return nil, err
	}
	if err = k8s.SetControllerReference(cr, job, r.scheme); err != nil {
		return nil, err
	}

	return r.createJob(job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) restoreS3(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster *api.PerconaXtraDBCluster, pitr bool) (*batchv1.Job, error) {
	job, err := backup.RestoreJob(cr, bcp, cluster, s3dest, pitr)
	if err != nil {
		return nil, err
	}
	k8s.SetControllerReference(cr, job, r.scheme)

	return r.createJob(job)
}

func (r *ReconcilePerconaXtraDBClusterRestore) createJob(job *batchv1.Job) (*batchv1.Job, error) {
	err := r.client.Create(context.TODO(), job)
	if err != nil {
		return nil, errors.Wrap(err, "create job")
	}
	return job, nil

}
