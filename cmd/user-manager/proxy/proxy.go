package proxy

import (
	"fmt"
	"os"
	"os/exec"
)

func SyncUsers() error {
	proxyUser := "root"
	proxyPass := os.Getenv("MYSQL_ROOT_PASSWORD")
	proxyHost := os.Getenv("PROXY_SERICE")
	proxyPort := "3306"
	clusterUser := "root"
	clusterPass := os.Getenv("MYSQL_ROOT_PASSWORD")
	clusterHost := os.Getenv("PXC_SERICE")

	pat := exec.Command("bash", "proxysql-admin",
		"--proxysql-username="+proxyUser,
		"--proxysql-password="+proxyPass,
		"--proxysql-hostname="+proxyHost,
		"--proxysql-port="+proxyPort,
		"--cluster-username="+clusterUser,
		"--cluster-password="+clusterPass,
		"--cluster-hostname="+clusterHost,
		"--syncusers")
	pat.Dir = "/percona/proxysql-admin-tool"
	o, err := pat.CombinedOutput()
	if err != nil {
		return fmt.Errorf(err.Error() + ": " + string(o))
	}

	return nil
}
