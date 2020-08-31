package users

import (
	"database/sql"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

type Manager struct {
	db *sql.DB
}

type SysUser struct {
	Name  string   `yaml:"username"`
	Pass  string   `yaml:"password"`
	Hosts []string `yaml:"hosts"`
}

func NewManager(addr string, user, pass string) (Manager, error) {
	var um Manager

	config := mysql.NewConfig()
	config.User = user
	config.Passwd = pass
	config.Net = "tcp"
	config.Addr = addr
	config.Params = map[string]string{"interpolateParams": "true"}

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
	tx, err := u.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	_, err = tx.Exec("CREATE USER IF NOT EXISTS 'operator'@'%' IDENTIFIED BY ?", pass)
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("create operator user: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrap(err, "create operator user")
	}

	_, err = tx.Exec("GRANT ALL ON *.* TO 'operator'@'%' WITH GRANT OPTION")
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("grant operator user: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrap(err, "grant operator user")
	}

	_, err = tx.Exec("FLUSH PRIVILEGES")
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("flush privileges: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrap(err, "flush privileges")
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}

func (u *Manager) UpdateUsersPass(users []SysUser) error {
	tx, err := u.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	for _, user := range users {
		for _, host := range user.Hosts {
			_, err = tx.Exec("ALTER USER ?@? IDENTIFIED BY ?", user.Name, host, user.Pass)
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("update password: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "update password")
			}
		}
	}

	_, err = tx.Exec("FLUSH PRIVILEGES")
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("flush privileges: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrap(err, "flush privileges")
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}

func (u *Manager) UpdateProxyUsers(proxyUsers []SysUser) error {
	tx, err := u.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	for _, user := range proxyUsers {
		switch user.Name {
		case "proxyadmin":
			_, err = tx.Exec("UPDATE global_variables SET variable_value=? WHERE variable_name='admin-admin_credentials'", "proxyadmin:"+user.Pass)
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("update proxy admin password: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "update proxy admin password")
			}
			_, err = tx.Exec("UPDATE global_variables SET variable_value=? WHERE variable_name='admin-cluster_password'", user.Pass)
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("update proxy admin password: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "update proxy admin password")
			}
			_, err = tx.Exec("LOAD ADMIN VARIABLES TO RUNTIME")
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("load to runtime: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "load to runtime")
			}

			_, err = tx.Exec("SAVE ADMIN VARIABLES TO DISK")
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("save to disk: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "save to disk")
			}
		case "monitor":
			_, err = tx.Exec("UPDATE global_variables SET variable_value=? WHERE variable_name='mysql-monitor_password'", user.Pass)
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("update proxy monitor password: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "update proxy monitor password")
			}
			_, err = tx.Exec("LOAD MYSQL VARIABLES TO RUNTIME")
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("load to runtime: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "load to runtime")
			}

			_, err = tx.Exec("SAVE MYSQL VARIABLES TO DISK")
			if err != nil {
				errT := tx.Rollback()
				if errT != nil {
					return errors.Errorf("save to disk: %v, tx rollback: %v", err, errT)
				}
				return errors.Wrap(err, "save to disk")
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}

// Update160MonitorUserGrant grant SERVICE_CONNECTION_ADMIN to monitor user ig version of pxc 8 or more and set his MAX_USER_CONNECTIONS to 100
func (u *Manager) Update160MonitorUserGrant() error {
	tx, err := u.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	_, err = tx.Exec("/*!80015 GRANT SERVICE_CONNECTION_ADMIN ON *.* TO 'monitor'@'%' */")
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("grant service_connection to user monitor: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrapf(err, "grant service_connection to user monitor")
	}

	_, err = tx.Exec("ALTER USER 'monitor'@'%' WITH MAX_USER_CONNECTIONS 100")
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("set max connections to user monitor: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrapf(err, "set max connections to user monitor")
	}

	_, err = tx.Exec("FLUSH PRIVILEGES")
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("flush privileges: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrap(err, "flush privileges")
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}
