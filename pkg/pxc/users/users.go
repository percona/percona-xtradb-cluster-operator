package users

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

const (
	Root         = "root"
	Operator     = "operator"
	Monitor      = "monitor"
	Xtrabackup   = "xtrabackup"
	Replication  = "replication"
	ProxyAdmin   = "proxyadmin"
	PMMServer    = "pmmserver"
	PMMServerKey = "pmmserverkey"
)

var UserNames = []string{Root, Operator, Monitor, Xtrabackup,
	Replication, ProxyAdmin, PMMServer, PMMServerKey}

type Manager struct {
	db *sql.DB
}

type SysUser struct {
	Name  string   `yaml:"username"`
	Pass  string   `yaml:"password"`
	Hosts []string `yaml:"hosts"`
}

type User struct {
	Name string `db:"User"`
	Host string `db:"Host"`
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

func (u *Manager) Exec(ctx context.Context, query []string, args ...any) error {
	println("EEEEEEEEEEEEEEE executing query: ", query)
	println("EEEEEEEEEEEEEEE executing ARGS: ", args)

	// queries := []string{
	// 	"CREATE DATABASE IF NOT EXISTS inel",
	// 	"CREATE USER IF NOT EXISTS 'my-usereeeeeeee'@'localhost' IDENTIFIED BY 'password'",
	// 	"GRANT ALL PRIVILEGES ON inel.* TO 'my-usereeeeeeee'@'localhost'",
	// }

	// how to properly pass multiple query statements to ExecContext?
	for _, q := range query {
		_, err := u.db.ExecContext(ctx, q, args...)
		if err != nil {
			println("EEEEEEEEEEEEEEE error: ", err.Error())
		}else {
			println("EEEEEEEEEEEEEEE success")
		}
	}

	// println("EEEEEEEEEEEEEEE done")
	// _, err := u.db.ExecContext(ctx, query, args...)
	// if err != nil {
	// 	return errors.Wrap(err, "exec query")
	// }

	return nil
}

// GetUsers returns a list of user@host for a given user
func (p *Manager) GetUsers(ctx context.Context, user string) ([]User, error) {
	rows, err := p.db.QueryContext(ctx, "SELECT User,Host FROM mysql.user WHERE User = ?", user)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	users := make([]User, 0)

	for rows.Next() {
		var u User

		err = rows.Scan(&u.Name, &u.Host)
		if err != nil {
			return nil, err
		}

		users = append(users, u)
	}

	return users, nil
}
