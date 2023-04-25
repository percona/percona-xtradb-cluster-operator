package pxcrestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

type StorageRestore interface {
	Init(ctx context.Context) error
	Job() (*batchv1.Job, error)
	Finalize(ctx context.Context) error
	Validate(ctx context.Context) error
}

type s3 struct {
	cr        *api.PerconaXtraDBClusterRestore
	bcp       *api.PerconaXtraDBClusterBackup
	cluster   *api.PerconaXtraDBCluster
	k8sClient client.Client
	pitr      bool
}

func (s *s3) Job() (*batchv1.Job, error) {
	return backup.S3RestoreJob(s.cr, s.bcp, strings.TrimPrefix(s.bcp.Status.Destination, api.AwsBlobStoragePrefix), s.cluster, s.pitr)
}

func (s *s3) Validate(ctx context.Context) error {
	sec := corev1.Secret{}
	err := s.k8sClient.Get(ctx,
		types.NamespacedName{Name: s.bcp.Status.S3.CredentialsSecret, Namespace: s.bcp.Namespace}, &sec)
	if err != nil {
		return errors.Wrap(err, "failed to get secret")
	}

	accessKeyID := string(sec.Data["AWS_ACCESS_KEY_ID"])
	secretAccessKey := string(sec.Data["AWS_SECRET_ACCESS_KEY"])
	ep := s.bcp.Status.S3.EndpointURL
	bucket, prefix := s.bcp.Status.S3.BucketAndPrefix()
	verifyTLS := true
	if s.cluster.Spec.Backup != nil && len(s.cluster.Spec.Backup.Storages) > 0 {
		storage, ok := s.cluster.Spec.Backup.Storages[s.bcp.Spec.StorageName]
		if ok && storage.VerifyTLS != nil {
			verifyTLS = *storage.VerifyTLS
		}
	}
	s3cli, err := storage.NewS3(ep, accessKeyID, secretAccessKey, bucket, prefix, s.bcp.Status.S3.Region, verifyTLS)
	if err != nil {
		return errors.Wrap(err, "failed to create s3 client")
	}
	dest := s.bcp.Status.Destination
	dest = strings.TrimPrefix(dest, api.AwsBlobStoragePrefix)
	dest = strings.TrimPrefix(dest, bucket+"/")
	if prefix != "" {
		dest = strings.TrimPrefix(dest, prefix)
		dest = strings.TrimPrefix(dest, "/")
	}
	dest = strings.TrimSuffix(dest, "/") + "/"

	objs, err := s3cli.ListObjects(ctx, dest)
	if err != nil {
		return errors.Wrap(err, "failed to list objects")
	}
	if len(objs) == 0 {
		return errors.New("backup not found")
	}

	return nil
}

func (s *s3) Init(ctx context.Context) error {
	return nil
}

func (s *s3) Finalize(ctx context.Context) error {
	return nil
}

type pvc struct {
	cr        *api.PerconaXtraDBClusterRestore
	cluster   *api.PerconaXtraDBCluster
	bcp       *api.PerconaXtraDBClusterBackup
	scheme    *runtime.Scheme
	k8sClient client.Client
}

func (s *pvc) Job() (*batchv1.Job, error) {
	return backup.PVCRestoreJob(s.cr, s.cluster, s.bcp)
}

func (s *pvc) Validate(ctx context.Context) error {
	return nil
}

func (s *pvc) Init(ctx context.Context) error {
	destination := s.bcp.Status.Destination

	svc := backup.PVCRestoreService(s.cr)
	if err := k8s.SetControllerReference(s.cr, svc, s.scheme); err != nil {
		return err
	}
	pod, err := backup.PVCRestorePod(s.cr, s.bcp.Status.StorageName, strings.TrimPrefix(destination, "pvc/"), s.cluster)
	if err != nil {
		return errors.Wrap(err, "restore pod")
	}
	if err := k8s.SetControllerReference(s.cr, pod, s.scheme); err != nil {
		return err
	}
	if err := s.k8sClient.Delete(ctx, svc); client.IgnoreNotFound(err) != nil {
		return err
	}
	if err := s.k8sClient.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
		return err
	}

	err = s.k8sClient.Create(ctx, svc)
	if err != nil {
		return errors.Wrap(err, "create service")
	}
	err = s.k8sClient.Create(ctx, pod)
	if err != nil {
		return errors.Wrap(err, "create pod")
	}
	for {
		time.Sleep(time.Second * 1)

		err := s.k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil {
			return errors.Wrap(err, "get pod status")
		}
		if pod.Status.Phase == corev1.PodRunning {
			break
		}
	}
	return nil
}

func (s *pvc) Finalize(ctx context.Context) error {
	svc := backup.PVCRestoreService(s.cr)
	if err := s.k8sClient.Delete(ctx, svc); err != nil {
		return errors.Wrap(err, "failed to delete pvc service")
	}
	pod, err := backup.PVCRestorePod(s.cr, s.bcp.Status.StorageName, strings.TrimPrefix(s.bcp.Status.Destination, "pvc/"), s.cluster)
	if err != nil {
		return err
	}
	if err := s.k8sClient.Delete(ctx, pod); err != nil {
		return errors.Wrap(err, "failed to delete pvc pod")
	}
	return nil
}

type azure struct {
	cr        *api.PerconaXtraDBClusterRestore
	bcp       *api.PerconaXtraDBClusterBackup
	cluster   *api.PerconaXtraDBCluster
	k8sClient client.Client
	pitr      bool
}

func (s *azure) Job() (*batchv1.Job, error) {
	return backup.AzureRestoreJob(s.cr, s.bcp, s.cluster, s.bcp.Status.Destination, s.pitr)
}

func (s *azure) Validate(ctx context.Context) error {
	secret := new(corev1.Secret)
	err := s.k8sClient.Get(ctx, types.NamespacedName{Name: s.bcp.Status.Azure.CredentialsSecret, Namespace: s.bcp.Namespace}, secret)
	if err != nil {
		return errors.Wrap(err, "failed to get secret")
	}
	accountName := string(secret.Data["AZURE_STORAGE_ACCOUNT_NAME"])
	accountKey := string(secret.Data["AZURE_STORAGE_ACCOUNT_KEY"])

	endpoint := fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	if s.bcp.Status.Azure.Endpoint != "" {
		endpoint = s.bcp.Status.Azure.Endpoint
	}
	container, prefix := s.bcp.Status.Azure.ContainerAndPrefix()
	azurecli, err := storage.NewAzure(accountName, accountKey, endpoint, container, prefix)
	if err != nil {
		return errors.Wrap(err, "new azure client")
	}

	dest := s.bcp.Status.Destination
	dest = strings.TrimPrefix(dest, api.AzureBlobStoragePrefix)
	dest = strings.TrimPrefix(dest, container+"/")
	if prefix != "" {
		dest = strings.TrimPrefix(dest, prefix)
		dest = strings.TrimPrefix(dest, "/")
	}
	dest = strings.TrimSuffix(dest, "/") + "/"

	blobs, err := azurecli.ListObjects(ctx, dest)
	if err != nil {
		return errors.Wrap(err, "list blobs")
	}

	if len(blobs) == 0 {
		return errors.New("no backups found")
	}
	return nil
}

func (s *azure) Init(ctx context.Context) error {
	return nil
}

func (s *azure) Finalize(ctx context.Context) error {
	return nil
}

func (r *ReconcilePerconaXtraDBClusterRestore) getStorageRestore(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster, pitr bool) (StorageRestore, error) {
	switch {
	case strings.HasPrefix(bcp.Status.Destination, "pvc/") && !pitr:
		return &pvc{
			cr:        cr,
			bcp:       bcp,
			cluster:   cluster,
			scheme:    r.scheme,
			k8sClient: r.client,
		}, nil
	case strings.HasPrefix(bcp.Status.Destination, api.AwsBlobStoragePrefix):
		return &s3{
			cr:        cr,
			bcp:       bcp,
			cluster:   cluster,
			pitr:      pitr,
			k8sClient: r.client,
		}, nil
	case bcp.Status.Azure != nil:
		return &azure{
			cr:        cr,
			bcp:       bcp,
			cluster:   cluster,
			pitr:      pitr,
			k8sClient: r.client,
		}, nil
	}
	return nil, errors.Errorf("unknown backup storage type")
}
