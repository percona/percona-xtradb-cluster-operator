package manager

import (
	"database/sql"
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/pkg/errors"
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
	secret, err := u.readSecretFile()
	if err != nil {
		return errors.Wrap(err, "read secret")
	}
	var data Data
	err = yaml.Unmarshal(secret, &data)
	if err != nil {
		return errors.Wrap(err, "unmarshal secret")
	}
	u.Users = data.Users

	return nil
}

func (u *Manager) readSecretFile() ([]byte, error) {
	file, err := os.Open(u.secretPath)
	if err != nil {
		return nil, errors.Wrap(err, "open secret file")
	}
	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, errors.Wrap(err, "read secret file")
	}

	return b, nil
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
			_, err = u.db.Exec("DROP USER [IF EXISTS] '?'@'?'", user.Name, host)
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "drop user")
			}

			log.Println("create user", user.Name)
			_, err = u.db.Exec("CREATE USER '?'@'?' IDENTIFIED BY '?'", user.Name, user.Pass, host)
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "create user")
			}

			for _, table := range user.Tables {
				log.Println("grant privileges for user ", user.Name)
				_, err = u.db.Exec("GRANT ? ON ? TO '?'@'?'", table.Privileges, table.Name, user.Name, host)
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
