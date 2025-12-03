package server

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (s *appServer) CreateBackup(req *api.CreateBackupRequest, stream api.XtrabackupService_CreateBackupServer) error {
	log := logf.Log.WithName("xtrabackup-server").WithName("CreateBackup")

	if !s.backupStatus.tryRunBackup() {
		log.Info("backup is already running")
		return status.Errorf(codes.FailedPrecondition, "backup is already running")
	}
	defer s.backupStatus.doneBackup()

	log = log.WithValues("namespace", s.namespace, "name", req.BackupName)

	s.backupStatus.setBackupConfig(req.BackupConfig)
	defer s.backupStatus.removeBackupConfig()

	ctx := stream.Context()

	log.Info("Checking if backup exists")
	exists, err := s.backupExists(ctx, req.BackupConfig)
	if err != nil {
		return errors.Wrap(err, "check if backup exists")
	}
	if exists {
		log.Info("Backup already exists, deleting")
		if err := s.deleteBackupFunc(ctx, req.BackupConfig, req.BackupName); err != nil {
			log.Error(err, "failed to delete backup")
			return errors.Wrap(err, "delete backup")
		}
	}

	backupUser := users.Xtrabackup
	backupPass, err := getUserPassword()
	if err != nil {
		log.Error(err, "failed to get backup user password")
		return errors.Wrap(err, "get backup user password")
	}

	g, gCtx := errgroup.WithContext(ctx)

	xtrabackup := req.BackupConfig.NewXtrabackupCmd(gCtx, backupUser, backupPass, s.tableSpaceEncryptionEnabled)
	xbOut, err := xtrabackup.StdoutPipe()
	if err != nil {
		log.Error(err, "xtrabackup stdout pipe failed")
		return errors.Wrap(err, "xtrabackup stdout pipe failed")
	}
	defer xbOut.Close() //nolint:errcheck

	xbErr, err := xtrabackup.StderrPipe()
	if err != nil {
		log.Error(err, "xtrabackup stderr pipe failed")
		return errors.Wrap(err, "xtrabackup stderr pipe failed")
	}
	defer xbErr.Close() //nolint:errcheck

	backupLog, err := os.Create(filepath.Join(app.BackupLogDir, req.BackupName+".log"))
	if err != nil {
		log.Error(err, "failed to create log file")
		return errors.Wrap(err, "failed to create log file")
	}
	defer backupLog.Close() //nolint:errcheck
	logWriter := io.MultiWriter(backupLog, os.Stderr)

	xbcloud := req.BackupConfig.NewXbcloudCmd(gCtx, api.XBCloudActionPut, xbOut)
	xbcloudErr, err := xbcloud.StderrPipe()
	if err != nil {
		log.Error(err, "xbcloud stderr pipe failed")
		return errors.Wrap(err, "xbcloud stderr pipe failed")
	}
	defer xbcloudErr.Close() //nolint:errcheck

	log.Info(
		"Backup starting",
		"destination", req.BackupConfig.Destination,
		"storage", req.BackupConfig.Type,
		"xtrabackupCmd", sanitizeCmd(xtrabackup),
		"xbcloudCmd", sanitizeCmd(xbcloud),
	)

	g.Go(func() error {
		if err := xbcloud.Start(); err != nil {
			log.Error(err, "failed to start xbcloud")
			return err
		}

		if _, err := io.Copy(logWriter, xbcloudErr); err != nil {
			log.Error(err, "failed to copy xbcloud stderr")
			return err
		}

		if err := xbcloud.Wait(); err != nil {
			log.Error(err, "failed waiting for xbcloud to finish")
			return err
		}
		return nil
	})

	g.Go(func() error {
		if err := xtrabackup.Start(); err != nil {
			log.Error(err, "failed to start xtrabackup command")
			return err
		}

		if _, err := io.Copy(logWriter, xbErr); err != nil {
			log.Error(err, "failed to copy xtrabackup stderr")
			return err
		}

		if err := xtrabackup.Wait(); err != nil {
			log.Error(err, "failed to wait for xtrabackup to finish")
			return err
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		log.Error(err, "backup failed")
		return errors.Wrap(err, "backup failed")
	}
	if err := s.checkBackupMD5Size(ctx, req.BackupConfig); err != nil {
		log.Error(err, "check backup md5 file size")
		return errors.Wrap(err, "check backup md5 file size")
	}
	log.Info("Backup finished successfully", "destination", req.BackupConfig.Destination, "storage", req.BackupConfig.Type)

	return nil
}

func (s *appServer) checkBackupMD5Size(ctx context.Context, cfg *api.BackupConfig) error {
	// xbcloud doesn't create md5 file for azure
	if cfg.Type == api.BackupStorageType_AZURE {
		return nil
	}

	opts, err := storage.GetOptionsFromBackupConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "get options from backup config")
	}
	storageClient, err := s.newStorageFunc(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "new storage")
	}
	r, err := storageClient.GetObject(ctx, cfg.Destination+".md5")
	if err != nil {
		return errors.Wrap(err, "get object")
	}
	defer r.Close() //nolint:errcheck
	data, err := io.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "read all")
	}

	// Q: what value we should use here?
	// size of the `demand-backup` test md5 file is 4575
	minSize := 3000
	if len(data) < minSize {
		return errors.Errorf("backup was finished unsuccessful: small md5 size: %d: expected to be >= %d", len(data), minSize)
	}
	return nil
}

func getUserPassword() (string, error) {
	password, ok := os.LookupEnv("XTRABACKUP_USER_PASS")
	if !ok {
		return "", errors.New("XTRABACKUP_USER_PASS environment variable is not set")
	}
	return password, nil
}

func (s *appServer) backupExists(ctx context.Context, cfg *api.BackupConfig) (bool, error) {
	opts, err := storage.GetOptionsFromBackupConfig(cfg)
	if err != nil {
		return false, errors.Wrap(err, "get options from backup config")
	}
	storage, err := s.newStorageFunc(ctx, opts)
	if err != nil {
		return false, errors.Wrap(err, "new storage")
	}
	objects, err := storage.ListObjects(ctx, cfg.Destination+"/")
	if err != nil {
		return false, errors.Wrap(err, "list objects")
	}
	if len(objects) == 0 {
		return false, nil
	}
	return true, nil
}

func deleteBackup(ctx context.Context, cfg *api.BackupConfig, backupName string) error {
	log := logf.Log.WithName("deleteBackup")

	logWriter := io.Writer(os.Stderr)
	if backupName != "" {
		backupLog, err := os.OpenFile(
			filepath.Join(app.BackupLogDir, backupName+".log"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			return errors.Wrap(err, "failed to open log file")
		}
		defer backupLog.Close() //nolint:errcheck
		logWriter = io.MultiWriter(backupLog, os.Stderr)
	}

	xbcloud := cfg.NewXbcloudCmd(ctx, api.XBCloudActionDelete, nil)
	xbcloudErr, err := xbcloud.StderrPipe()
	if err != nil {
		return errors.Wrap(err, "xbcloud stderr pipe failed")
	}
	defer xbcloudErr.Close() //nolint:errcheck
	log.Info(
		"Deleting Backup",
		"destination", cfg.Destination,
		"storage", cfg.Type,
		"xbcloudCmd", sanitizeCmd(xbcloud),
	)
	if err := xbcloud.Start(); err != nil {
		return errors.Wrap(err, "failed to start xbcloud")
	}

	if _, err := io.Copy(logWriter, xbcloudErr); err != nil {
		return errors.Wrap(err, "failed to copy xbcloud stderr")
	}

	if err := xbcloud.Wait(); err != nil {
		return errors.Wrap(err, "failed waiting for xbcloud to finish")
	}
	return nil

}

func sanitizeCmd(cmd *exec.Cmd) string {
	sensitiveFlags := regexp.MustCompile("--password=(.*)|--.*-access-key=(.*)|--.*secret-key=(.*)")
	c := []string{cmd.Path}

	for _, arg := range cmd.Args[1:] {
		c = append(c, sensitiveFlags.ReplaceAllString(arg, ""))
	}

	return strings.Join(c, " ")
}
