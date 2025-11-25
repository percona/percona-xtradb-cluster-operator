package server

import (
	"context"
	"time"

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

func (s *appServer) CreateBackup(req *api.CreateBackupRequest, stream api.XtrabackupService_CreateBackupServer) error {
	// do some work, then send message
	time.Sleep(120 * time.Second)
	stream.Send(&api.CreateBackupResponse{})
	return nil
}

func (s *appServer) DeleteBackup(ctx context.Context, req *api.DeleteBackupRequest) (*api.DeleteBackupResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteBackup not implemented")
}
