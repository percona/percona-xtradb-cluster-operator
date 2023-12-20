package pxcrestore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

type Restorer interface {
	Init(ctx context.Context) error
	Job() (*batchv1.Job, error)
	PITRJob() (*batchv1.Job, error)
	Finalize(ctx context.Context) error
	Validate(ctx context.Context) error
	ValidateJob(ctx context.Context, job *batchv1.Job) error
}

type s3 struct{ *restorerOptions }

func (s *s3) Init(context.Context) error     { return nil }
func (s *s3) Finalize(context.Context) error { return nil }

func (s *s3) Job() (*batchv1.Job, error) {
	return backup.RestoreJob(s.cr, s.bcp, s.cluster, strings.TrimPrefix(s.bcp.Status.Destination, api.AwsBlobStoragePrefix), false)
}

func (s *s3) PITRJob() (*batchv1.Job, error) {
	return backup.RestoreJob(s.cr, s.bcp, s.cluster, strings.TrimPrefix(s.bcp.Status.Destination, api.AwsBlobStoragePrefix), true)
}

func (s *s3) ValidateJob(ctx context.Context, job *batchv1.Job) error {
	if s.bcp.Status.S3.CredentialsSecret == "" {
		// Skip validation if the credentials secret isn't set.
		// This allows authentication via IAM roles.
		// More info: https://github.com/percona/k8spxc-docs/blob/87f98e6ddae8114474836c0610155d05d3531e03/docs/backups-storage.md?plain=1#L116-L126
		return nil
	}

	return s.restorerOptions.ValidateJob(ctx, job)
}

func (s *s3) Validate(ctx context.Context) error {
	sec := corev1.Secret{}
	err := s.k8sClient.Get(ctx,
		types.NamespacedName{Name: s.bcp.Status.S3.CredentialsSecret, Namespace: s.bcp.Namespace}, &sec)
	if client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "failed to get secret")
	}

	accessKeyID := string(sec.Data["AWS_ACCESS_KEY_ID"])
	secretAccessKey := string(sec.Data["AWS_SECRET_ACCESS_KEY"])
	ep := s.bcp.Status.S3.EndpointURL
	bucket, prefix := s.bcp.Status.S3.BucketAndPrefix()
	verifyTLS := true
	if s.bcp.Status.VerifyTLS != nil && !*s.bcp.Status.VerifyTLS {
		verifyTLS = false
	}
	if s.cluster.Spec.Backup != nil && len(s.cluster.Spec.Backup.Storages) > 0 {
		storage, ok := s.cluster.Spec.Backup.Storages[s.bcp.Spec.StorageName]
		if ok && storage.VerifyTLS != nil {
			verifyTLS = *storage.VerifyTLS
		}
	}
	s3cli, err := s.newStorageClient(&storage.S3Options{
		Endpoint:        ep,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		BucketName:      bucket,
		Prefix:          prefix,
		Region:          s.bcp.Status.S3.Region,
		VerifyTLS:       verifyTLS,
	})
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

type pvc struct{ *restorerOptions }

func (s *pvc) Validate(ctx context.Context) error {
	destination := s.bcp.Status.Destination

	pod, err := backup.PVCRestorePod(s.cr, s.bcp.Status.StorageName, strings.TrimPrefix(destination, "pvc/"), s.cluster)
	if err != nil {
		return errors.Wrap(err, "restore pod")
	}
	if err := k8s.SetControllerReference(s.cr, pod, s.scheme); err != nil {
		return err
	}
	pod.Name += "-verify"
	pod.Spec.Containers[0].Command = []string{"bash", "-c", `[[ $(stat -c%s /backup/xtrabackup.stream) -gt 5000000 ]]`}
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever

	if err := s.k8sClient.Delete(ctx, pod); client.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "failed to delete")
	}

	if err := s.k8sClient.Create(ctx, pod); err != nil {
		return errors.Wrap(err, "failed to create pod")
	}
	for {
		time.Sleep(time.Second * 1)

		err := s.k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil {
			return errors.Wrap(err, "get pod status")
		}
		if pod.Status.Phase == corev1.PodFailed {
			return errors.Errorf("backup files not found on %s", destination)
		}
		if pod.Status.Phase == corev1.PodSucceeded {
			break
		}
	}

	return nil
}

func (s *pvc) Job() (*batchv1.Job, error) {
	return backup.RestoreJob(s.cr, s.bcp, s.cluster, "", false)
}

func (s *pvc) PITRJob() (*batchv1.Job, error) {
	return nil, errors.New("pitr restore is not supported for pvc")
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

type azure struct{ *restorerOptions }

func (s *azure) Init(context.Context) error     { return nil }
func (s *azure) Finalize(context.Context) error { return nil }

func (s *azure) Job() (*batchv1.Job, error) {
	return backup.RestoreJob(s.cr, s.bcp, s.cluster, s.bcp.Status.Destination, false)
}

func (s *azure) PITRJob() (*batchv1.Job, error) {
	return backup.RestoreJob(s.cr, s.bcp, s.cluster, s.bcp.Status.Destination, true)
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
	azurecli, err := s.newStorageClient(&storage.AzureOptions{
		StorageAccount: accountName,
		AccessKey:      accountKey,
		Endpoint:       endpoint,
		Container:      container,
		Prefix:         prefix,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create s3 client")
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

func (r *ReconcilePerconaXtraDBClusterRestore) getRestorer(
	cr *api.PerconaXtraDBClusterRestore,
	bcp *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
) (Restorer, error) {
	s := restorerOptions{
		cr:               cr,
		bcp:              bcp,
		cluster:          cluster,
		k8sClient:        r.client,
		scheme:           r.scheme,
		newStorageClient: r.newStorageClientFunc,
	}
	switch {
	case strings.HasPrefix(s.bcp.Status.Destination, "pvc/"):
		sr := pvc{&s}
		return &sr, nil
	case strings.HasPrefix(s.bcp.Status.Destination, api.AwsBlobStoragePrefix):
		sr := s3{&s}
		return &sr, nil
	case s.bcp.Status.Azure != nil:
		sr := azure{&s}
		return &sr, nil
	}
	return nil, errors.Errorf("unknown backup storage type")
}

type restorerOptions struct {
	cr               *api.PerconaXtraDBClusterRestore
	bcp              *api.PerconaXtraDBClusterBackup
	cluster          *api.PerconaXtraDBCluster
	k8sClient        client.Client
	scheme           *runtime.Scheme
	newStorageClient storage.NewClientFunc
}

func (opts *restorerOptions) ValidateJob(ctx context.Context, job *batchv1.Job) error {
	cl := opts.k8sClient

	secrets := []string{}
	for _, container := range job.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name != "" {
				secrets = append(secrets, env.ValueFrom.SecretKeyRef.Name)
			}
		}
	}

	notExistingSecrets := make(map[string]struct{})
	for _, secret := range secrets {
		err := cl.Get(ctx, types.NamespacedName{
			Name:      secret,
			Namespace: job.Namespace,
		}, new(corev1.Secret))
		if err != nil {
			if k8serrors.IsNotFound(err) {
				notExistingSecrets[secret] = struct{}{}
				continue
			}
			return err
		}
	}
	if len(notExistingSecrets) > 0 {
		secrets := make([]string, 0, len(notExistingSecrets))
		for k := range notExistingSecrets {
			secrets = append(secrets, k)
		}
		sort.StringSlice(secrets).Sort()
		return errors.Errorf("secrets %s not found", strings.Join(secrets, ", "))
	}

	return nil
}
