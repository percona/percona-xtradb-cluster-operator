package server

import (
	"context"
	"os"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DefaultPort is the default port for the app server.
const DefaultPort = 6450

type appServer struct {
	api.UnimplementedXtrabackupServiceServer

	backupStatus     backupStatus
	namespace        string
	newStorageFunc   storage.NewClientFunc
	deleteBackupFunc func(ctx context.Context, cfg *api.BackupConfig, backupName string) error
}

var _ api.XtrabackupServiceServer = (*appServer)(nil)

// New returns a new app server.
func New() (api.XtrabackupServiceServer, error) {
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok || namespace == "" {
		return nil, status.Errorf(codes.InvalidArgument, "POD_NAMESPACE environment variable is not set")
	}
	return &appServer{
		namespace:        namespace,
		backupStatus:     backupStatus{},
		newStorageFunc:   storage.NewClient,
		deleteBackupFunc: deleteBackup,
	}, nil
}

func (s *appServer) GetCurrentBackupConfig(ctx context.Context, req *api.GetCurrentBackupConfigRequest) (*api.BackupConfig, error) {
	// TODO
	return nil, status.Errorf(codes.Unimplemented, "method GetCurrentBackupConfig not implemented")
}

func (s *appServer) DeleteBackup(ctx context.Context, req *api.DeleteBackupRequest) (*api.DeleteBackupResponse, error) {
	// TODO
	return nil, status.Errorf(codes.Unimplemented, "method DeleteBackup not implemented")
}
