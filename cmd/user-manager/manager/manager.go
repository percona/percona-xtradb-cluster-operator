package manager

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Manager struct {
	db         *sql.DB
	secretPath string
	Users      []User
}

type UsersSecret struct {
	StringData map[string]string `yaml:"stringData"`
}

type Data struct {
	Users []User `yaml:"users"`
}

type User struct {
	Name   string   `yaml:"username"`
	Pass   string   `yaml:"password"`
	Tables []Table  `yaml:"tables"`
	Hosts  []string `yaml:"hosts"`
}

type Table struct {
	Name       string `yaml:"name"`
	Privileges string `yaml:"privileges"`
}

func New(hosts []string, rootPass, secretPath string) (Manager, error) {
	var um Manager
	var err error
	for _, host := range hosts {
		mysqlDB, err := sql.Open("mysql", "root:"+rootPass+"@tcp("+host+")/")
		if err != nil {
			return um, errors.Wrap(err, "create db connection")
		}
		um.db = mysqlDB
		log.Println("using  host: " + host)
		break
	}
	if um.db == nil {
		return um, errors.Wrap(err, "cannot connect to any host")
	}
	um.secretPath = "./data/secret.yaml"
	if len(secretPath) > 0 {
		um.secretPath = secretPath
	}
	return um, nil
}

func (u *Manager) GetUsers() error {
	file, err := os.Open(u.secretPath)
	if err != nil {
		return errors.Wrap(err, "open secret file")
	}
	var data Data
	err = yaml.NewDecoder(file).Decode(&data)
	if err != nil {
		return errors.Wrap(err, "unmarshal secret")
	}
	u.Users = data.Users

	return nil
}

func (u *Manager) ManageUsers() error {
	defer u.db.Close()
	for _, user := range u.Users {
		for _, host := range user.Hosts {
			tx, err := u.db.Begin()
			if err != nil {
				return errors.Wrap(err, "begin transaction")
			}
			log.Println("drop user", user.Name)
			_, err = u.db.Exec(fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s'", user.Name, host))
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "drop user")
			}

			log.Println("create user", user.Name)
			_, err = u.db.Exec(fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s'", user.Name, host, user.Pass))
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "create user")
			}

			for _, table := range user.Tables {
				log.Println("grant privileges for user ", user.Name)
				_, err = u.db.Exec(fmt.Sprintf("GRANT %s ON %s TO '%s'@'%s'", table.Privileges, table.Name, user.Name, host))
				if err != nil {
					tx.Rollback()
					return errors.Wrap(err, "grant privileges")
				}
			}
			log.Println("flush privileges for user ", user.Name)
			_, err = u.db.Exec("FLUSH PRIVILEGES")
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "flush privileges")
			}

			err = tx.Commit()
			if err != nil {
				return errors.Wrap(err, "commit transaction")
			}
		}
	}

	return nil
}
