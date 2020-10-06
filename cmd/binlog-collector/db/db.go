package db

import (
	"bytes"
	"database/sql"
	"os/exec"
	"strings"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

// Manager is a type for working with pxc
type Manager struct {
	db     *sql.DB      // using for work with PXC database
	config mysql.Config // config with data for connection to PXC database
}

// NewManager return new manager for work with pxc
func NewManager(addr string, user, pass string) (*Manager, error) {
	var um Manager

	config := mysql.NewConfig()
	config.User = user
	config.Passwd = pass
	config.Net = "tcp"
	config.Addr = addr
	config.Params = map[string]string{"interpolateParams": "true"}

	mysqlDB, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return &um, errors.Wrap(err, "cannot connect to host")
	}

	um.db = mysqlDB
	um.config = *config

	return &um, nil
}

// Close is for closing db connection
func (m *Manager) Close() error {
	return m.db.Close()
}

// GetGTIDSetByBinLog return GTID set by binary log file name
func (m *Manager) GetGTIDSetByBinLog(binlogName string) (string, error) {
	_, err := m.db.Exec("DROP FUNCTION get_gtid_set_by_binlog")
	if err != nil && !strings.Contains(err.Error(), "does not exist") {
		return "", errors.Wrap(err, "drop function")
	}
	_, err = m.db.Exec("CREATE FUNCTION get_gtid_set_by_binlog RETURNS STRING SONAME 'binlog_utils_udf.so'")
	if err != nil {
		return "", errors.Wrap(err, "create function")
	}

	var binlogSet string
	binlogName = "./" + binlogName
	rows, err := m.db.Query("SELECT get_gtid_set_by_binlog(?)", binlogName)
	if err != nil {
		return "", errors.Wrap(err, "select gtid set")
	}
	for rows.Next() {
		if err := rows.Scan(&binlogSet); err != nil {
			return "", errors.Wrap(err, "scan set")
		}
	}

	return binlogSet, nil
}

// GetBinLogFilesList return binary log files list
func (m *Manager) GetBinLogFilesList() ([]string, error) {
	var binlogList []string
	rows, err := m.db.Query("SHOW BINARY LOGS")
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

// GetBinLogNameByGTIDSet return name og binary log file by passed GTID set
func (m *Manager) GetBinLogNameByGTIDSet(gtidSet string) (string, error) {
	_, err := m.db.Exec("DROP FUNCTION get_binlog_by_gtid_set")
	if err != nil && !strings.Contains(err.Error(), "does not exist") {
		return "", errors.Wrap(err, "drop function")
	}
	_, err = m.db.Exec("CREATE FUNCTION get_binlog_by_gtid_set RETURNS STRING SONAME 'binlog_utils_udf.so'")
	if err != nil {
		return "", errors.Wrap(err, "create function")
	}

	var binlog string
	rows, err := m.db.Query("SELECT get_binlog_by_gtid_set(?)", gtidSet)
	if err != nil {
		return "", errors.Wrap(err, "select binlog by set")
	}
	for rows.Next() {
		if err := rows.Scan(&binlog); err != nil {
			return "", errors.Wrap(err, "scan binlog")
		}
	}

	return strings.Replace(binlog, "./", "", -1), nil
}

// GetBinLogFileContent return content of given binary log file
func (m *Manager) GetBinLogFileContent(binlogName string) ([]byte, error) {
	cmnd := exec.Command("mysqlbinlog", "-R", "-h"+m.config.Addr, "-u"+m.config.User, "-p"+m.config.Passwd, binlogName)
	var stdout, stderr bytes.Buffer
	cmnd.Stdout = &stdout
	cmnd.Stderr = &stderr
	err := cmnd.Run()
	if err != nil {
		return nil, errors.Wrap(err, "run mysqlbinlog command")
	}
	if stderr.Bytes() != nil && !strings.Contains(stderr.String(), "Using a password on the command line") {
		return nil, errors.New(stderr.String())
	}

	return stdout.Bytes(), nil
}
