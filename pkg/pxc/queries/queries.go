package queries

import (
	"context"
    "bytes"
	"database/sql"
	"errors"
	"net/http"
    "net/url"
	"fmt"
    "strings"


	_ "github.com/go-sql-driver/mysql"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/transport/spdy"
    "k8s.io/client-go/tools/portforward"
	"sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/config"
    "sigs.k8s.io/controller-runtime/pkg/runtime/log"

)

// value of writer group is hardcoded in ProxySQL config inside docker image
// https://github.com/percona/percona-docker/blob/pxc-operator-1.3.0/proxysql/dockerdir/etc/proxysql-admin.cnf#L23
const writerID = 11

type Database struct {
	db *sql.DB
}

var ErrNotFound = errors.New("not found")

func New(client client.Client, namespace, secretName, user, pod string, port int32) (Database, error) {
	secretObj := corev1.Secret{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: namespace,
			Name:      secretName,
		},
		&secretObj,
	)
	if err != nil {
		return Database{}, err
	}

    // get kube config
    cfg, _:= config.GetConfig()

    // get kubernetes api url
    apiHost := cfg.Host

    log.Log.Info(fmt.Sprintf("hostname: %s", apiHost))

	pass := string(secretObj.Data[user])

	// portforward api path
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, pod)

	// get api ip
    hostIP := strings.TrimLeft(apiHost, "htps:/")

    // url to kubernetes api
    serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}

    // http dial for portforward
    roundTripper, upgrader, err := spdy.RoundTripperFor(cfg)
    if err != nil {
    	panic(err)
    }
    dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL)

    // portforward stuff see https://github.com/kubernetes/client-go/issues/51#issuecomment-436200428
    readyChan, stopChan := make(chan struct{}, 1), make(chan struct{}, 1)
    out, errOut := new(bytes.Buffer), new(bytes.Buffer)

    forwarder, err := portforward.New(dialer, []string{fmt.Sprintf("%d:%d", port, port)}, stopChan, readyChan, out, errOut)
    if err != nil {
    	panic(err)
    }

    go func() {
        if err = forwarder.ForwardPorts(); err != nil { // Locks until stopChan is closed.
            panic(err)
        }
    	for range readyChan { // Kubernetes will close this channel when it has something to tell us.
    	}
    	if len(errOut.String()) != 0 {
    		panic(errOut.String())
    	} else if len(out.String()) != 0 {
    		fmt.Println(out.String())
    	}
    }()

    select {
    case <-readyChan:
        break
    }

	connStr := fmt.Sprintf("%s:%s@tcp(127.0.0.1:%d)/mysql?interpolateParams=true", user, pass, port)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		return Database{}, err
	}

	go func() {
		for {
			err = db.Ping()
			if err != nil {
				log.Log.Info(fmt.Sprintf("sql connection to: %s closed, stopping portforward", pod))
				close(stopChan)
				return
			}
		}
	}()

	err = db.Ping()
	if err != nil {
		return Database{}, err
	}

	return Database{
		db: db,
	}, nil
}

func (p *Database) Status(host, ip string) ([]string, error) {
	rows, err := p.db.Query("select status from mysql_servers where hostname like ? or hostname = ?;", host+"%", ip)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	statuses := []string{}
	for rows.Next() {
		var status string

		err := rows.Scan(&status)
		if err != nil {
			return nil, err
		}

		statuses = append(statuses, status)
	}

    log.Log.Info(fmt.Sprintf("db status okay: %s", ip))

	return statuses, nil
}

func (p *Database) PrimaryHost() (string, error) {
	var host string
	err := p.db.QueryRow("SELECT hostname FROM runtime_mysql_servers WHERE hostgroup_id = ?", writerID).Scan(&host)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}

	return host, nil
}

func (p *Database) Hostname() (string, error) {
	var hostname string
	err := p.db.QueryRow("SELECT @@hostname hostname").Scan(&hostname)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}

	return hostname, nil
}

func (p *Database) WsrepLocalStateComment() (string, error) {
	var variable_name string
	var value string

	err := p.db.QueryRow("SHOW GLOBAL STATUS LIKE 'wsrep_local_state_comment'").Scan(&variable_name, &value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

	return value, nil
}

func (p *Database) Version() (string, error) {
	var version string

	err := p.db.QueryRow("select @@VERSION;").Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("variable was not found")
		}
		return "", err
	}

    log.Log.Info(fmt.Sprintf("db version: %s", version))

	return version, nil
}

func (p *Database) Close() error {
	return p.db.Close()
}