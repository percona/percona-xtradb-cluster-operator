package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

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

	connUrl := fmt.Sprintf("%s:%d", serverHost, xbscserver.DefaultPort)
	conn, err := grpc.NewClient(connUrl, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	log.Printf("Created connection to server at %s", connUrl)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	client := xbscapi.NewXtrabackupServiceClient(conn)

	defer printLogs(ctx, req.BackupName, client)

	stream, err := client.CreateBackup(ctx, req)
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			log.Fatal("Backup is already running")
		}
		log.Fatal("Failed to create backup: %w", err)
	}

	log.Println("Backup requested")
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

func printLogs(ctx context.Context, backupName string, client xbscapi.XtrabackupServiceClient) {
	stream, err := client.GetLogs(ctx, &xbscapi.GetLogsRequest{
		BackupName: backupName,
	})
	if err != nil {
		log.Fatal("Failed to get logs: %w", err)
	}
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("Failed to receive log chunk: %w", err)
		}
		fmt.Fprint(os.Stdout, chunk.Log)
	}
}

func getRequestObject() *xbscapi.CreateBackupRequest {
	req := &xbscapi.CreateBackupRequest{
		BackupConfig: &api.BackupConfig{},
	}

	containerOptions := &xbscapi.ContainerOptions{}
	if opts := os.Getenv("CONTAINER_OPTIONS"); opts != "" {
		err := json.Unmarshal([]byte(opts), containerOptions)
		if err != nil {
			log.Fatalf("Failed to unmarshal container options: %v", err)
		}
	}

	req.BackupName = os.Getenv("BACKUP_NAME")
	req.BackupConfig.Destination = os.Getenv("BACKUP_DEST")
	req.BackupConfig.VerifyTls = os.Getenv("VERIFY_TLS") == "true"
	req.BackupConfig.ContainerOptions = containerOptions

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

	reqJson, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}
	log.Printf("Request=", string(reqJson))
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
