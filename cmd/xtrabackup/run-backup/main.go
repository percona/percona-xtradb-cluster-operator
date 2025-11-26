package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	xbscapi "github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	xbscserver "github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {

	req := getRequestObject()
	if req == nil {
		log.Fatal("Failed to get request object")
	}

	serverHost, ok := os.LookupEnv("HOST")
	if !ok {
		log.Fatalf("HOST environment variable is not set")
	}

	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", serverHost, xbscserver.DefaultPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Failed to connect to server: %w", err)
	}
	defer conn.Close()

	client := xbscapi.NewXtrabackupServiceClient(conn)

	stream, err := client.CreateBackup(context.Background(), req)
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			log.Fatal("Backup is already running")
		}
		log.Fatal("Failed to create backup: %w", err)
	}
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("Failed to receive response: %w", err)
		}
	}
	log.Println("Backup created successfully")
}

func getRequestObject() *xbscapi.CreateBackupRequest {
	req := &xbscapi.CreateBackupRequest{
		BackupConfig: &api.BackupConfig{},
	}

	req.BackupName = os.Getenv("BACKUP_NAME")
	storageType := os.Getenv("STORAGE_TYPE")
	switch storageType {
	case "s3":
		req.BackupConfig.Type = xbscapi.BackupStorageType_S3
		setS3Config(req)
	case "azure":
		req.BackupConfig.Type = xbscapi.BackupStorageType_AZURE
		setAzureConfig(req)
	default:
		log.Fatalf("Invalid storage type: %s", storageType)
	}
	return req
}

func setS3Config(req *xbscapi.CreateBackupRequest) {
	req.BackupConfig.S3 = &xbscapi.S3Config{
		Bucket:       os.Getenv("S3_BUCKET"),
		Region:       os.Getenv("DEFAULT_REGION"),
		EndpointUrl:  os.Getenv("ENDPOINT"),
		AccessKey:    os.Getenv("ACCESS_KEY_ID"),
		SecretKey:    os.Getenv("SECRET_ACCESS_KEY"),
		StorageClass: os.Getenv("S3_STORAGE_CLASS"),
	}
}

func setAzureConfig(req *xbscapi.CreateBackupRequest) {
	req.BackupConfig.Azure = &xbscapi.AzureConfig{
		ContainerName:  os.Getenv("AZURE_CONTAINER_NAME"),
		EndpointUrl:    os.Getenv("AZURE_ENDPOINT"),
		StorageClass:   os.Getenv("AZURE_STORAGE_CLASS"),
		StorageAccount: os.Getenv("AZURE_STORAGE_ACCOUNT"),
		AccessKey:      os.Getenv("AZURE_ACCESS_KEY"),
	}
}
