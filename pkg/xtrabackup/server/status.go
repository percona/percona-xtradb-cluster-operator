package server

import (
	"sync"
	"sync/atomic"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
)

type backupStatus struct {
	isRunning         atomic.Bool
	currentBackupConf *api.BackupConfig
	mu                sync.Mutex
}

func (s *backupStatus) tryRunBackup() bool {
	return s.isRunning.CompareAndSwap(false, true)
}

func (s *backupStatus) doneBackup() {
	s.isRunning.Store(false)
}

func (s *backupStatus) setBackupConfig(conf *api.BackupConfig) {
	s.mu.Lock()
	s.currentBackupConf = conf
	s.mu.Unlock()
}

func (s *backupStatus) removeBackupConfig() {
	s.mu.Lock()
	s.currentBackupConf = nil
	s.mu.Unlock()
}
