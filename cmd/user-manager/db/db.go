package db

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

func Conn(connection string) (db *sql.DB, err error) {
	db, err = sql.Open("mysql", connection)
	if err != nil {
		return db, err
	}

	return db, err
}

func DropUser(userName, userHost string) string {
	return fmt.Sprintf("DROP USER '%s'@'%s'", userName, userHost)
}

func CreateUser(userName, userPass, userHost string) string {
	return fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s'", userName, userHost, userPass)
}

func GrantUser(privileges, table, userName, userHost string) string {
	return fmt.Sprintf("GRANT %s ON %s TO '%s'@'%s'", privileges, table, userName, userHost)
}

func FlushPrivileges() string {
	return "FLUSH PRIVILEGES"
}
