package queries

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/gocarina/gocsv"
	corev1 "k8s.io/api/core/v1"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
)

var sensitiveRegexp = regexp.MustCompile(":.*@")

type DatabaseExec struct {
	client *clientcmd.Client
	pod    *corev1.Pod
	user   string
	pass   string
	host   string
}

func NewExec(pod *corev1.Pod, cliCmd *clientcmd.Client, user, pass, host string) *DatabaseExec {
	return &DatabaseExec{client: cliCmd, pod: pod, user: user, pass: pass, host: host}
}

func (d *DatabaseExec) exec(ctx context.Context, stm string, stdout, stderr *bytes.Buffer) error {
	cmd := []string{"mysql", "--database", "performance_schema", fmt.Sprintf("-p%s", d.pass), "-u", string(d.user), "-h", d.host, "-e", stm}

	err := d.client.Exec(d.pod, "mysql", cmd, nil, stdout, stderr, false)
	if err != nil {
		sout := sensitiveRegexp.ReplaceAllString(stdout.String(), ":*****@")
		serr := sensitiveRegexp.ReplaceAllString(stderr.String(), ":*****@")
		return errors.Wrapf(err, "run %s, stdout: %s, stderr: %s", cmd, sout, serr)
	}

	if strings.Contains(stderr.String(), "ERROR") {
		return fmt.Errorf("sql error: %s", stderr)
	}

	return nil
}

func (d *DatabaseExec) query(ctx context.Context, query string, out interface{}) error {
	var errb, outb bytes.Buffer
	err := d.exec(ctx, query, &outb, &errb)
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

func (p *DatabaseExec) CurrentReplicationChannelsExec(ctx context.Context) ([]string, error) {
	rows := []*struct {
		name string `csv:"name"`
	}{}

	q := `SELECT DISTINCT(Channel_name) as name from replication_asynchronous_connection_failover`
	err := p.query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "select current replication channels")
	}

	result := make([]string, 0)
	for _, row := range rows {
		result = append(result, row.name)
	}
	return result, nil
}

func (p *DatabaseExec) ChangeChannelPasswordExec(ctx context.Context, channel, password string) error {
	var errb, outb bytes.Buffer

	q := fmt.Sprintf(`START TRANSACTION;
		STOP REPLICA IO_THREAD FOR CHANNEL %s;
		CHANGE REPLICATION SOURCE TO SOURCE_PASSWORD='%s' FOR CHANNEL %s;
		START REPLICA IO_THREAD FOR CHANNEL %s;
		COMMIT;
	`, channel, password, channel, channel)

	err := p.exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "change channel password")
	}

	return nil
}

// channel name moze biti: group_replication_applier ili SHOW REPLICA STATUS FOR CHANNEL group_replication_recovery
func (p *DatabaseExec) ReplicationStatusExec(ctx context.Context, channel string) (ReplicationStatus, error) {
	panic("not implemented")
}

func (p *DatabaseExec) StopAllReplicationExec(ctx context.Context) error {
	var errb, outb bytes.Buffer
	if err := p.exec(ctx, "STOP REPLICA", &outb, &errb); err != nil {
		return errors.Wrap(err, "failed to stop replication")
	}
	return nil
}

func (p *DatabaseExec) AddReplicationSourceExec(ctx context.Context, name, host string, port, weight int) error {
	var errb, outb bytes.Buffer
	q := fmt.Sprintf("SELECT asynchronous_connection_failover_add_source('%s', '%s', %d, null, %d)", name, host, port, weight)
	err := p.exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "add replication source")
	}
	return nil
}

func (p *DatabaseExec) ReplicationChannelSourcesExec(ctx context.Context, channelName string) ([]ReplicationChannelSource, error) {
	rows := []*struct {
		host   string `csv:"host"`
		port   int    `csv:"port"`
		wieght int    `csv:"weight"`
	}{}

	q := fmt.Sprintf("SELECT host, port, weight FROM replication_asynchronous_connection_failover WHERE Channel_name = '%s'", channelName)
	err := p.query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "get replication channels")
	}

	result := make([]ReplicationChannelSource, 0)
	for _, row := range rows {
		result = append(result, ReplicationChannelSource{Host: row.host, Port: row.port, Weight: row.wieght})
	}
	return result, nil
}

func (p *DatabaseExec) StopReplicationExec(ctx context.Context, name string) error {
	var errb, outb bytes.Buffer
	err := p.exec(ctx, fmt.Sprintf("STOP REPLICA FOR CHANNEL '%s'", name), &outb, &errb)
	return errors.Wrap(err, "stop replication for channel "+name)
}

func (p *DatabaseExec) EnableReadonlyExec(ctx context.Context) error {
	var errb, outb bytes.Buffer
	err := p.exec(ctx, "SET GLOBAL READ_ONLY=1", &outb, &errb)
	return errors.Wrap(err, "set global read_only param to 1")
}

func (p *DatabaseExec) DisableReadonlyExec(ctx context.Context) error {
	var errb, outb bytes.Buffer
	err := p.exec(ctx, "SET GLOBAL READ_ONLY=0", &outb, &errb)
	return errors.Wrap(err, "set global read_only param to 0")
}

func (p *DatabaseExec) IsReadonlyExec(ctx context.Context) (bool, error) {
	rows := []*struct {
		ro int `csv:"ro"`
	}{}
	err := p.query(ctx, "select @@read_only as ro", &rows)
	return rows[0].ro == 1, errors.Wrap(err, "select global read_only param")
}

func (p *DatabaseExec) StartReplicationExec(ctx context.Context, replicaPass string, config ReplicationConfig) error {
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
	err := p.exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrapf(err, "change source for channel %s", config.Source.Name)
	}

	outb.Reset()
	errb.Reset()
	err = p.exec(ctx, fmt.Sprintf(`START REPLICA FOR CHANNEL '%s'`, config.Source.Name), &outb, &errb)
	return errors.Wrapf(err, "start replica for source %s", config.Source.Name)

}

func (p *DatabaseExec) DeleteReplicationSourceExec(ctx context.Context, name, host string, port int) error {
	var errb, outb bytes.Buffer
	q := fmt.Sprintf("SELECT asynchronous_connection_failover_delete_source('%s', '%s', %d, null)", name, host, port)
	err := p.exec(ctx, q, &outb, &errb)
	return errors.Wrap(err, "delete replication source "+name)
}

func (p *DatabaseExec) ProxySQLInstanceStatusExec(ctx context.Context, host string) ([]string, error) {

	rows := []*struct {
		status string `csv:"status"`
	}{}

	q := fmt.Sprintf("SELECT status FROM proxysql_servers WHERE hostname LIKE '%s%%'", host)

	err := p.query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	statuses := []string{}
	for _, row := range rows {
		statuses = append(statuses, row.status)
	}

	return statuses, nil
}

func (p *DatabaseExec) PresentInHostgroupsExec(ctx context.Context, host string) (bool, error) {
	hostgroups := []string{WriterHostgroup, ReaderHostgroup}

	rows := []*struct {
		count int `csv:"count"`
	}{}

	q := fmt.Sprintf(`
		SELECT COUNT(*) FROM mysql_servers
		INNER JOIN mysql_galera_hostgroups ON hostgroup_id IN (%s)
		WHERE hostname LIKE '%s' GROUP BY hostname`, strings.Join(hostgroups, ","), host+"%")

	err := p.query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	if rows[0].count != len(hostgroups) {
		return false, nil
	}
	return true, nil
}

func (p *DatabaseExec) PrimaryHostExec(ctx context.Context) (string, error) {
	rows := []*struct {
		hostname string `csv:"host"`
	}{}

	q := fmt.Sprintf("SELECT hostname FROM runtime_mysql_servers WHERE hostgroup_id = %d", writerID)

	err := p.query(ctx, q, &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	return rows[0].hostname, nil
}

func (p *DatabaseExec) HostnameExec(ctx context.Context) (string, error) {
	rows := []*struct {
		hostname string `csv:"hostname"`
	}{}

	err := p.query(ctx, "SELECT @@hostname hostname", &rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}

	return rows[0].hostname, nil
}

func (p *DatabaseExec) WsrepLocalStateCommentExec(ctx context.Context) (string, error) {
	rows := []*struct {
		variable_name string `csv:"Variable_name"`
		value         string `csv:"Value"`
	}{}

	err := p.query(ctx, "SHOW GLOBAL STATUS LIKE 'wsrep_local_state_comment'", &rows)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return rows[0].value, nil
}

func (p *DatabaseExec) VersionExec(ctx context.Context) (string, error) {
	rows := []*struct {
		version string `csv:"version"`
	}{}

	err := p.query(ctx, "select @@VERSION;", &rows)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return rows[0].version, nil
}
