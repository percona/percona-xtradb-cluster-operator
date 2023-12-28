package storage

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

type Options interface {
	Type() api.BackupStorageType
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
