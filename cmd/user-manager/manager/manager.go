package manager

import (
	"database/sql"
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/user-manager/db"
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

func New(hosts []string, rootPass string) (Manager, error) {
	var um Manager
	var err error
	for _, host := range hosts {
		mysqlDB, err := db.Conn("root:" + rootPass + "@tcp(" + host + ")/")
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
	um.secretPath = "./secret.yaml"

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
	for _, user := range u.Users {
		for _, host := range user.Hosts {
			log.Println("drop user", user.Name)
			_, err := u.db.Exec(db.DropUser(user.Name, host))
			if err != nil {
				// no need to return because can be no user yet
				log.Println(errors.Wrap(err, "drop user"))
			}
			log.Println("create user", user.Name)
			_, err = u.db.Exec(db.CreateUser(user.Name, user.Pass, host))
			if err != nil {
				return errors.Wrap(err, "create user")
			}
			for _, table := range user.Tables {
				log.Println("grant privileges for user ", user.Name)
				_, err = u.db.Exec(db.GrantUser(table.Privileges, table.Name, user.Name, host))
				if err != nil {
					return errors.Wrap(err, "grant privileges")
				}
				log.Println("flush privileges for user ", user.Name)
				_, err = u.db.Exec(db.FlushPrivileges())
				if err != nil {
					return errors.Wrap(err, "flush privileges")
				}
			}
		}
	}

	return nil
}
