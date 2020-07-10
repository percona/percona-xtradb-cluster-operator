package queries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// value of writer group is hardcoded in ProxySQL config inside docker image
// https://github.com/percona/percona-docker/blob/pxc-operator-1.3.0/proxysql/dockerdir/etc/proxysql-admin.cnf#L23
const writerID = 11

type Database struct {
	db *sql.DB
}

var ErrNotFound = errors.New("not found")

func New(client client.Client, namespace, secretName, user, host string, port int32) (Database, error) {
	secretObj := corev1.Secret{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: namespace,
			Name:      secretName,
		},
		&secretObj,
	)
	if err != nil {
		return Database{}, err
	}

	pass := string(secretObj.Data[user])
	connStr := fmt.Sprintf("%s:%s@tcp(%s:%d)/mysql?interpolateParams=true", user, pass, host, port)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		return Database{}, err
	}

	err = db.Ping()
	if err != nil {
		return Database{}, err
	}

	return Database{
		db: db,
	}, nil
}

func (p *Database) Status(host, ip string) ([]string, error) {
	rows, err := p.db.Query("select status from mysql_servers where hostname like ? or hostname = ?;", host+"%", ip)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	statuses := []string{}
	for rows.Next() {
		var status string

		err := rows.Scan(&status)
		if err != nil {
			return nil, err
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (p *Database) PrimaryHost() (string, error) {
	var host string
	err := p.db.QueryRow("SELECT hostname FROM runtime_mysql_servers WHERE hostgroup_id = ?", writerID).Scan(&host)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}

	return host, nil
}

func (p *Database) Hostname() (string, error) {
	var hostname string
	err := p.db.QueryRow("SELECT @@hostname hostname").Scan(&hostname)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}

	return hostname, nil
}

func (p *Database) WsrepLocalStateComment() (string, error) {
	var variable_name string
	var value string

	err := p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'wsrep_local_state_comment'").Scan(&variable_name, &value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return value, nil
}

func (p *Database) Version() (string, error) {
	var version string

	err := p.db.QueryRow("select @@VERSION;").Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return version, nil
}

func (p *Database) Close() error {
	return p.db.Close()
}
