package pxc

import (
	"context"
	"database/sql"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

const UsingPassErrorMessage = `mysqlbinlog: [Warning] Using a password on the command line interface can be insecure.`

// PXC is a type for working with pxc
type PXC struct {
	db   *sql.DB // handle for work with database
	host string  // host for connection
}

// NewManager return new manager for work with pxc
func NewPXC(addr string, user, pass string) (*PXC, error) {
	var pxc PXC

	config := mysql.NewConfig()
	config.User = user
	config.Passwd = pass
	config.Net = "tcp"
	config.Addr = addr
	config.Params = map[string]string{"interpolateParams": "true"}

	mysqlDB, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, errors.Wrap(err, "cannot connect to host")
	}

	pxc.db = mysqlDB
	pxc.host = addr

	return &pxc, nil
}

// Close is for closing db connection
func (p *PXC) Close() error {
	return p.db.Close()
}

// GetHost returns pxc host
func (p *PXC) GetHost() string {
	return p.host
}

// GetGTIDSet return GTID set by binary log file name
func (p *PXC) GetGTIDSet(ctx context.Context, binlogName string) (string, error) {
	//select name from mysql.func where name='get_gtid_set_by_binlog'
	var existFunc string
	nameRow := p.db.QueryRowContext(ctx, "select name from mysql.func where name='get_gtid_set_by_binlog'")
	err := nameRow.Scan(&existFunc)
	if err != nil && err != sql.ErrNoRows {
		return "", errors.Wrap(err, "get udf name")
	}
	if len(existFunc) == 0 {
		_, err = p.db.ExecContext(ctx, "CREATE FUNCTION get_gtid_set_by_binlog RETURNS STRING SONAME 'binlog_utils_udf.so'")
		if err != nil {
			return "", errors.Wrap(err, "create function")
		}
	}
	var binlogSet string
	row := p.db.QueryRowContext(ctx, "SELECT get_gtid_set_by_binlog(?)", binlogName)
	err = row.Scan(&binlogSet)
	if err != nil && !strings.Contains(err.Error(), "Binary log does not exist") {
		return "", errors.Wrap(err, "scan set")
	}

	return binlogSet, nil
}

type Binlog struct {
	Name      string
	Size      int64
	Encrypted string
	GTIDSet   string
}

// GetBinLogList return binary log files list
func (p *PXC) GetBinLogList(ctx context.Context) ([]Binlog, error) {
	rows, err := p.db.QueryContext(ctx, "SHOW BINARY LOGS")
	if err != nil {
		return nil, errors.Wrap(err, "show binary logs")
	}

	var binlogs []Binlog
	for rows.Next() {
		var b Binlog
		if err := rows.Scan(&b.Name, &b.Size, &b.Encrypted); err != nil {
			return nil, errors.Wrap(err, "scan binlogs")
		}
		binlogs = append(binlogs, b)
	}

	_, err = p.db.ExecContext(ctx, "FLUSH BINARY LOGS")
	if err != nil {
		return nil, errors.Wrap(err, "flush binary logs")
	}

	return binlogs, nil
}

// GetBinLogList return binary log files list
func (p *PXC) GetBinLogNamesList(ctx context.Context) ([]string, error) {
	rows, err := p.db.QueryContext(ctx, "SHOW BINARY LOGS")
	if err != nil {
		return nil, errors.Wrap(err, "show binary logs")
	}
	defer rows.Close()

	var binlogs []string
	for rows.Next() {
		var b Binlog
		if err := rows.Scan(&b.Name, &b.Size, &b.Encrypted); err != nil {
			return nil, errors.Wrap(err, "scan binlogs")
		}
		binlogs = append(binlogs, b.Name)
	}

	return binlogs, nil
}

// GetBinLogName returns name of the binary log file by given GTID set
func (p *PXC) GetBinLogName(ctx context.Context, gtidSet string) (string, error) {
	if len(gtidSet) == 0 {
		return "", nil
	}
	var existFunc string
	nameRow := p.db.QueryRowContext(ctx, "select name from mysql.func where name='get_binlog_by_gtid_set'")
	err := nameRow.Scan(&existFunc)
	if err != nil && err != sql.ErrNoRows {
		return "", errors.Wrap(err, "get udf name")
	}
	if len(existFunc) == 0 {
		_, err = p.db.ExecContext(ctx, "CREATE FUNCTION get_binlog_by_gtid_set RETURNS STRING SONAME 'binlog_utils_udf.so'")
		if err != nil {
			return "", errors.Wrap(err, "create function")
		}
	}
	var binlog sql.NullString
	row := p.db.QueryRowContext(ctx, "SELECT get_binlog_by_gtid_set(?)", gtidSet)

	err = row.Scan(&binlog)
	if err != nil {
		return "", errors.Wrap(err, "scan binlog")
	}

	return strings.TrimPrefix(binlog.String, "./"), nil
}

// GetBinLogFirstTimestamp return binary log file first timestamp
func (p *PXC) GetBinLogFirstTimestamp(ctx context.Context, binlog string) (string, error) {
	var existFunc string
	nameRow := p.db.QueryRowContext(ctx, "select name from mysql.func where name='get_first_record_timestamp_by_binlog'")
	err := nameRow.Scan(&existFunc)
	if err != nil && err != sql.ErrNoRows {
		return "", errors.Wrap(err, "get udf name")
	}
	if len(existFunc) == 0 {
		_, err = p.db.ExecContext(ctx, "CREATE FUNCTION get_first_record_timestamp_by_binlog RETURNS INTEGER SONAME 'binlog_utils_udf.so'")
		if err != nil {
			return "", errors.Wrap(err, "create function")
		}
	}
	var timestamp string
	row := p.db.QueryRowContext(ctx, "SELECT get_first_record_timestamp_by_binlog(?) DIV 1000000", binlog)

	err = row.Scan(&timestamp)
	if err != nil {
		return "", errors.Wrap(err, "scan binlog timestamp")
	}

	return timestamp, nil
}

func (p *PXC) SubtractGTIDSet(ctx context.Context, set, subSet string) (string, error) {
	var result string
	row := p.db.QueryRowContext(ctx, "SELECT GTID_SUBTRACT(?,?)", set, subSet)
	err := row.Scan(&result)
	if err != nil {
		return "", errors.Wrap(err, "scan gtid subtract result")
	}

	return result, nil
}

func getNodesByServiceName(ctx context.Context, pxcServiceName string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "peer-list", "-on-start=/usr/bin/get-pxc-state", "-service="+pxcServiceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.Wrap(err, "get peer-list output")
	}
	return strings.Split(string(out), "node:"), nil
}

func GetPXCFirstHost(ctx context.Context, pxcServiceName string) (string, error) {
	nodes, err := getNodesByServiceName(ctx, pxcServiceName)
	if err != nil {
		return "", errors.Wrap(err, "get nodes by service name")
	}
	sort.Strings(nodes)
	lastHost := ""
	for _, node := range nodes {
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			nodeArr := strings.Split(node, ":")
			lastHost = nodeArr[0]
			break
		}
	}
	if len(lastHost) == 0 {
		return "", errors.New("can't find host")
	}

	return lastHost, nil
}

func GetPXCOldestBinlogHost(ctx context.Context, pxcServiceName, user, pass string) (string, error) {
	nodes, err := getNodesByServiceName(ctx, pxcServiceName)
	if err != nil {
		return "", errors.Wrap(err, "get nodes by service name")
	}

	var oldestHost string
	var oldestTS int64
	for _, node := range nodes {
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			nodeArr := strings.Split(node, ":")
			binlogTime, err := getBinlogTime(ctx, nodeArr[0], user, pass)
			if err != nil {
				log.Printf("ERROR: get binlog time %v", err)
				continue
			}
			if len(oldestHost) == 0 || oldestTS > 0 && binlogTime < oldestTS {
				oldestHost = nodeArr[0]
				oldestTS = binlogTime
			}

		}
	}

	if len(oldestHost) == 0 {
		return "", errors.New("can't find host")
	}

	return oldestHost, nil
}

func getBinlogTime(ctx context.Context, host, user, pass string) (int64, error) {
	db, err := NewPXC(host, user, pass)
	if err != nil {
		return 0, errors.Errorf("creating connection for host %s: %v", host, err)
	}
	defer db.Close()
	list, err := db.GetBinLogNamesList(ctx)
	if err != nil {
		return 0, errors.Errorf("get binlog list for host %s: %v", host, err)
	}
	if len(list) == 0 {
		return 0, errors.Errorf("get binlog list for host %s: no binlogs found", host)
	}
	var binlogTime int64
	for _, binlogName := range list {
		binlogTime, err = getBinlogTimeByName(ctx, db, binlogName)
		if err != nil {
			log.Printf("ERROR: get binlog timestamp for binlog %s host %s: %v", binlogName, host, err)
			continue
		}
		if binlogTime > 0 {
			break
		}
	}
	if binlogTime == 0 {
		return 0, errors.Errorf("get binlog oldest timestamp for host %s: no binlogs timestamp found", host)
	}

	return binlogTime, nil
}

func getBinlogTimeByName(ctx context.Context, db *PXC, binlogName string) (int64, error) {
	ts, err := db.GetBinLogFirstTimestamp(ctx, binlogName)
	if err != nil {
		return 0, errors.Wrap(err, "get binlog first timestamp")
	}
	binlogTime, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "parse timestamp")
	}

	return binlogTime, nil
}

func (p *PXC) DropCollectorFunctions(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, "DROP FUNCTION IF EXISTS get_first_record_timestamp_by_binlog")
	if err != nil {
		return errors.Wrap(err, "drop get_first_record_timestamp_by_binlog function")
	}
	_, err = p.db.ExecContext(ctx, "DROP FUNCTION IF EXISTS get_binlog_by_gtid_set")
	if err != nil {
		return errors.Wrap(err, "drop get_binlog_by_gtid_set function")
	}

	_, err = p.db.ExecContext(ctx, "DROP FUNCTION IF EXISTS get_gtid_set_by_binlog")
	if err != nil {
		return errors.Wrap(err, "drop get_gtid_set_by_binlog function")
	}

	return nil
}
