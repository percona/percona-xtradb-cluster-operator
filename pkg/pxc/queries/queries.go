package queries

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
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

var sensitiveRegexp = regexp.MustCompile(":.*@")

type Database struct {
	Client    *clientcmd.Client
	Pod       *corev1.Pod
	cmd       []string
	container string
}

// NewPXC creates a new Database instance for a given PXC pod
func NewPXC(pod *corev1.Pod, cliCmd *clientcmd.Client, user, pass, host string) *Database {
	cmd := []string{"mysql", "--database", "mysql", fmt.Sprintf("-p%s", pass), "-u", string(user), "-h", host, "-e"}
	return &Database{Client: cliCmd, Pod: pod, container: "pxc", cmd: cmd}
}

// NewProxySQL creates a new Database instance for a given ProxySQL pod
func NewProxySQL(pod *corev1.Pod, cliCmd *clientcmd.Client, user, pass string) *Database {
	cmd := []string{"mysql", fmt.Sprintf("-p%s", pass), "-u", string(user), "-h", "127.0.0.1", "-P", "6032", "-e"}
	return &Database{Client: cliCmd, Pod: pod, container: "proxysql", cmd: cmd}
}

// NewHAProxy creates a new Database instance for a given HAProxy pod
func NewHAProxy(pod *corev1.Pod, cliCmd *clientcmd.Client, user, pass string) *Database {
	cmd := []string{"mysql", fmt.Sprintf("-p%s", pass), "-u", string(user), "-h", "127.0.0.1", "-e"}
	return &Database{Client: cliCmd, Pod: pod, container: "haproxy", cmd: cmd}
}

// Exec executes a given SQL statement on a database and populates
// stdout and stderr buffers with the output.
func (d *Database) Exec(ctx context.Context, stm string, stdout, stderr *bytes.Buffer) error {
	cmd := append(d.cmd, stm)
	err := d.Client.Exec(d.Pod, d.container, cmd, nil, stdout, stderr, false)
	if err != nil {
		sout := sensitiveRegexp.ReplaceAllString(stdout.String(), ":*****@")
		serr := sensitiveRegexp.ReplaceAllString(stderr.String(), ":*****@")
		return errors.Wrapf(err, "stdout: %s, stderr: %s", sout, serr)
	}

	if strings.Contains(stderr.String(), "ERROR") {
		return fmt.Errorf("sql error: %s", stderr)
	}

	return nil
}

// Query executes a given SQL statement on a database and populates out with the result
func (d *Database) Query(ctx context.Context, query string, out interface{}) error {
	var errb, outb bytes.Buffer
	err := d.Exec(ctx, query, &outb, &errb)
	if err != nil {
		return err
	}

	if !strings.Contains(errb.String(), "ERROR") && outb.Len() == 0 {
		return sql.ErrNoRows
	}

	csv := csv.NewReader(bytes.NewReader(outb.Bytes()))
	csv.Comma = '\t'

	if err = gocsv.UnmarshalCSV(csv, out); err != nil {
		return err
	}

	return nil
}

func (d *Database) CurrentReplicationChannels(ctx context.Context) ([]string, error) {
	rows := []*struct {
		Name string `csv:"name"`
	}{}

	q := `SELECT DISTINCT(Channel_name) as name from replication_asynchronous_connection_failover`
	err := d.Query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "select current replication channels")
	}

	result := make([]string, 0)
	for _, row := range rows {
		result = append(result, row.Name)
	}
	return result, nil
}

func (d *Database) ChangeChannelPassword(ctx context.Context, channel, password string) error {
	var errb, outb bytes.Buffer

	q := fmt.Sprintf(`START TRANSACTION;
		STOP REPLICA IO_THREAD FOR CHANNEL %s;
		CHANGE REPLICATION SOURCE TO SOURCE_PASSWORD='%s' FOR CHANNEL %s;
		START REPLICA IO_THREAD FOR CHANNEL %s;
		COMMIT;
	`, channel, password, channel, channel)

	err := d.Exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "change channel password")
	}

	return nil
}

func (d *Database) ReplicationStatus(ctx context.Context, channel string) (ReplicationStatus, error) {
	rows := []*struct {
		IORunning  string `csv:"Replica_IO_Running"`
		SQLRunning string `csv:"Replica_SQL_Running"`
		LastErrNo  int    `csv:"Last_Errno"`
	}{}

	q := fmt.Sprintf("SHOW REPLICA STATUS FOR CHANNEL '%s'", channel)
	err := d.Query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReplicationStatusNotInitiated, nil
		}
		return ReplicationStatusError, errors.Wrap(err, "select replication status")
	}

	ioRunning := rows[0].IORunning == "Yes"
	sqlRunning := rows[0].SQLRunning == "Yes"
	lastErrNo := rows[0].LastErrNo

	if ioRunning && sqlRunning {
		return ReplicationStatusActive, nil
	}

	if !ioRunning && !sqlRunning && lastErrNo == 0 {
		return ReplicationStatusNotInitiated, nil
	}

	return ReplicationStatusError, nil
}

func (d *Database) StopAllReplication(ctx context.Context) error {
	var errb, outb bytes.Buffer
	if err := d.Exec(ctx, "STOP REPLICA", &outb, &errb); err != nil {
		return errors.Wrap(err, "failed to stop replication")
	}
	return nil
}

func (d *Database) AddReplicationSource(ctx context.Context, name, host string, port, weight int) error {
	var errb, outb bytes.Buffer
	q := fmt.Sprintf("SELECT asynchronous_connection_failover_add_source('%s', '%s', %d, null, %d)", name, host, port, weight)
	err := d.Exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "add replication source")
	}
	return nil
}

func (d *Database) ReplicationChannelSources(ctx context.Context, channelName string) ([]ReplicationChannelSource, error) {
	rows := []*struct {
		Host   string `csv:"host"`
		Port   int    `csv:"port"`
		Wieght int    `csv:"weight"`
	}{}

	q := fmt.Sprintf("SELECT host, port, weight FROM replication_asynchronous_connection_failover WHERE Channel_name = '%s'", channelName)
	err := d.Query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "get replication channels")
	}

	result := make([]ReplicationChannelSource, 0)
	for _, row := range rows {
		result = append(result, ReplicationChannelSource{Host: row.Host, Port: row.Port, Weight: row.Wieght})
	}
	return result, nil
}

func (d *Database) StopReplication(ctx context.Context, name string) error {
	var errb, outb bytes.Buffer
	err := d.Exec(ctx, fmt.Sprintf("STOP REPLICA FOR CHANNEL '%s'", name), &outb, &errb)
	return errors.Wrap(err, "stop replication for channel "+name)
}

func (d *Database) EnableReadonly(ctx context.Context) error {
	var errb, outb bytes.Buffer
	err := d.Exec(ctx, "SET GLOBAL READ_ONLY=1", &outb, &errb)
	return errors.Wrap(err, "set global read_only param to 1")
}

func (d *Database) DisableReadonly(ctx context.Context) error {
	var errb, outb bytes.Buffer
	err := d.Exec(ctx, "SET GLOBAL READ_ONLY=0", &outb, &errb)
	return errors.Wrap(err, "set global read_only param to 0")
}

func (p *Database) IsReadonlyExec(ctx context.Context) (bool, error) {
	rows := []*struct {
		ReadOnly int `csv:"readOnly"`
	}{}
	err := p.Query(ctx, "select @@read_only as readOnly", &rows)
	if err != nil {
		return false, errors.Wrap(err, "select global read_only param")
	}
	return rows[0].ReadOnly == 1, nil
}

func (d *Database) StartReplication(ctx context.Context, replicaPass string, config ReplicationConfig) error {
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

	q := fmt.Sprintf(`
		CHANGE REPLICATION SOURCE TO
			SOURCE_USER='replication',
			SOURCE_PASSWORD='%s',
			SOURCE_HOST='%s',
			SOURCE_PORT=%d,
			SOURCE_CONNECTION_AUTO_FAILOVER=1,
			SOURCE_AUTO_POSITION=1,
			SOURCE_RETRY_COUNT=%d,
			SOURCE_CONNECT_RETRY=%d,
			SOURCE_SSL=%d,
			SOURCE_SSL_CA='%s',
			SOURCE_SSL_VERIFY_SERVER_CERT=%d
			FOR CHANNEL '%s'
	`, replicaPass, config.Source.Host, config.Source.Port, config.SourceRetryCount, config.SourceConnectRetry, ssl, ca, sslVerify, config.Source.Name)

	var errb, outb bytes.Buffer
	err := d.Exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrapf(err, "change source for channel %s", config.Source.Name)
	}

	outb.Reset()
	errb.Reset()
	err = d.Exec(ctx, fmt.Sprintf(`START REPLICA FOR CHANNEL '%s'`, config.Source.Name), &outb, &errb)
	return errors.Wrapf(err, "start replica for source %s", config.Source.Name)

}

func (d *Database) DeleteReplicationSource(ctx context.Context, name, host string, port int) error {
	var errb, outb bytes.Buffer
	q := fmt.Sprintf("SELECT asynchronous_connection_failover_delete_source('%s', '%s', %d, null)", name, host, port)
	err := d.Exec(ctx, q, &outb, &errb)
	return errors.Wrap(err, "delete replication source "+name)
}

func (d *Database) ProxySQLInstanceStatus(ctx context.Context, host string) ([]string, error) {

	rows := []*struct {
		Status string `csv:"status"`
	}{}

	q := fmt.Sprintf("SELECT status FROM proxysql_servers WHERE hostname LIKE '%s%%'", host)

	err := d.Query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	statuses := []string{}
	for _, row := range rows {
		statuses = append(statuses, row.Status)
	}

	return statuses, nil
}

func (d *Database) PresentInHostgroups(ctx context.Context, host string) (bool, error) {
	hostgroups := []string{WriterHostgroup, ReaderHostgroup}

	rows := []*struct {
		Count int `csv:"count"`
	}{}

	q := fmt.Sprintf(`
		SELECT COUNT(*) FROM mysql_servers
		INNER JOIN mysql_galera_hostgroups ON hostgroup_id IN (%s)
		WHERE hostname LIKE '%s' GROUP BY hostname`, strings.Join(hostgroups, ","), host+"%")

	err := d.Query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	if rows[0].Count != len(hostgroups) {
		return false, nil
	}
	return true, nil
}

func (d *Database) PrimaryHost(ctx context.Context) (string, error) {
	rows := []*struct {
		Hostname string `csv:"host"`
	}{}

	q := fmt.Sprintf("SELECT hostname as host FROM runtime_mysql_servers WHERE hostgroup_id = %d", writerID)

	err := d.Query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	return rows[0].Hostname, nil
}

func (d *Database) Hostname(ctx context.Context) (string, error) {
	rows := []*struct {
		Hostname string `csv:"hostname"`
	}{}

	err := d.Query(ctx, "SELECT @@hostname hostname", &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	return rows[0].Hostname, nil
}

func (d *Database) WsrepLocalStateComment(ctx context.Context) (string, error) {
	rows := []*struct {
		VariableName string `csv:"Variable_name"`
		Value        string `csv:"Value"`
	}{}

	err := d.Query(ctx, "SHOW GLOBAL STATUS LIKE 'wsrep_local_state_comment'", &rows)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return rows[0].Value, nil
}

func (d *Database) Version(ctx context.Context) (string, error) {
	rows := []*struct {
		Version string `csv:"version"`
	}{}

	err := d.Query(ctx, "select @@VERSION as version;", &rows)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return rows[0].Version, nil
}
