package server

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	goversion "github.com/hashicorp/go-version"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// DefaultPort is the default port for the app server.
const DefaultPort = 6450

type appServer struct {
	api.UnimplementedXtrabackupServiceServer

	backupStatus                backupStatus
	namespace                   string
	newStorageFunc              storage.NewClientFunc
	deleteBackupFunc            func(ctx context.Context, cfg *api.BackupConfig, backupName string) error
	log                         logr.Logger
	mysqlVersion                *goversion.Version
	tableSpaceEncryptionEnabled bool
}

var _ api.XtrabackupServiceServer = (*appServer)(nil)

// New returns a new app server.
func New() (api.XtrabackupServiceServer, error) {
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok || namespace == "" {
		return nil, status.Errorf(codes.InvalidArgument, "POD_NAMESPACE environment variable is not set")
	}
	tableSpaceEncryptionEnabled := vaultKeyringFileExists()

	mysqlVer, err := getMySQLVersionFromXtrabackup()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get MySQL version from XtraBackup")
	}

	logger := zap.New()
	return &appServer{
		namespace:                   namespace,
		backupStatus:                backupStatus{},
		newStorageFunc:              storage.NewClient,
		deleteBackupFunc:            deleteBackup,
		log:                         logger,
		tableSpaceEncryptionEnabled: tableSpaceEncryptionEnabled,
		mysqlVersion:                goversion.Must(goversion.NewVersion(mysqlVer)),
	}, nil
}

func vaultKeyringFileExists() bool {
	vaultKeyringPath := os.Getenv("VAULT_KEYRING_PATH")
	_, err := os.Stat(vaultKeyringPath)
	if err != nil && !os.IsNotExist(err) {
		panic(errors.Wrap(err, "failed to stat vault keyring file"))
	}
	return err == nil
}

func (s *appServer) GetCurrentBackupConfig(ctx context.Context, req *api.GetCurrentBackupConfigRequest) (*api.BackupConfig, error) {
	// TODO
	return nil, status.Errorf(codes.Unimplemented, "method GetCurrentBackupConfig not implemented")
}

func (s *appServer) DeleteBackup(ctx context.Context, req *api.DeleteBackupRequest) (*api.DeleteBackupResponse, error) {
	// TODO
	return nil, status.Errorf(codes.Unimplemented, "method DeleteBackup not implemented")
}

func init() {
	log.SetLogger(zap.New())
}
