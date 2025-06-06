package users

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	Root           = "root"
	Operator       = "operator"
	Monitor        = "monitor"
	Xtrabackup     = "xtrabackup"
	Replication    = "replication"
	ProxyAdmin     = "proxyadmin"
	PMMServer      = "pmmserver"
	PMMServerKey   = "pmmserverkey"
	PMMServerToken = "pmmservertoken"
)

var UserNames = []string{Root, Operator, Monitor, Xtrabackup,
	Replication, ProxyAdmin, PMMServer, PMMServerKey, PMMServerToken}

type Manager struct {
	db *sql.DB
}

type SysUser struct {
	Name  string   `yaml:"username"`
	Pass  string   `yaml:"password"`
	Hosts []string `yaml:"hosts"`
}

type User struct {
	Name  string
	Hosts sets.Set[string]
	DBs   sets.Set[string]

	// Grants holds the grants for each user@host
	Grants map[string][]string
}

func NewManager(addr string, user, pass string, timeout int32) (Manager, error) {
	var um Manager

	timeoutStr := fmt.Sprintf("%ds", timeout)
	config := mysql.NewConfig()
	config.User = user
	config.Passwd = pass
	config.Net = "tcp"
	config.Addr = addr
	config.DBName = "mysql"
	config.Params = map[string]string{
		"interpolateParams": "true",
		"timeout":           timeoutStr,
		"readTimeout":       timeoutStr,
		"writeTimeout":      timeoutStr,
		"tls":               "preferred",
	}

	mysqlDB, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return um, errors.Wrap(err, "cannot connect to any host")
	}

	um.db = mysqlDB

	return um, nil
}

func (u *Manager) Close() error {
	return u.db.Close()
}

func (u *Manager) CreateOperatorUser(pass string) error {
	_, err := u.db.Exec("CREATE USER IF NOT EXISTS 'operator'@'%' IDENTIFIED BY ?", pass)
	if err != nil {
		return errors.Wrap(err, "create operator user")
	}

	_, err = u.db.Exec("GRANT ALL ON *.* TO 'operator'@'%' WITH GRANT OPTION")
	if err != nil {
		return errors.Wrap(err, "grant operator user")
	}

	return nil
}

// UpdateUserPassWithoutDP updates user pass without Dual Password
// feature introduced in MsSQL 8
func (u *Manager) UpdateUserPassWithoutDP(user *SysUser) error {
	if user == nil {
		return nil
	}

	for _, host := range user.Hosts {
		_, err := u.db.Exec("ALTER USER ?@? IDENTIFIED BY ?", user.Name, host, user.Pass)
		if err != nil {
			return errors.Wrap(err, "update password")
		}
	}

	return nil
}

// UpdateUserPass updates user passwords but retains the current password
// using Dual Password feature of MySQL 8.
func (m *Manager) UpdateUserPass(user *SysUser) error {
	if user == nil {
		return nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	for _, host := range user.Hosts {
		_, err = tx.Exec("ALTER USER ?@? IDENTIFIED BY ? RETAIN CURRENT PASSWORD", user.Name, host, user.Pass)
		if err != nil {
			err = errors.Wrap(err, "alter user")

			if errT := tx.Rollback(); errT != nil {
				return errors.Wrap(errors.Wrap(errT, "rollback"), err.Error())
			}

			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}

// DiscardOldPassword discards old passwords of given users
func (m *Manager) DiscardOldPassword(user *SysUser) error {
	if user == nil {
		return nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	for _, host := range user.Hosts {
		_, err = tx.Exec("ALTER USER ?@? DISCARD OLD PASSWORD", user.Name, host)
		if err != nil {
			err = errors.Wrap(err, "alter user")

			if errT := tx.Rollback(); errT != nil {
				return errors.Wrap(errors.Wrap(errT, "rollback"), err.Error())
			}

			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}

// DiscardOldPassword discards old passwords of given users
func (m *Manager) IsOldPassDiscarded(user *SysUser) (bool, error) {
	var attributes sql.NullString
	r := m.db.QueryRow("SELECT User_attributes FROM mysql.user WHERE user=?", user.Name)

	err := r.Scan(&attributes)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, errors.Wrap(err, "select User_attributes field")
	}

	if attributes.Valid {
		return false, nil
	}

	return true, nil
}

func (u *Manager) UpdateProxyUser(user *SysUser) error {
	switch user.Name {
	case ProxyAdmin:
		_, err := u.db.Exec("UPDATE global_variables SET variable_value=? WHERE variable_name='admin-admin_credentials'", "proxyadmin:"+user.Pass)
		if err != nil {
			return errors.Wrap(err, "update proxy admin password")
		}
		_, err = u.db.Exec("UPDATE global_variables SET variable_value=? WHERE variable_name='admin-cluster_password'", user.Pass)
		if err != nil {
			return errors.Wrap(err, "update proxy admin password")
		}
		_, err = u.db.Exec("LOAD ADMIN VARIABLES TO RUNTIME")
		if err != nil {
			return errors.Wrap(err, "load to runtime")
		}

		_, err = u.db.Exec("SAVE ADMIN VARIABLES TO DISK")
		if err != nil {
			return errors.Wrap(err, "save to disk")
		}
	case Monitor:
		_, err := u.db.Exec("UPDATE global_variables SET variable_value=? WHERE variable_name='mysql-monitor_password'", user.Pass)
		if err != nil {
			return errors.Wrap(err, "update proxy monitor password")
		}
		_, err = u.db.Exec("LOAD MYSQL VARIABLES TO RUNTIME")
		if err != nil {
			return errors.Wrap(err, "load to runtime")
		}

		_, err = u.db.Exec("SAVE MYSQL VARIABLES TO DISK")
		if err != nil {
			return errors.Wrap(err, "save to disk")
		}
	}

	return nil
}

// Update160MonitorUserGrant grants SERVICE_CONNECTION_ADMIN rights to the monitor user
// if pxc version is 8 or more and sets the MAX_USER_CONNECTIONS parameter to 100 (empirically determined)
func (u *Manager) Update160MonitorUserGrant(pass string) (err error) {

	_, err = u.db.Exec("CREATE USER IF NOT EXISTS 'monitor'@'%' IDENTIFIED BY ?", pass)
	if err != nil {
		return errors.Wrap(err, "create operator user")
	}

	_, err = u.db.Exec("/*!80015 GRANT SERVICE_CONNECTION_ADMIN ON *.* TO 'monitor'@'%' */")
	if err != nil {
		return errors.Wrapf(err, "grant service_connection to user monitor")
	}

	_, err = u.db.Exec("ALTER USER 'monitor'@'%' WITH MAX_USER_CONNECTIONS 100")
	if err != nil {
		return errors.Wrapf(err, "set max connections to user monitor")
	}

	return nil
}

// Update1150XtrabackupUser grants all needed rights to the xtrabackup user
func (u *Manager) Update1150XtrabackupUser(pass string) (err error) {

	_, err = u.db.Exec("CREATE USER IF NOT EXISTS 'xtrabackup'@'%' IDENTIFIED BY ?", pass)
	if err != nil {
		return errors.Wrap(err, "create operator user")
	}

	_, err = u.db.Exec("GRANT ALL ON *.* TO 'xtrabackup'@'%' WITH GRANT OPTION")
	if err != nil {
		return errors.Wrapf(err, "grant privileges to user xtrabackup")
	}

	return nil
}

// Update1100MonitorUserPrivilege grants system_user privilege for monitor
func (u *Manager) Update1100MonitorUserPrivilege() (err error) {
	if _, err := u.db.Exec("GRANT SYSTEM_USER ON *.* TO 'monitor'@'%'"); err != nil {
		return errors.Wrap(err, "monitor user")
	}

	return nil
}

func (u *Manager) CreateReplicationUser(password string) error {

	_, err := u.db.Exec("CREATE USER IF NOT EXISTS 'replication'@'%' IDENTIFIED BY ?", password)
	if err != nil {
		return errors.Wrap(err, "create replication user")
	}

	_, err = u.db.Exec("GRANT REPLICATION SLAVE ON *.* to 'replication'@'%'")
	if err != nil {
		return errors.Wrap(err, "grant replication user")
	}

	return nil
}

// UpdatePassExpirationPolicy sets user password expiration policy to never
func (u *Manager) UpdatePassExpirationPolicy(user *SysUser) error {
	if user == nil {
		return nil
	}

	for _, host := range user.Hosts {
		_, err := u.db.Exec("ALTER USER ?@? PASSWORD EXPIRE NEVER", user.Name, host)
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *Manager) UpsertUser(ctx context.Context, query []string, pass string) error {
	for _, q := range query {
		var err error
		if strings.Contains(q, "?") {
			_, err = u.db.ExecContext(ctx, q, pass)
		} else {
			_, err = u.db.ExecContext(ctx, q)
		}
		if err != nil {
			return errors.Wrap(err, "exec")
		}
	}

	return nil
}

// GetUsers returns a user stored in the database
func (p *Manager) GetUser(ctx context.Context, user string) (*User, error) {
	u := &User{
		Name:   user,
		Hosts:  sets.New[string](),
		DBs:    sets.New[string](),
		Grants: make(map[string][]string),
	}

	rows, err := p.db.QueryContext(ctx, "SELECT DISTINCT u.Host, d.Db FROM mysql.user u LEFT JOIN mysql.db d ON u.User = d.User WHERE u.User = ?", user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var host string
		var db sql.NullString
		err = rows.Scan(&host, &db)
		if err != nil {
			return nil, err
		}

		if db.Valid {
			u.DBs.Insert(db.String)
		}
		u.Hosts.Insert(host)
	}

	if len(u.Hosts) == 0 {
		return nil, nil
	}

	for host := range u.Hosts {
		rows, err := p.db.QueryContext(ctx, "SHOW GRANTS FOR ?@?", user, host)
		if err != nil {
			return nil, err
		}
		// Plus 1 is for the default grant every user has, which is USAGE.
		grants := make([]string, 0, len(u.DBs)+1)
		for rows.Next() {
			var grant string
			err = rows.Scan(&grant)
			if err != nil {
				return nil, err
			}
			grants = append(grants, grant)
		}

		u.Grants[host] = grants
	}

	return u, nil
}
