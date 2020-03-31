package manager

import (
	"database/sql"
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
	Drop   bool    `yaml:"grop"`
	Name   string  `yaml:"username"`
	Pass   string  `yaml:"password"`
	Tables []Table `yaml:"tables"`
	Host   string  `yaml:"host"`
}

type Table struct {
	Name       string `yaml:"name"`
	Privileges string `yaml:"privileges"`
}

func New(hosts []string, rootPass, secretPath string) (Manager, error) {
	var um Manager
	var err error
	for _, host := range hosts {
		mysqlDB, err := sql.Open("mysql", "root:"+rootPass+"@tcp("+host+")/?interpolateParams=true")
		if err != nil {
			log.Println(errors.Wrap(err, "create db connection"))
			continue
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
	tx, err := u.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}
	for _, user := range u.Users {
		log.Println("drop user", user.Name)
		_, err = tx.Exec("DROP USER IF EXISTS ?@?", user.Name, user.Host)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "drop user with query")
		}
		if user.Drop {
			continue
		}
		_, err = tx.Exec("CREATE USER ?@? IDENTIFIED BY ?", user.Name, user.Host, user.Pass)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "cretae user")
		}
		for _, table := range user.Tables {
			log.Println("grant privileges for user ", user.Name)
			err = grant(user, table, user.Host, tx)
			if err != nil {
				return errors.Wrap(err, "grant privileges")
			}
		}
		log.Println("flush privileges for user ", user.Name)
		_, err = tx.Exec("FLUSH PRIVILEGES")
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "flush privileges")
		}
	}
	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit transaction")
	}
	return nil
}

func grant(user User, table Table, host string, tx *sql.Tx) error {
	_, err := tx.Exec(`SET @username = ?`, user.Name)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "set usernmae")
	}
	_, err = tx.Exec(`SET @userhost = ?`, host)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "set host")
	}
	_, err = tx.Exec(`SET @table = ?`, table.Name)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "set table")
	}
	_, err = tx.Exec(`SET @priveleges = ?`, table.Privileges)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "set priveleges")
	}
	_, err = tx.Exec(`SET @grantUser = CONCAT('GRANT ',@priveleges,' ON ',@table,' TO "',@username,'"@"',@userhost,'" ')`)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "create grant user")
	}
	_, err = tx.Exec(`PREPARE st FROM @grantUser`)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "prepare grant")
	}
	_, err = tx.Exec(`EXECUTE st`)
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "exec grant st")
	}

	return nil
}
