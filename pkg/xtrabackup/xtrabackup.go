package xtrabackup

import (
	"fmt"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
)

type XBCloudAction string

const (
	XBCloudActionPut    XBCloudAction = "put"
	XBCloudActionDelete XBCloudAction = "delete"
)

func XBCloudArgs(action XBCloudAction, conf *api.BackupConfig) []string {
	args := []string{string(action), "--parallel=10", "--curl-retriable-errors=7"}

	if !conf.VerifyTls {
		args = append(args, "--insecure")
	}

	if conf.ContainerOptions != nil && conf.ContainerOptions.Args != nil {
		args = append(args, conf.ContainerOptions.Args.Xbcloud...)
	}

	switch conf.Type {
	case api.BackupStorageType_GCS:
		args = append(
			args,
			[]string{
				"--md5",
				"--storage=google",
				fmt.Sprintf("--google-bucket=%s", conf.Gcs.Bucket),
				fmt.Sprintf("--google-access-key=%s", conf.Gcs.AccessKey),
				fmt.Sprintf("--google-secret-key=%s", conf.Gcs.SecretKey),
			}...,
		)
		if len(conf.Gcs.EndpointUrl) > 0 {
			args = append(args, fmt.Sprintf("--google-endpoint=%s", conf.Gcs.EndpointUrl))
		}
	case api.BackupStorageType_S3:
		args = append(
			args,
			[]string{
				"--md5",
				"--storage=s3",
				fmt.Sprintf("--s3-bucket=%s", conf.S3.Bucket),
				fmt.Sprintf("--s3-region=%s", conf.S3.Region),
				fmt.Sprintf("--s3-access-key=%s", conf.S3.AccessKey),
				fmt.Sprintf("--s3-secret-key=%s", conf.S3.SecretKey),
			}...,
		)
		if len(conf.S3.EndpointUrl) > 0 {
			args = append(args, fmt.Sprintf("--s3-endpoint=%s", conf.S3.EndpointUrl))
		}
	case api.BackupStorageType_AZURE:
		args = append(
			args,
			[]string{
				"--storage=azure",
				fmt.Sprintf("--azure-storage-account=%s", conf.Azure.StorageAccount),
				fmt.Sprintf("--azure-container-name=%s", conf.Azure.ContainerName),
				fmt.Sprintf("--azure-access-key=%s", conf.Azure.AccessKey),
			}...,
		)
		if len(conf.Azure.EndpointUrl) > 0 {
			args = append(args, fmt.Sprintf("--azure-endpoint=%s", conf.Azure.EndpointUrl))
		}
	}

	args = append(args, conf.Destination)

	return args
}
