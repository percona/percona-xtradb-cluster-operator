package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/collector"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/recoverer"
	"github.com/pkg/errors"
)

var action string

func main() {
	flag.StringVar(&action, "action", "c", "'c' - binlogs collection, 'r' - recovery")
	flag.Parse()

	switch action {
	case "c":
		runCollector()
	case "r":
		runRecoverer()
	default:
		fmt.Println("wrong or none flag")
		os.Exit(1)
	}

}

func runCollector() {
	config, err := getCollectorConfig()
	if err != nil {
		log.Println("ERROR: get config:", err)
		os.Exit(1)
	}

	c, err := collector.New(config)
	if err != nil {
		log.Println("ERROR: new controller:", err)
		os.Exit(1)
	}

	sleep, err := strconv.ParseInt(getEnv("COLLECT_SPAN_SEC", "60"), 10, 64)
	if err != nil {
		log.Println("ERROR: get sleep env:", err)
		os.Exit(1)
	}
	log.Println("run collector")
	for {
		err := c.Run()
		if err != nil {
			log.Println("ERROR:", err)
		}

		time.Sleep(time.Duration(sleep) * time.Second)
	}
}

func runRecoverer() {
	config, err := getRecovererConfig()
	if err != nil {
		log.Println("ERROR: get recoverer config:", err)
		os.Exit(1)
	}

	c, err := recoverer.New(config)
	if err != nil {
		log.Println("ERROR: new  recoverer controller:", err)
		os.Exit(1)
	}
	log.Println("run recover")
	err = c.Run()
	if err != nil {
		log.Println("ERROR: recover:", err)
		os.Exit(1)
	}
}

func getCollectorConfig() (collector.Config, error) {
	bufferSize, err := strconv.ParseInt(getEnv("BUFFER_SIZE", ""), 10, 64)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get buffer size")
	}

	return collector.Config{
		PXCUser:        getEnv("PXC_USER", "root"),
		PXCPass:        getEnv("PXC_PASS", "root"),
		PXCServiceName: getEnv("PXC_SERVICE", "some-name"),
		S3Endpoint:     getEnv("ENDPOINT", ""),
		S3AccessKeyID:  getEnv("ACCESS_KEY_ID", ""),
		S3AccessKey:    getEnv("SECRET_ACCESS_KEY", ""),
		S3BucketName:   getEnv("S3_BUCKET", "binlog-test"),
		S3Region:       getEnv("DEFAULT_REGION", ""),
		BufferSize:     bufferSize,
	}, nil
}

func getRecovererConfig() (recoverer.Config, error) {
	return recoverer.Config{
		PXCUser:        getEnv("PXC_USER", "root"),
		PXCServiceName: getEnv("PXC_SERVICE", "some-name"),
		S3Endpoint:     strings.TrimPrefix(getEnv("ENDPOINT", ""), "https://"),
		S3AccessKeyID:  getEnv("ACCESS_KEY_ID", ""),
		S3AccessKey:    getEnv("SECRET_ACCESS_KEY", ""),
		S3BucketName:   getEnv("S3_BUCKET", "binlog-test"),
		S3Region:       getEnv("DEFAULT_REGION", ""),
		RecoverTime:    getEnv("DATE", ""),
		RecoverType:    getEnv("RECOVERY_TYPE", ""),
		BackupName:     getEnv("BACKUP_NAME", ""),
		GTIDSet:        getEnv("GTID_SET", ""),
	}, nil
}

func getEnv(key, defaultVal string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = defaultVal
	}

	return value
}
