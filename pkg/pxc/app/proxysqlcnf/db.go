package proxysqlcnf

import (
	"context"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type DBManager struct {
	db *sqlx.DB
}

func NewDB(connstr string) (*DBManager, error) {
	db, err := sqlx.Connect("mysql", connstr)
	if err != nil {
		return nil, errors.Wrap(err, "can't connect to database")
	}
	return &DBManager{
		db: db,
	}, nil
}

func (m *DBManager) close() error {
	if err := m.db.Close(); err != nil {
		return errors.Wrap(err, "can't close connection to database")
	}
	return nil
}

func (m *DBManager) initializeNode(hostname string, hostnameList []string) error {
	initialized, err := m.isNodeInitialized()
	if err != nil {
		return errors.Wrap(err, "can't check node initialization status")
	}

	if initialized {
		return nil
	}

	if err := m.setupProxySchema(); err != nil {
		return errors.Wrap(err, "can't initialize node")
	}

	if err := m.updateProxysqlServersTable(hostnameList); err != nil {
		return errors.Wrap(err, "can't initialize node")
	}
	return nil
}

func (m *DBManager) setupProxySchema() error {
	ctx := context.Background()

	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return errors.Wrap(err, "can't setup proxysql schema")
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, proxysqlServersTable); err != nil {
		return errors.Wrap(err, "setup proxysql_servers table transaction failed")
	}
	if _, err := tx.ExecContext(ctx, runtimeProxysqlServersTable); err != nil {
		return errors.Wrap(err, "setup runtime_proxysql_servers table transaction failed")
	}
	if _, err := tx.ExecContext(ctx, runtimeChecksumsValuesTable); err != nil {
		return errors.Wrap(err, "setup runtime_checksums_values table transaction failed")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}
	return nil
}

func (m *DBManager) updateProxysqlServersTable(hostnameList []string) error {
	ctx := context.Background()
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return errors.Wrap(err, "can't add proxysql nodes list to node")
	}
	defer tx.Rollback()

	for _, hostname := range hostnameList {
		if _, err := tx.Exec(`INSERT INTO proxysql_servers(hostname, port) VALUES ($1, $2);`, hostname, 3306); err != nil {
			return errors.Wrapf(err, "can't insert hostname %s to proxysql_servers table", hostname)
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "transaction failed")
	}
	return nil
}

func (m *DBManager) isNodeInitialized() (bool, error) {
	tables := make([]string, 0)
	if err := m.db.Select(&tables, "SHOW TABLES"); err != nil {
		return false, errors.Wrap(err, "can't check if proxysql node is initialized")
	}
	proxysqlServersExist := false
	runtimeProxysqlServersExist := false
	runtimeChecksumsValuesExist := false

	for _, table := range tables {
		if table == "proxysql_servers" {
			proxysqlServersExist = true
		}
		if table == "runtime_proxysql_servers" {
			runtimeProxysqlServersExist = true
		}
		if table == "runtime_checksums_values" {
			runtimeChecksumsValuesExist = true
		}
	}
	if proxysqlServersExist && runtimeProxysqlServersExist && runtimeChecksumsValuesExist {
		return true, nil
	}

	return false, nil
}
