package pxc

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
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
	config.Addr = addr + ":33062"
	config.Params = map[string]string{
		"interpolateParams": "true",
		"tls":               "preferred",
		"multiStatements":   "true",
	}
	config.DBName = "mysql"

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
	var binlogSet string

	scan := func() error {
		row := p.db.QueryRowContext(ctx, "SELECT get_gtid_set_by_binlog(?)", binlogName)
		if err := row.Scan(&binlogSet); err != nil && !strings.Contains(err.Error(), "Binary log does not exist") {
			return errors.Wrap(err, "scan set")
		}
		return nil
	}

	if err := scan(); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			if cerr := p.CreateCollectorFunctions(ctx); cerr != nil {
				return "", stderrors.Join(err, cerr)
			}

			return binlogSet, scan()
		}
		return "", errors.Wrap(err, "scan binlog timestamp")
	}

	return binlogSet, nil
}

type Binlog struct {
	Name      string
	Size      int64
	Encrypted string
	GTIDSet   GTIDSet
}

func (b Binlog) String() string {
	return fmt.Sprintf("%s (%d bytes) [E:%s]: %s", b.Name, b.Size, b.Encrypted, b.GTIDSet.Raw())
}

type GTIDSet struct {
	gtidSet string
}

func NewGTIDSet(gtidSet string) GTIDSet {
	return GTIDSet{gtidSet: gtidSet}
}

func (s *GTIDSet) IsEmpty() bool {
	return s == nil || len(s.gtidSet) == 0
}

func (s *GTIDSet) Raw() string {
	return s.gtidSet
}

func (s *GTIDSet) List() []string {
	if len(s.gtidSet) == 0 {
		return nil
	}
	list := strings.Split(s.gtidSet, ",")
	sort.Strings(list)
	return list
}

func (p *PXC) GetVersion(ctx context.Context) (string, error) {
	var version string

	if err := p.db.QueryRowContext(ctx, "select @@VERSION").Scan(&version); err != nil {
		return "", errors.Wrap(err, "select @@VERSION")
	}

	return version, nil
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

func (p *PXC) WsrepClusterStateUUID() (string, error) {
	var variable_name string
	var value string

	err := p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'wsrep_cluster_state_uuid'").Scan(&variable_name, &value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return value, nil
}

func (p *PXC) GTIDSubset(ctx context.Context, set1, set2 string) (bool, error) {
	row := p.db.QueryRowContext(ctx, "SELECT GTID_SUBSET(?,?)", set1, set2)
	var result int
	if err := row.Scan(&result); err != nil {
		return false, errors.Wrap(err, "scan result")
	}

	return result == 1, nil
}

// GetBinLogFirstTimestamp return binary log file first timestamp
func (p *PXC) GetBinLogFirstTimestamp(ctx context.Context, binlog string) (string, error) {
	var timestamp string
	scan := func() error {
		row := p.db.QueryRowContext(ctx, "SELECT get_first_record_timestamp_by_binlog(?) DIV 1000000", binlog)
		err := row.Scan(&timestamp)
		return errors.Wrap(err, "scan binlog timestamp")
	}
	if err := scan(); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			if cerr := p.CreateCollectorFunctions(ctx); cerr != nil {
				return "", stderrors.Join(err, cerr)
			}

			return timestamp, scan()
		}
		return "", errors.Wrap(err, "scan binlog timestamp")
	}

	return timestamp, nil
}

// GetBinLogLastTimestamp return binary log file last timestamp
func (p *PXC) GetBinLogLastTimestamp(ctx context.Context, binlog string) (string, error) {
	var timestamp string
	scan := func() error {
		row := p.db.QueryRowContext(ctx, "SELECT get_last_record_timestamp_by_binlog(?) DIV 1000000", binlog)
		err := row.Scan(&timestamp)
		return errors.Wrap(err, "scan binlog timestamp")
	}

	if err := scan(); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			if cerr := p.CreateCollectorFunctions(ctx); cerr != nil {
				return "", stderrors.Join(err, cerr)
			}

			return timestamp, scan()
		}
		return "", errors.Wrap(err, "scan binlog timestamp")
	}

	return timestamp, nil
}

func (p *PXC) SubtractGTIDSet(ctx context.Context, set, subSet string) (string, error) {
	var result string
	row := p.db.QueryRowContext(ctx, "SELECT GTID_SUBTRACT(?,?)", set, subSet)

	if err := row.Scan(&result); err != nil {
		return "", errors.Wrap(err, "scan gtid subtract result")
	}

	return result, nil
}

func GetNodesByServiceName(ctx context.Context, pxcServiceName string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "/opt/percona/peer-list", "-on-start=/opt/percona/get-pxc-state.sh", "-service="+pxcServiceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "run peer-list output: %s", out)
	}
	return strings.Split(string(out), "node:"), nil
}

func GetPXCFirstHost(ctx context.Context, pxcServiceName string) (string, error) {
	nodes, err := GetNodesByServiceName(ctx, pxcServiceName)
	if err != nil {
		return "", errors.Wrap(err, "get nodes by service name")
	}
	sort.Strings(nodes)
	lastHost := ""
	for _, node := range nodes {
		log.Printf("PXC Node: %s", node)
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			nodeArr := strings.Split(node, ":")
			lastHost = nodeArr[0]
			break
		}
	}
	if len(lastHost) == 0 {
		return "", errors.New("can't find host")
	}

	log.Printf("connecting to %s", lastHost)

	return lastHost, nil
}

func GetPXCOldestBinlogHost(ctx context.Context, pxcServiceName, user, pass string) (string, error) {
	nodes, err := GetNodesByServiceName(ctx, pxcServiceName)
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
				log.Printf("ERROR: get binlog time: %v", err)
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

func (p *PXC) InstallBinlogUDFComponent(ctx context.Context) error {
	var urn string
	component := p.db.QueryRowContext(ctx, "SELECT component_urn FROM mysql.component WHERE component_urn = 'file://component_binlog_utils_udf'")
	if err := component.Scan(&urn); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.Wrap(err, "get component_binlog_utils_udf")
	}

	if len(urn) > 0 {
		log.Printf("file://component_binlog_utils_udf is already installed")
		return nil
	}

	_, err := p.db.ExecContext(ctx, "SET SESSION wsrep_on = OFF; INSTALL COMPONENT 'file://component_binlog_utils_udf'; SET SESSION wsrep_on = ON;")
	if err != nil {
		return errors.Wrap(err, "install component")
	}

	return nil
}

func (p *PXC) UninstallBinlogUDFComponent(ctx context.Context) error {
	var urn string
	component := p.db.QueryRowContext(ctx, "SELECT component_urn FROM mysql.component WHERE component_urn = 'file://component_binlog_utils_udf'")
	if err := component.Scan(&urn); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.Wrap(err, "get component_binlog_utils_udf")
	}

	if len(urn) == 0 {
		log.Printf("file://component_binlog_utils_udf is already uninstalled")
		return nil
	}

	_, err := p.db.ExecContext(ctx, "SET SESSION wsrep_on = OFF; UNINSTALL COMPONENT 'file://component_binlog_utils_udf'; SET SESSION wsrep_on = ON;")
	if err != nil {
		return errors.Wrap(err, "uninstall component")
	}

	return nil
}

func collectorFunctions() map[string]string {
	return map[string]string{
		"get_last_record_timestamp_by_binlog":  "INTEGER",
		"get_gtid_set_by_binlog":               "STRING",
		"get_first_record_timestamp_by_binlog": "INTEGER",
	}
}

func (p *PXC) CreateCollectorFunctions(ctx context.Context) error {
	for functionName, returnType := range collectorFunctions() {
		var x int
		err := p.db.QueryRowContext(ctx, `SELECT 1 FROM mysql.func WHERE name = ? LIMIT 1`, functionName).Scan(&x)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Wrapf(err, "check if function %s exists", functionName)
		}
		if err == nil {
			log.Printf("function %s already exists", functionName)
			continue
		}

		log.Printf("Creating %s function on %s node", functionName, p.GetHost())
		createQ := fmt.Sprintf("SET SESSION wsrep_on = OFF; CREATE FUNCTION IF NOT EXISTS %s RETURNS %s SONAME 'binlog_utils_udf.so'; SET SESSION wsrep_on = ON;", functionName, returnType)
		if _, err := p.db.ExecContext(ctx, createQ); err != nil {
			return errors.Wrapf(err, "create function %s", functionName)
		}
	}

	return nil
}

func (p *PXC) DropCollectorFunctions(ctx context.Context) error {
	for functionName := range collectorFunctions() {
		dropQ := fmt.Sprintf("SET SESSION wsrep_on = OFF; DROP FUNCTION IF EXISTS %s; SET SESSION wsrep_on = ON;", functionName)
		if _, err := p.db.ExecContext(ctx, dropQ); err != nil {
			return errors.Wrapf(err, "create function %s", functionName)
		}
	}

	return nil
}
