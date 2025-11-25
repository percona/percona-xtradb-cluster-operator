package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
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

func getRequestObject() *api.CreateBackupRequest {
	var rawB64Json string
	flag.StringVar(&rawB64Json, "request-json", "", "Request JSON in base64 encoded string")
	flag.Parse()

	if rawB64Json == "" {
		log.Fatal("Backup config is required")
	}

	req := &api.CreateBackupRequest{}
	if err := json.Unmarshal([]byte(rawB64Json), req); err != nil {
		log.Fatal("Failed to unmarshal request JSON: %w", err)
	}
	return req
}
