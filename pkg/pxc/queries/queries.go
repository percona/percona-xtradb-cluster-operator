package queries

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// value of writer group is hardcoded in ProxySQL config inside docker image 
// https://github.com/percona/percona-docker/blob/pxc-operator-1.3.0/proxysql/dockerdir/etc/proxysql-admin.cnf#L23
const writerID = 11

type database struct {
	db *sql.DB
}

func New(user, pass, host string, port int) (database, error) {
	connStr := fmt.Sprintf("%s:%s@tcp(%s:%d)/mysql?interpolateParams=true", user, pass, host, port)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		return database{}, err
	}

	err = db.Ping()
	if err != nil {
		return database{}, err
	}

	return database{
		db: db,
	}, nil
}

func (p *database) PrimaryHost() (string, error) {
	var host string
	err := p.db.QueryRow("SELECT hostname FROM runtime_mysql_servers WHERE hostgroup_id = ?", writerID).Scan(&host)
	if err != nil {
		return "", err
	}

	return host, nil
}

func (p *database) Close() error {
	return p.db.Close()
}
