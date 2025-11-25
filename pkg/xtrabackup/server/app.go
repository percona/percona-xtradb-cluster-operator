package server

import (
	"context"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DefaultPort is the default port for the app server.
const DefaultPort = 6450

type appServer struct {
	api.UnimplementedXtrabackupServiceServer
}

var _ api.XtrabackupServiceServer = (*appServer)(nil)

// New returns a new app server.
func New() api.XtrabackupServiceServer {
	return &appServer{}
}

func (s *appServer) GetCurrentBackupConfig(ctx context.Context, req *api.GetCurrentBackupConfigRequest) (*api.BackupConfig, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCurrentBackupConfig not implemented")
}

func (s *appServer) CreateBackup(ctx context.Context, req *api.CreateBackupRequest) (*api.CreateBackupResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateBackup not implemented")
}

func (s *appServer) DeleteBackup(ctx context.Context, req *api.DeleteBackupRequest) (*api.DeleteBackupResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteBackup not implemented")
}
