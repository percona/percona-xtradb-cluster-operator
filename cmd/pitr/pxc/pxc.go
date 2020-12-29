package pxc

import (
	"database/sql"
	"os/exec"
	"sort"
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
func (p *PXC) GetGTIDSet(binlogName string) (string, error) {
	//select name from mysql.func where name='get_gtid_set_by_binlog'
	var existFunc string
	nameRow := p.db.QueryRow("select name from mysql.func where name='get_gtid_set_by_binlog'")
	err := nameRow.Scan(&existFunc)
	if err != nil && err != sql.ErrNoRows {
		return "", errors.Wrap(err, "get udf name")
	}
	if len(existFunc) == 0 {
		_, err = p.db.Exec("CREATE FUNCTION get_gtid_set_by_binlog RETURNS STRING SONAME 'binlog_utils_udf.so'")
		if err != nil {
			return "", errors.Wrap(err, "create function")
		}
	}
	var binlogSet string
	row := p.db.QueryRow("SELECT get_gtid_set_by_binlog(?)", binlogName)
	err = row.Scan(&binlogSet)
	if err != nil && !strings.Contains(err.Error(), "Binary log does not exist") {
		return "", errors.Wrap(err, "scan set")
	}

	return binlogSet, nil
}

// GetBinLogList return binary log files list
func (p *PXC) GetBinLogList() ([]string, error) {
	var binlogList []string
	rows, err := p.db.Query("SHOW BINARY LOGS")
	if err != nil {
		return nil, errors.Wrap(err, "show binary logs")
	}
	type binlog struct {
		Name      string
		Size      int64
		Encrypted string
	}
	for rows.Next() {
		var b binlog
		if err := rows.Scan(&b.Name, &b.Size, &b.Encrypted); err != nil {
			return nil, errors.Wrap(err, "scan binlogs")
		}
		binlogList = append(binlogList, b.Name)
	}

	_, err = p.db.Exec("FLUSH BINARY LOGS")
	if err != nil {
		return nil, errors.Wrap(err, "flush binary logs")
	}

	return binlogList, nil
}

// GetBinLogName returns name of the binary log file by given GTID set
func (p *PXC) GetBinLogName(gtidSet string) (string, error) {
	if len(gtidSet) == 0 {
		return "", nil
	}
	var existFunc string
	nameRow := p.db.QueryRow("select name from mysql.func where name='get_binlog_by_gtid_set'")
	err := nameRow.Scan(&existFunc)
	if err != nil && err != sql.ErrNoRows {
		return "", errors.Wrap(err, "get udf name")
	}
	if len(existFunc) == 0 {
		_, err = p.db.Exec("CREATE FUNCTION get_binlog_by_gtid_set RETURNS STRING SONAME 'binlog_utils_udf.so'")
		if err != nil {
			return "", errors.Wrap(err, "create function")
		}
	}
	var binlog string
	row := p.db.QueryRow("SELECT get_binlog_by_gtid_set(?)", gtidSet)

	err = row.Scan(&binlog)
	if err != nil {
		return "", errors.Wrap(err, "scan binlog")
	}

	return strings.TrimPrefix(binlog, "./"), nil
}

// GetBinLogFirstTimestamp return binary log file first timestamp
func (p *PXC) GetBinLogFirstTimestamp(binlog string) (string, error) {
	var existFunc string
	nameRow := p.db.QueryRow("select name from mysql.func where name='get_first_record_timestamp_by_binlog'")
	err := nameRow.Scan(&existFunc)
	if err != nil && err != sql.ErrNoRows {
		return "", errors.Wrap(err, "get udf name")
	}
	if len(existFunc) == 0 {
		_, err = p.db.Exec("CREATE FUNCTION get_first_record_timestamp_by_binlog RETURNS INTEGER SONAME 'binlog_utils_udf.so'")
		if err != nil {
			return "", errors.Wrap(err, "create function")
		}
	}
	var timestamp string
	row := p.db.QueryRow("SELECT get_first_record_timestamp_by_binlog(?) DIV 1000000", binlog)

	err = row.Scan(&timestamp)
	if err != nil {
		return "", errors.Wrap(err, "scan binlog timestamp")
	}

	return timestamp, nil
}

func (p *PXC) IsGTIDSubset(subSet, gtidSet string) (bool, error) {
	var isSubset bool
	row := p.db.QueryRow("SELECT GTID_SUBSET(?,?)", subSet, gtidSet)

	err := row.Scan(&isSubset)
	if err != nil {
		return false, errors.Wrap(err, "scan binlog timestamp")
	}

	return isSubset, nil
}

func GetPXCLastHost(pxcServiceName string) (string, error) {
	cmd := exec.Command("peer-list", "-on-start=/usr/bin/get-pxc-state", "-service="+pxcServiceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, "get output")
	}
	nodes := strings.Split(string(out), "node:")
	sort.Strings(nodes)
	lastHost := ""
	for _, node := range nodes {
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			nodeArr := strings.Split(node, ":")
			lastHost = nodeArr[0]
		}
	}
	if len(lastHost) == 0 {
		return "", errors.New("cant find host")
	}

	return lastHost, nil
}

func (p *PXC) DropCollectorFunctions() error {
	_, err := p.db.Exec("DROP FUNCTION IF EXISTS get_first_record_timestamp_by_binlog")
	if err != nil {
		return errors.Wrap(err, "drop get_first_record_timestamp_by_binlog function")
	}
	_, err = p.db.Exec("DROP FUNCTION IF EXISTS get_binlog_by_gtid_set")
	if err != nil {
		return errors.Wrap(err, "drop get_binlog_by_gtid_set function")
	}

	_, err = p.db.Exec("DROP FUNCTION IF EXISTS get_gtid_set_by_binlog")
	if err != nil {
		return errors.Wrap(err, "drop get_gtid_set_by_binlog function")
	}

	return nil
}
