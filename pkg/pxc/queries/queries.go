package queries

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pkg/errors"

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

type ReplicationChannelSource struct {
	Name   string
	Host   string
	Port   int
	Weight int
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

func (p *Database) IsReplica() (bool, error) {
	rows, err := p.db.Query("show replica status")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to check if pod is replica")
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return false, errors.Wrap(err, "get columns")
	}
	vals := make([]interface{}, len(cols))
	for i := range cols {
		vals[i] = new(sql.RawBytes)
	}
	for rows.Next() {
		err = rows.Scan(vals...)
		if err != nil {
			return false, errors.Wrap(err, "scan replication status")
		}
	}
	return true, err
}

func (p *Database) StopAllReplication() error {
	_, err := p.db.Exec("stop replica")
	return errors.Wrap(err, "failed to stop replication")
}

func (p *Database) AddReplicationSource(name, host string, port, weight int) error {
	_, err := p.db.Exec("SELECT asynchronous_connection_failover_add_source(?, ?, ?, null, ?)", name, host, port, weight)
	return errors.Wrap(err, "add replication source")
}

func (p *Database) ReplicationChannelSources(channelName string) ([]ReplicationChannelSource, error) {
	rows, err := p.db.Query("select host,port,weight from replication_asynchronous_connection_failover where channel_name=?", channelName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "get replication channels")
	}
	defer rows.Close()
	result := make([]ReplicationChannelSource, 0)
	for rows.Next() {
		r := ReplicationChannelSource{}
		err = rows.Scan(&r.Host, &r.Port, &r.Weight)
		if err != nil {
			return nil, errors.Wrap(err, "read replication channel info")
		}
		result = append(result, r)
	}
	return result, nil
}

func (p *Database) StopReplication(name string) error {
	_, err := p.db.Exec("stop replica for channel ?", name)
	return errors.Wrap(err, "stop replication for channel "+name)
}

func (p *Database) StartReplication(replicaPass string, src ReplicationChannelSource) error {
	_, err := p.db.Exec(`
	change master to
    master_user='replication',
    master_password=?,
    master_host=?,
	master_port=?,
    source_connection_auto_failover=1,
	master_auto_position=1,
    master_retry_count=3,
    master_connect_retry=60  
    for channel ?
`, replicaPass, src.Host, src.Port, src.Name)
	if err != nil {
		return errors.Wrap(err, "change source for channel "+src.Name)
	}

	_, err = p.db.Exec(`start replica for channel ?`, src.Name)
	return errors.Wrap(err, "start replica for source "+src.Name)

}

func (p *Database) DeleteReplicationSource(name, host string, port int) error {
	_, err := p.db.Exec("SELECT asynchronous_connection_failover_delete_source(?, ?, ?, null)", name, host, port)
	return errors.Wrap(err, "delete replication source "+name)
}

func (p *Database) Status(host, ip string) ([]string, error) {
	rows, err := p.db.Query("select status from mysql_servers where hostname like ? or hostname = ?;", host+"%", ip)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
		if errors.Is(err, sql.ErrNoRows) {
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
		if errors.Is(err, sql.ErrNoRows) {
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
