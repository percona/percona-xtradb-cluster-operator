package db

import (
	"database/sql"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

const UsingPassErrorMessage = `mysqlbinlog: [Warning] Using a password on the command line interface can be insecure.
`

type DB interface {
	Close() error
	GetGTIDSet(binlogName string) (string, error)
	GetBinLogList() ([]string, error)
	GetBinLogName(gtidSet string) (string, error)
	GetHost() string
}

// PXC is a type for working with pxc
type PXC struct {
	db   *sql.DB // handle for work with database
	host string  //config mysql.Config // config with data for connection to PXC database
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
		return &pxc, errors.Wrap(err, "cannot connect to host")
	}

	pxc.db = mysqlDB
	pxc.host = addr

	return &pxc, nil
}

// Close is for closing db connection
func (p *PXC) Close() error {
	return p.db.Close()
}

func (p *PXC) GetHost() string {
	return p.host
}

// GetGTIDSet return GTID set by binary log file name
func (p *PXC) GetGTIDSet(binlogName string) (string, error) {
	_, err := p.db.Exec("DROP FUNCTION get_gtid_set_by_binlog")
	if err != nil && !strings.Contains(err.Error(), "does not exist") {
		return "", errors.Wrap(err, "drop function")
	}
	_, err = p.db.Exec("CREATE FUNCTION get_gtid_set_by_binlog RETURNS STRING SONAME 'binlog_utils_udf.so'")
	if err != nil {
		return "", errors.Wrap(err, "create function")
	}

	var binlogSet string
	binlogName = "./" + binlogName
	row := p.db.QueryRow("SELECT get_gtid_set_by_binlog(?)", binlogName)
	err = row.Scan(&binlogSet)
	if err != nil {
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

	return binlogList, nil
}

// GetBinLogName return name og binary log file by passed GTID set
func (p *PXC) GetBinLogName(gtidSet string) (string, error) {
	_, err := p.db.Exec("DROP FUNCTION get_binlog_by_gtid_set")
	if err != nil && !strings.Contains(err.Error(), "does not exist") {
		return "", errors.Wrap(err, "drop function")
	}
	_, err = p.db.Exec("CREATE FUNCTION get_binlog_by_gtid_set RETURNS STRING SONAME 'binlog_utils_udf.so'")
	if err != nil {
		return "", errors.Wrap(err, "create function")
	}

	var binlog string
	row := p.db.QueryRow("SELECT get_binlog_by_gtid_set(?)", gtidSet)
	if err != nil {
		return "", errors.Wrap(err, "select binlog by set")
	}

	err = row.Scan(&binlog)
	if err != nil {
		return "", errors.Wrap(err, "scan binlog")
	}

	return strings.TrimPrefix(binlog, "./"), nil
}
