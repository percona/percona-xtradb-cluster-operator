package server

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (s *appServer) GetLogs(req *api.GetLogsRequest, stream api.XtrabackupService_GetLogsServer) error {
	log := logf.Log.WithName("xtrabackup-server").WithName("GetLogs")

	log.Info("Getting logs", "backup_name", req.BackupName)

	logFile, err := os.Open(filepath.Join(app.BackupLogDir, req.BackupName+".log"))
	if err != nil {
		return errors.Wrap(err, "failed to open log file")
	}
	defer logFile.Close()

	buf := bufio.NewScanner(logFile)
	for buf.Scan() {
		if err := stream.Send(&api.LogChunk{
			Log: buf.Text(),
		}); err != nil {
			return fmt.Errorf("error streaming log chunk: %w", err)
		}
	}

	if err := buf.Err(); err != nil {
		return errors.Wrap(err, "failed to read log file")
	}
	return nil
}
