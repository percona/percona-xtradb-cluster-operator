package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/collector"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/recoverer"
	"github.com/pkg/errors"
)

var action string

func main() {
	flag.StringVar(&action, "action", "c", "'c' - collacting bimnlogs, 'r' - recover")
	switch action {
	case "c":
		runCollector()
	case "r":
		runRecoverer()
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

	for {
		err := c.Run()
		if err != nil {
			log.Println("ERROR:", err)
		}

		time.Sleep(time.Duration(sleep) * time.Second)
	}
}

func runRecoverer() {

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
	recTime, err := strconv.ParseInt(getEnv("RECOVER_TIME", ""), 10, 64)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get buffer size")
	}

	return recoverer.Config{
		PXCUser:        getEnv("PXC_USER", "root"),
		PXCPass:        getEnv("PXC_PASS", "root"),
		PXCServiceName: getEnv("PXC_SERVICE", "some-name"),
		S3Endpoint:     getEnv("ENDPOINT", ""),
		S3AccessKeyID:  getEnv("ACCESS_KEY_ID", ""),
		S3AccessKey:    getEnv("SECRET_ACCESS_KEY", ""),
		S3BucketName:   getEnv("S3_BUCKET", "binlog-test"),
		S3Region:       getEnv("DEFAULT_REGION", ""),
		RecoverTime:    recTime,
		RecoverType:    getEnv("RECOVERY_TYPE", ""),
		BackupName:     getEnv("BACKUP_NAME", ""),
	}, nil
}

func getEnv(key, defaultVal string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = defaultVal
	}

	return value
}
