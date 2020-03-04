package main

import (
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/user-manager/manager"
	"github.com/pkg/errors"
)

func main() {
	rootPass := os.Getenv("PXC-ROOT-PASS")
	hosts := GetHostsFromEnvVar("PXC_CONNS")
	um, err := manager.New(hosts, rootPass)
	if err != nil {
		errors.Wrap(err, "create user manager")
		os.Exit(1)
	}
	err = um.GetUsers()
	if err != nil {
		errors.Wrap(err, "get users")
		os.Exit(1)
	}
	err = um.ManageUsers()
	if err != nil {
		errors.Wrap(err, "manage users")
		os.Exit(1)
	}
	fmt.Println("Done")
}

func GetHostsFromEnvVar(varName string) []string {
	hostsString := os.Getenv(varName)

	return strings.Split(hostsString, ",")
}
