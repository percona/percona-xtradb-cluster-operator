package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	req, err := parseFlags()
	if err != nil {
		log.Fatal("Failed to parse flags: %w", err)
	}

	serverHost, ok := os.LookupEnv("HOST")
	if !ok {
		log.Fatalf("HOST environment variable is not set")
	}

	conn, err := grpc.NewClient(serverHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Failed to connect to server: %w", err)
	}
	defer conn.Close()

	client := api.NewXtrabackupServiceClient(conn)

	_, err = client.CreateBackup(context.Background(), req)
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			log.Fatal("Backup is already running")
		}
		log.Fatal("Failed to create backup: %w", err)
	}
	log.Println("Backup created successfully")
}

func parseFlags() (*api.CreateBackupRequest, error) {
	var (
		request = &api.CreateBackupRequest{
			BackupConfig: &api.BackupConfig{
				S3:    &api.S3Config{},
				Gcs:   &api.GCSConfig{},
				Azure: &api.AzureConfig{},
				ContainerOptions: &api.ContainerOptions{
					Env: []*api.EnvVar{},
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{},
					},
				},
			},
		}
		backupType string
	)

	// Backup name
	flag.StringVar(&request.BackupName, "backup-name", "", "Name of the backup")

	// BackupConfig fields
	flag.StringVar(&request.BackupConfig.Destination, "destination", "", "Backup destination path")
	flag.BoolVar(&request.BackupConfig.VerifyTls, "verify-tls", true, "Verify TLS certificates")
	flag.StringVar(&backupType, "type", "", "Storage type: s3, azure, or gcs")
	switch backupType {
	case "s3":
		request.BackupConfig.Type = api.BackupStorageType_S3
	case "azure":
		request.BackupConfig.Type = api.BackupStorageType_AZURE
	case "gcs":
		request.BackupConfig.Type = api.BackupStorageType_GCS
	default:
		return nil, fmt.Errorf("invalid storage type: %s", backupType)
	}

	// S3Config fields
	flag.StringVar(&request.BackupConfig.S3.Bucket, "s3.bucket", "", "S3 bucket name")
	flag.StringVar(&request.BackupConfig.S3.Region, "s3.region", "", "S3 region")
	flag.StringVar(&request.BackupConfig.S3.EndpointUrl, "s3.endpoint", "", "S3 endpoint URL")
	flag.StringVar(&request.BackupConfig.S3.AccessKey, "s3.access-key", "", "S3 access key")
	flag.StringVar(&request.BackupConfig.S3.SecretKey, "s3.secret-key", "", "S3 secret key")
	flag.StringVar(&request.BackupConfig.S3.StorageClass, "s3.storage-class", "", "S3 storage class")

	// GCSConfig fields
	flag.StringVar(&request.BackupConfig.Gcs.Bucket, "gcs.bucket", "", "GCS bucket name")
	flag.StringVar(&request.BackupConfig.Gcs.EndpointUrl, "gcs.endpoint", "", "GCS endpoint URL")
	flag.StringVar(&request.BackupConfig.Gcs.StorageClass, "gcs.storage-class", "", "GCS storage class")
	flag.StringVar(&request.BackupConfig.Gcs.AccessKey, "gcs.access-key", "", "GCS access key")
	flag.StringVar(&request.BackupConfig.Gcs.SecretKey, "gcs.secret-key", "", "GCS secret key")

	// AzureConfig fields
	flag.StringVar(&request.BackupConfig.Azure.ContainerName, "azure.container", "", "Azure container name")
	flag.StringVar(&request.BackupConfig.Azure.EndpointUrl, "azure.endpoint", "", "Azure endpoint URL")
	flag.StringVar(&request.BackupConfig.Azure.StorageClass, "azure.storage-class", "", "Azure storage class")
	flag.StringVar(&request.BackupConfig.Azure.StorageAccount, "azure.storage-account", "", "Azure storage account")
	flag.StringVar(&request.BackupConfig.Azure.AccessKey, "azure.access-key", "", "Azure access key")

	// ContainerOptions - environment variables (format: KEY=VALUE,KEY2=VALUE2)
	var envVars string
	flag.StringVar(&envVars, "env", "", "Environment variables as comma-separated KEY=VALUE pairs")
	// ContainerOptions - xtrabackup args (comma-separated)
	var xtrabackupArgs string
	flag.StringVar(&xtrabackupArgs, "xtrabackup-args", "", "Xtrabackup arguments (comma-separated)")
	var xbcloudArgs string
	flag.StringVar(&xbcloudArgs, "xbcloud-args", "", "Xbcloud arguments (comma-separated)")
	var xbstreamArgs string
	flag.StringVar(&xbstreamArgs, "xbstream-args", "", "Xbstream arguments (comma-separated)")

	flag.Parse()

	// Parse ContainerOptions after flag parsing
	if envVars != "" || xtrabackupArgs != "" || xbcloudArgs != "" || xbstreamArgs != "" {
		request.BackupConfig.ContainerOptions = &api.ContainerOptions{}

		// Parse environment variables
		if envVars != "" {
			pairs := strings.Split(envVars, ",")
			for _, pair := range pairs {
				parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
				if len(parts) == 2 {
					request.BackupConfig.ContainerOptions.Env = append(
						request.BackupConfig.ContainerOptions.Env,
						&api.EnvVar{
							Key:   strings.TrimSpace(parts[0]),
							Value: strings.TrimSpace(parts[1]),
						},
					)
				}
			}
		}

		// Parse container args
		if xtrabackupArgs != "" || xbcloudArgs != "" || xbstreamArgs != "" {
			request.BackupConfig.ContainerOptions.Args = &api.BackupContainerArgs{}

			if xtrabackupArgs != "" {
				parts := strings.Split(xtrabackupArgs, ",")
				request.BackupConfig.ContainerOptions.Args.Xtrabackup = make([]string, len(parts))
				for i, part := range parts {
					request.BackupConfig.ContainerOptions.Args.Xtrabackup[i] = strings.TrimSpace(part)
				}
			}

			if xbcloudArgs != "" {
				parts := strings.Split(xbcloudArgs, ",")
				request.BackupConfig.ContainerOptions.Args.Xbcloud = make([]string, len(parts))
				for i, part := range parts {
					request.BackupConfig.ContainerOptions.Args.Xbcloud[i] = strings.TrimSpace(part)
				}
			}

			if xbstreamArgs != "" {
				parts := strings.Split(xbstreamArgs, ",")
				request.BackupConfig.ContainerOptions.Args.Xbstream = make([]string, len(parts))
				for i, part := range parts {
					request.BackupConfig.ContainerOptions.Args.Xbstream[i] = strings.TrimSpace(part)
				}
			}
		}
	}

	// Clean up empty nested configs
	switch request.BackupConfig.Type {
	case api.BackupStorageType_S3:
		request.BackupConfig.Azure = nil
		request.BackupConfig.Gcs = nil
	case api.BackupStorageType_AZURE:
		request.BackupConfig.S3 = nil
		request.BackupConfig.Gcs = nil
	case api.BackupStorageType_GCS:
		request.BackupConfig.S3 = nil
		request.BackupConfig.Azure = nil
	}
	return request, nil
}
