package users

import (
	"database/sql"

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

func NewManager(host string, rootPass string) (Manager, error) {
	var um Manager

	mysqlDB, err := sql.Open("mysql", "root:"+rootPass+"@tcp("+host+")/?interpolateParams=true")
	if err != nil {
		return um, errors.Wrap(err, "cannot connect to any host")
	}
	um.db = mysqlDB

	return um, nil
}

func (u *Manager) UpdateUsersPass(users []SysUser) error {
	defer u.db.Close()
	tx, err := u.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}

	_, err = tx.Exec("FLUSH PRIVILEGES")
	if err != nil {
		errT := tx.Rollback()
		if errT != nil {
			return errors.Errorf("flush privileges: %v, tx rollback: %v", err, errT)
		}
		return errors.Wrap(err, "flush privileges")
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
