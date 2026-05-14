package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
)

type BackupMeta struct {
	ClusterUUID string `json:"cluster_uuid"`
}

func (s *appServer) writeBackupMetaFile(ctx context.Context, cfg *api.BackupConfig, password string) error {
	// Write the cluster UUID into the metadata file.
	// Technically this info can be provided by pxb by setting --galera-info.
	// But we use backup locks which causes this flag to be ignored.
	// Disabling backup locks degrades performance, hence we write our own file.
	// This will be used in PITR to determine the timeline identity of the backup.
	uuid, err := readWsrepClusterStateUUID(ctx, password)
	if err != nil {
		return errors.Wrap(err, "read wsrep_cluster_state_uuid")
	}
	if uuid == "" {
		return errors.New("wsrep_cluster_state_uuid is empty")
	}

	meta := BackupMeta{
		ClusterUUID: uuid,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return errors.Wrap(err, "marshal backup meta")
	}

	opts, err := storage.GetOptionsFromBackupConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "get options from backup config")
	}
	storageClient, err := s.newStorageFunc(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "new storage")
	}

	objectName := cfg.Destination + ".meta.json"
	if err := storageClient.PutObject(ctx, objectName, bytes.NewReader(data), int64(len(data))); err != nil {
		return errors.Wrapf(err, "put '%s' object", objectName)
	}
	return nil
}

func readWsrepClusterStateUUID(ctx context.Context, password string) (string, error) {
	config := mysql.NewConfig()
	config.User = users.Xtrabackup
	config.Passwd = password
	config.Net = "unix"
	config.Addr = "/tmp/mysql.sock"
	config.DBName = "performance_schema"

	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return "", errors.Wrap(err, "open mysql connection")
	}
	defer db.Close() //nolint:errcheck

	var variableName, uuid string
	err = db.QueryRowContext(ctx,
		"SHOW GLOBAL STATUS LIKE 'wsrep_cluster_state_uuid'",
	).Scan(&variableName, &uuid)
	if err != nil {
		return "", errors.Wrap(err, "query wsrep_cluster_state_uuid")
	}
	return strings.TrimSpace(uuid), nil
}
