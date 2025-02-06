package storage

import (
	"context"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Options interface {
	Type() api.BackupStorageType
}

func GetOptions(ctx context.Context, cl client.Client, cluster *api.PerconaXtraDBCluster, stgName string) (Options, error) {
	stg, ok := cluster.Spec.Backup.Storages[stgName]
	if !ok {
		return nil, errors.Errorf("unknown storage %s", stgName)
	}

	switch stg.Type {
	case api.BackupStorageS3:
		return getS3Options(ctx, cl, cluster, stg.S3, stg.VerifyTLS)
	case api.BackupStorageAzure:
		return getAzureOptions(ctx, cl, cluster, stg.Azure)
	default:
		return nil, errors.Errorf("unknown storage type %s", stg.Type)
	}
}

func GetOptionsFromBackup(ctx context.Context, cl client.Client, cluster *api.PerconaXtraDBCluster, backup *api.PerconaXtraDBClusterBackup) (Options, error) {
	switch {
	case backup.Status.S3 != nil:
		return getS3OptionsFromBackup(ctx, cl, cluster, backup)
	case backup.Status.Azure != nil:
		return getAzureOptionsFromBackup(ctx, cl, backup)
	default:
		return nil, errors.Errorf("unknown storage type %s", backup.Status.StorageType)
	}
}

func getAzureOptions(
	ctx context.Context,
	cl client.Client,
	cluster *api.PerconaXtraDBCluster,
	azure *api.BackupStorageAzureSpec,
) (*AzureOptions, error) {
	secret := new(corev1.Secret)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      azure.CredentialsSecret,
		Namespace: cluster.Namespace,
	}, secret)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}
	accountName := string(secret.Data["AZURE_STORAGE_ACCOUNT_NAME"])
	accountKey := string(secret.Data["AZURE_STORAGE_ACCOUNT_KEY"])

	container, prefix := azure.ContainerAndPrefix()
	if container == "" {
		return nil, errors.New("container name is not set")
	}

	return &AzureOptions{
		StorageAccount: accountName,
		AccessKey:      accountKey,
		Endpoint:       azure.Endpoint,
		Container:      container,
		Prefix:         prefix,
	}, nil
}

func getAzureOptionsFromBackup(ctx context.Context, cl client.Client, backup *api.PerconaXtraDBClusterBackup) (*AzureOptions, error) {
	secret := new(corev1.Secret)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      backup.Status.Azure.CredentialsSecret,
		Namespace: backup.Namespace,
	}, secret)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}
	accountName := string(secret.Data["AZURE_STORAGE_ACCOUNT_NAME"])
	accountKey := string(secret.Data["AZURE_STORAGE_ACCOUNT_KEY"])

	container, prefix := backup.Status.Azure.ContainerAndPrefix()
	if container == "" {
		container, prefix = backup.Status.Destination.BucketAndPrefix()
	}

	if container == "" {
		return nil, errors.New("container name is not set")
	}

	return &AzureOptions{
		StorageAccount: accountName,
		AccessKey:      accountKey,
		Endpoint:       backup.Status.Azure.Endpoint,
		Container:      container,
		Prefix:         prefix,
	}, nil
}

func getS3Options(
	ctx context.Context,
	cl client.Client,
	cluster *api.PerconaXtraDBCluster,
	s3 *api.BackupStorageS3Spec,
	verifyTLS *bool,
) (*S3Options, error) {
	secret := new(corev1.Secret)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      s3.CredentialsSecret,
		Namespace: cluster.Namespace,
	}, secret)
	if client.IgnoreNotFound(err) != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}

	accessKeyID := string(secret.Data["AWS_ACCESS_KEY_ID"])
	secretAccessKey := string(secret.Data["AWS_SECRET_ACCESS_KEY"])

	bucket, prefix := s3.BucketAndPrefix()
	if bucket == "" {
		return nil, errors.New("bucket name is not set")
	}

	region := s3.Region
	if region == "" {
		region = "us-east-1"
	}

	verify := true
	if verifyTLS != nil && !*verifyTLS {
		verify = false
	}

	return &S3Options{
		Endpoint:        s3.EndpointURL,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		BucketName:      bucket,
		Prefix:          prefix,
		Region:          region,
		VerifyTLS:       verify,
	}, nil
}

func getS3OptionsFromBackup(ctx context.Context, cl client.Client, cluster *api.PerconaXtraDBCluster, backup *api.PerconaXtraDBClusterBackup) (*S3Options, error) {
	secret := new(corev1.Secret)
	err := cl.Get(ctx, types.NamespacedName{
		Name:      backup.Status.S3.CredentialsSecret,
		Namespace: backup.Namespace,
	}, secret)
	if client.IgnoreNotFound(err) != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}
	accessKeyID := string(secret.Data["AWS_ACCESS_KEY_ID"])
	secretAccessKey := string(secret.Data["AWS_SECRET_ACCESS_KEY"])

	bucket, prefix := backup.Status.S3.BucketAndPrefix()
	if bucket == "" {
		bucket, prefix = backup.Status.Destination.BucketAndPrefix()
	}

	if bucket == "" {
		return nil, errors.New("bucket name is not set")
	}

	region := backup.Status.S3.Region
	if region == "" {
		region = "us-east-1"
	}

	verifyTLS := true
	if backup.Status.VerifyTLS != nil && !*backup.Status.VerifyTLS {
		verifyTLS = false
	}
	if cluster != nil && cluster.Spec.Backup != nil && len(cluster.Spec.Backup.Storages) > 0 {
		storage, ok := cluster.Spec.Backup.Storages[backup.Spec.StorageName]
		if ok && storage.VerifyTLS != nil {
			verifyTLS = *storage.VerifyTLS
		}
	}

	return &S3Options{
		Endpoint:        backup.Status.S3.EndpointURL,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		BucketName:      bucket,
		Prefix:          prefix,
		Region:          region,
		VerifyTLS:       verifyTLS,
	}, nil
}

var _ = Options(new(S3Options))

type S3Options struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	Prefix          string
	Region          string
	VerifyTLS       bool
}

func (o *S3Options) Type() api.BackupStorageType {
	return api.BackupStorageS3
}

var _ = Options(new(AzureOptions))

type AzureOptions struct {
	StorageAccount string
	AccessKey      string
	Endpoint       string
	Container      string
	Prefix         string
}

func (o *AzureOptions) Type() api.BackupStorageType {
	return api.BackupStorageAzure
}
