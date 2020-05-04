package users

import (
	"database/sql"
	"encoding/json"

	"log"

	"regexp"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

type Manager struct {
	db                 *sql.DB
	secretPath         string
	internalSecretPath string
	Users              []User
	InternalUsers      []InternalUser
	Roles              map[string]Role
	Passwords          map[string]string
}

type UsersSecret struct {
	StringData map[string]string `yaml:"stringData"`
}

type Data struct {
	Users []User `yaml:"users"`
	Roles []Role `yaml:"roles"`
}

type User struct {
	Role   string   `yaml:"role"`
	Name   string   `yaml:"username"`
	Pass   string   `yaml:"password"`
	Tables []Table  `yaml:"tables"`
	Hosts  []string `yaml:"hosts"`
}

type Table struct {
	Name       string `yaml:"name"`
	Privileges string `yaml:"privileges"`
}

type Role struct {
	Name   string  `yaml:"name"`
	Tables []Table `yaml:"tables"`
}

type InternalUser struct {
	Name   string `json:"name"`
	Owner  string `json:"owner"`
	Time   int64  `json:"time"`
	Status string `json:"status"`
}

func NewManager(hosts []string, rootPass string) (Manager, error) {
	var um Manager
	var err error
	for _, host := range hosts {
		mysqlDB, err := sql.Open("mysql", "root:"+rootPass+"@tcp("+host+")/?interpolateParams=true")
		if err != nil {
			log.Println(errors.Wrap(err, "create db connection"))
			continue
		}
		um.db = mysqlDB
		break
	}
	if um.db == nil {
		return um, errors.Wrap(err, "cannot connect to any host")
	}
	um.secretPath = "./data/grants.yaml"

	um.internalSecretPath = "./internal-data/users"

	um.Roles = make(map[string]Role)
	um.Passwords = make(map[string]string)

	return um, nil
}

func (u *Manager) GetUsersData(externalSecret, internalSecret corev1.Secret) error {
	var data Data

	extSecretData, ok := externalSecret.Data["grants.yaml"]
	if !ok {
		return errors.New("no grants data")
	}
	if ok {
		err := yaml.Unmarshal(extSecretData, &data)
		if err != nil {
			return errors.Wrap(err, "unmarshal secret")
		}
		u.Users = data.Users
		for _, r := range data.Roles {
			u.Roles[r.Name] = r
		}
	}

	intSecretData, ok := internalSecret.Data["users"]
	var internalData []InternalUser
	err := json.Unmarshal(intSecretData, &internalData)
	if err != nil {
		return errors.Wrap(err, "unmarshal internal secret")
	}
	u.Users = data.Users
	u.InternalUsers = internalData

	return nil
}

func (u *Manager) ManageUsers(externalSecretName string) error {
	defer u.db.Close()
	tx, err := u.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}
	for _, user := range u.Users {
		for _, host := range user.Hosts {
			_, err = tx.Exec("DROP USER IF EXISTS ?@?", user.Name, host)
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "drop user with query")
			}
			_, err = tx.Exec("CREATE USER ?@? IDENTIFIED BY ?", user.Name, host, u.Passwords[user.Name])
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "cretae user")
			}
			for _, table := range user.Tables {
				err = u.grant(user, table, host, tx)
				if err != nil {
					return errors.Wrapf(err, "grant privileges%s", user.Name)
				}
			}
			if len(user.Role) > 0 {
				if _, ok := u.Roles[user.Role]; ok {
					for _, table := range u.Roles[user.Role].Tables {
						err = u.grant(user, table, host, tx)
						if err != nil {
							return errors.Wrapf(err, "grant privileges for user %s", user.Name)
						}
					}
				}
			}
			_, err = tx.Exec("FLUSH PRIVILEGES")
			if err != nil {
				tx.Rollback()
				return errors.Wrap(err, "flush privileges")
			}
		}
	}

	oldUsers := []InternalUser{}
	for _, oldUser := range u.InternalUsers {
		exist := false
		for _, newUser := range u.Users {
			for _, host := range newUser.Hosts {
				if newUser.Name+"@"+host == oldUser.Name {
					exist = true
				}

			}
		}
		if !exist && oldUser.Owner == externalSecretName {
			oldUsers = append(oldUsers, oldUser)
		}
	}
	for _, oldUser := range oldUsers {
		_, err = tx.Exec("DROP USER IF EXISTS ?", oldUser.Name)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(err, "drop user with query")
		}
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}

func (u *Manager) grant(user User, table Table, host string, tx *sql.Tx) error {
	privStr, err := getPrivilegesString(table.Privileges)
	if err != nil {
		return errors.Wrap(err, "check provoleges")
	}
	if !tableNameCorrect(table.Name) {
		return errors.Wrap(err, "table name incorrect")
	}
	_, err = tx.Exec(`GRANT `+privStr+` ON `+table.Name+` TO ?@?`, user.Name, u.Passwords[user.Name])
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "grant privileges")
	}

	return nil
}

func getPrivilegesString(income string) (string, error) {
	privileges := map[string]string{
		"all privileges": "ALL PRIVILEGES",
		"delete":         "DELETE",
		"insert":         "INSERT",
		"select":         "SELECT",
		"update":         "UPDATE",
	}
	privStr := ""
	privArr := strings.Split(income, ",")
	switch len(privArr) {
	case 0:
		return "", errors.New("privileges not set")
	case 1:
		if priv, ok := privileges[strings.ToLower(privArr[0])]; ok {
			return priv, nil
		}
	default:

		for k, incomePriv := range privArr {
			priv, ok := privileges[strings.Replace(strings.ToLower(incomePriv), " ", "", -1)]
			if !ok {
				return "", errors.New("incorrect privilege " + priv)
			}
			if k > 0 {
				privStr += ", " + priv
				continue
			}
			privStr += priv
		}
	}

	return privStr, nil
}

func tableNameCorrect(tableNAme string) bool {
	match, _ := regexp.MatchString(`^[\p{L}_][\p{L}\p{N}@$#_]{0,127}$`, tableNAme)

	return match
}
