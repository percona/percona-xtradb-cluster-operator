package queries

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/go-sql-driver/mysql"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReplicationStatus int8

const (
	ReplicationStatusActive ReplicationStatus = iota
	ReplicationStatusError
	ReplicationStatusNotInitiated
)

const (
	WriterHostgroup = "writer_hostgroup"
	ReaderHostgroup = "reader_hostgroup"
)

// value of writer group is hardcoded in ProxySQL config inside docker image
// https://github.com/percona/percona-docker/blob/pxc-operator-1.3.0/proxysql/dockerdir/etc/proxysql-admin.cnf#L23
const writerID = 11

type Database struct {
	db *sql.DB
}

type ReplicationConfig struct {
	Source             ReplicationChannelSource
	SourceRetryCount   uint
	SourceConnectRetry uint
	SSL                bool
	SSLSkipVerify      bool
	CA                 string
}

type ReplicationChannelSource struct {
	Name   string
	Host   string
	Port   int
	Weight int
}

var ErrNotFound = errors.New("not found")

func New(client client.Client, namespace, secretName, user, host string, port int32, timeout int32) (Database, error) {
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

	timeoutStr := fmt.Sprintf("%ds", timeout)
	config := mysql.NewConfig()
	config.User = user
	config.Passwd = string(secretObj.Data[user])
	config.Net = "tcp"
	config.DBName = "mysql"
	config.Addr = fmt.Sprintf("%s:%d", host, port)
	config.Params = map[string]string{
		"interpolateParams": "true",
		"timeout":           timeoutStr,
		"readTimeout":       timeoutStr,
		"writeTimeout":      timeoutStr,
		"tls":               "preferred",
	}

	db, err := sql.Open("mysql", config.FormatDSN())
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

func (p *Database) CurrentReplicationChannels() ([]string, error) {
	rows, err := p.db.Query(`SELECT DISTINCT(Channel_name) from replication_asynchronous_connection_failover`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "select current replication channels")
	}

	defer rows.Close()

	result := make([]string, 0)
	for rows.Next() {
		src := ""
		err = rows.Scan(&src)
		if err != nil {
			return nil, errors.Wrap(err, "scan channel name")
		}
		result = append(result, src)
	}
	return result, nil
}

func (p *Database) ChangeChannelPassword(channel, password string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return errors.Wrap(err, "start transaction for updating channel password")
	}
	_, err = tx.Exec(`STOP REPLICA IO_THREAD FOR CHANNEL ?`, channel)
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Wrapf(err, "rollback STOP REPLICA IO_THREAD FOR CHANNEL %s", channel)
		}
		return errors.Wrapf(err, "stop replication IO thread for channel %s", channel)
	}
	_, err = tx.Exec(`CHANGE REPLICATION SOURCE TO SOURCE_PASSWORD=? FOR CHANNEL ?`, password, channel)
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Wrapf(err, "rollback CHANGE SOURCE_PASSWORD FOR CHANNEL %s", channel)
		}
		return errors.Wrapf(err, "change master password for channel %s", channel)
	}
	_, err = tx.Exec(`START REPLICA IO_THREAD FOR CHANNEL ?`, channel)
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Wrapf(err, "rollback START REPLICA IO_THREAD FOR CHANNEL %s", channel)
		}
		return errors.Wrapf(err, "start io thread for channel %s", channel)
	}
	return tx.Commit()
}

func (p *Database) ReplicationStatus(channel string) (ReplicationStatus, error) {
	rows, err := p.db.Query(`SHOW REPLICA STATUS FOR CHANNEL ?`, channel)
	if err != nil {
		if strings.HasSuffix(err.Error(), "does not exist.") || errors.Is(err, sql.ErrNoRows) {
			return ReplicationStatusNotInitiated, nil
		}
		return ReplicationStatusError, errors.Wrap(err, "get current replica status")
	}

	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return ReplicationStatusError, errors.Wrap(err, "get columns")
	}
	vals := make([]interface{}, len(cols))
	for i := range cols {
		vals[i] = new(sql.RawBytes)
	}

	for rows.Next() {
		err = rows.Scan(vals...)
		if err != nil {
			return ReplicationStatusError, errors.Wrap(err, "scan replication status")
		}
	}

	IORunning := string(*vals[10].(*sql.RawBytes))
	SQLRunning := string(*vals[11].(*sql.RawBytes))
	LastErrNo := string(*vals[18].(*sql.RawBytes))
	if IORunning == "Yes" && SQLRunning == "Yes" {
		return ReplicationStatusActive, nil
	}

	if IORunning == "No" && SQLRunning == "No" && LastErrNo == "0" {
		return ReplicationStatusNotInitiated, nil
	}

	return ReplicationStatusError, nil
}

func (p *Database) StopAllReplication() error {
	_, err := p.db.Exec("STOP REPLICA")
	return errors.Wrap(err, "failed to stop replication")
}

func (p *Database) AddReplicationSource(name, host string, port, weight int) error {
	_, err := p.db.Exec("SELECT asynchronous_connection_failover_add_source(?, ?, ?, null, ?)", name, host, port, weight)
	return errors.Wrap(err, "add replication source")
}

func (p *Database) ReplicationChannelSources(channelName string) ([]ReplicationChannelSource, error) {
	rows, err := p.db.Query(`
        SELECT host,
               port,
               weight
        FROM   replication_asynchronous_connection_failover
        WHERE  channel_name = ?
    `, channelName)
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
	_, err := p.db.Exec("STOP REPLICA FOR CHANNEL ?", name)
	return errors.Wrap(err, "stop replication for channel "+name)
}

func (p *Database) EnableReadonly() error {
	_, err := p.db.Exec("SET GLOBAL READ_ONLY=1")
	return errors.Wrap(err, "set global read_only param to 1")
}

func (p *Database) DisableReadonly() error {
	_, err := p.db.Exec("SET GLOBAL READ_ONLY=0")
	return errors.Wrap(err, "set global read_only param to 0")
}

func (p *Database) IsReadonly() (bool, error) {
	readonly := 0
	err := p.db.QueryRow("select @@read_only").Scan(&readonly)
	return readonly == 1, errors.Wrap(err, "select global read_only param")
}

func (p *Database) StartReplication(replicaPass string, config ReplicationConfig) error {
	var ca string
	var ssl int
	if config.SSL {
		ssl = 1
		ca = config.CA
	}

	var sslVerify int
	if !config.SSLSkipVerify {
		sslVerify = 1
	}

	_, err := p.db.Exec(`
	CHANGE REPLICATION SOURCE TO
		SOURCE_USER='replication',
		SOURCE_PASSWORD=?,
		SOURCE_HOST=?,
		SOURCE_PORT=?,
		SOURCE_CONNECTION_AUTO_FAILOVER=1,
		SOURCE_AUTO_POSITION=1,
		SOURCE_RETRY_COUNT=?,
		SOURCE_CONNECT_RETRY=?,
		SOURCE_SSL=?,
		SOURCE_SSL_CA=?,
		SOURCE_SSL_VERIFY_SERVER_CERT=?
		FOR CHANNEL ?
`, replicaPass, config.Source.Host, config.Source.Port, config.SourceRetryCount, config.SourceConnectRetry, ssl, ca, sslVerify, config.Source.Name)
	if err != nil {
		return errors.Wrapf(err, "change source for channel %s", config.Source.Name)
	}

	_, err = p.db.Exec(`START REPLICA FOR CHANNEL ?`, config.Source.Name)
	return errors.Wrapf(err, "start replica for source %s", config.Source.Name)

}

func (p *Database) DeleteReplicationSource(name, host string, port int) error {
	_, err := p.db.Exec("SELECT asynchronous_connection_failover_delete_source(?, ?, ?, null)", name, host, port)
	return errors.Wrap(err, "delete replication source "+name)
}

func (p *Database) ProxySQLInstanceStatus(host string) ([]string, error) {
	rows, err := p.db.Query("SELECT status FROM mysql_servers WHERE hostname LIKE ?;", host+"%")
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

func (p *Database) PresentInHostgroups(host string) (bool, error) {
	hostgroups := []string{WriterHostgroup, ReaderHostgroup}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM mysql_servers
		INNER JOIN mysql_galera_hostgroups ON hostgroup_id IN (%s)
		WHERE hostname LIKE ? GROUP BY hostname`, strings.Join(hostgroups, ","))
	var count int
	err := p.db.QueryRow(query, host+"%").Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	if count != len(hostgroups) {
		return false, nil
	}
	return true, nil
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
