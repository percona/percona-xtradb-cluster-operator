package proxysqlcnf

import (
	"context"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const (
	NodeReadyProxyCluster = "admin_node_ready_proxy_cluster"
	NodeReadyPXCCluster   = "admin_node_ready_pxc_cluster"
)

type ProxyConfManager struct {
	conn *sqlx.DB
}

func NewProxyConfManager(connstr string) (*ProxyConfManager, error) {
	conn, err := sqlx.Connect("mysql", connstr)
	if err != nil {
		return nil, errors.Wrap(err, "can't connect to database")
	}
	return &ProxyConfManager{
		conn: conn,
	}, nil
}

func (m *ProxyConfManager) Close() error {
	if err := m.conn.Close(); err != nil {
		return errors.Wrap(err, "can't Close connection to database")
	}
	return nil
}

func (m *ProxyConfManager) insertToProxysqlServersTable(hostnameList []string) error {
	ctx := context.Background()
	tx, err := m.conn.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return errors.Wrap(err, "can't add proxysql nodes list to node")
	}
	defer tx.Rollback()

	for _, hostname := range hostnameList {
		if _, err := tx.ExecContext(ctx, `INSERT INTO proxysql_servers(hostname, port) VALUES ($1, $2);`, hostname, 6032); err != nil {
			return errors.Wrapf(err, "can't insert hostname %s to proxysql_servers table", hostname)
		}
	}
	if _, err := tx.ExecContext(ctx, `LOAD PROXYSQL SERVERS TO RUNTIME`); err != nil {
		return errors.Wrap(err, "setup proxysql_servers table transaction failed")
	}
	if _, err := tx.ExecContext(ctx, `SAVE PROXYSQL SERVERS TO DISK`); err != nil {
		return errors.Wrap(err, "setup proxysql_servers table transaction failed")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "transaction failed")
	}
	return nil
}

func (m *ProxyConfManager) isNodeReadyProxyCluster() (bool, error) {
	var isReady bool
	if err := m.conn.Select(&isReady, `SELECT $1 FROM global_variables;`, NodeReadyProxyCluster); err != nil {
		return false, errors.Wrap(err, "can't check if proxysql is ready to serve requests")
	}
	return isReady, nil
}

func (m *ProxyConfManager) setNodeReadyProxyCluster(status bool) error {
	ctx := context.Background()
	tx, err := m.conn.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return errors.Wrap(err, "can't add proxysql nodes list to node")
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO global_variables (variable_name, variable_value) VALUES ($1, $2) ON DUPLICATE KEY UPDATE variable_value=$2;`, NodeReadyProxyCluster, status); err != nil {
		return errors.Wrap(err, "failed to set NodeReadyProxyCluster status")
	}
	if _, err := tx.ExecContext(ctx, `LOAD ADMIN VARIABLES TO RUNTIME`); err != nil {
		return errors.Wrap(err, "failed to load proxysql_servers admin variables to memory")
	}
	if _, err := tx.ExecContext(ctx, `SAVE ADMIN VARIABLES TO DISK`); err != nil {
		return errors.Wrap(err, "failed to save proxysql_servers admin variables to disk")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "transaction failed")
	}

	return nil
}

func (m *ProxyConfManager) insertToMySQLServersTable(hostnameList []string) error {
	ctx := context.Background()
	tx, err := m.conn.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return errors.Wrap(err, "can't add proxysql nodes list to node")
	}
	defer tx.Rollback()

	for _, hostname := range hostnameList {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mysql_servers(hostname, hostgroup_id, port, weight) VALUES ($1, $2, $3, $4);`, hostname, 101, 3306, 1000); err != nil {
			return errors.Wrapf(err, "can't insert hostname %s to proxysql_servers table", hostname)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO mysql_galera_hostgroups (writer_hostgroup,backup_writer_hostgroup,reader_hostgroup, offline_hostgroup, active, max_writers, writer_is_also_reader, max_transactions_behind) 
				VALUES (100,102,101,9101,0,1,1,16);`); err != nil {
		return errors.Wrap(err, "can't insert values to mysql_galera_hostgroups")
	}
	if _, err := tx.ExecContext(ctx, `LOAD MYSQL SERVERS TO RUNTIME`); err != nil {
		return errors.Wrap(err, "setup proxysql_servers table transaction failed")
	}
	if _, err := tx.ExecContext(ctx, `SAVE MYSQL SERVERS TO DISK`); err != nil {
		return errors.Wrap(err, "setup proxysql_servers table transaction failed")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "transaction failed")
	}
	return nil
}

func (m *ProxyConfManager) isNodeReadyPCXCluster() (bool, error) {
	var isReady bool
	if err := m.conn.Select(&isReady, `SELECT $1 FROM global_variables;`, NodeReadyPXCCluster); err != nil {
		return false, errors.Wrap(err, "can't check if proxysql is ready to serve requests")
	}
	return isReady, nil
}

func (m *ProxyConfManager) setNodeReadyPXCCluster(status bool) error {
	ctx := context.Background()
	tx, err := m.conn.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return errors.Wrap(err, "can't add pxc nodes list to node")
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO global_variables (variable_name, variable_value) VALUES ($1, $2) ON DUPLICATE KEY UPDATE variable_value=$2;`, NodeReadyPXCCluster, status); err != nil {
		return errors.Wrap(err, "failed to set NodeReadyProxyCluster status")
	}
	if _, err := tx.ExecContext(ctx, `LOAD ADMIN VARIABLES TO RUNTIME`); err != nil {
		return errors.Wrap(err, "failed to load proxysql_servers admin variables to memory")
	}
	if _, err := tx.ExecContext(ctx, `SAVE ADMIN VARIABLES TO DISK`); err != nil {
		return errors.Wrap(err, "failed to save proxysql_servers admin variables to disk")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "transaction failed")
	}

	return nil
}
