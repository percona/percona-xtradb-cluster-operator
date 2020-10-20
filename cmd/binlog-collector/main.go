package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/collector"
)

func main() {
	c, err := collector.New(getConfig())
	if err != nil {
		log.Println("ERROR: new controller", err)
		os.Exit(1)
	}

	sleep, err := strconv.ParseInt(getEnv("SLEEP_SECONDS", "60"), 10, 64)
	if err != nil {
		log.Println("ERROR: get sleep env", err)
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

func getConfig() collector.Config {
	return collector.Config{
		PXCUser:        getEnv("PXC_USER", "root"),
		PXCPass:        getEnv("PXC_PASS", "root"),
		PXCServiceName: getEnv("PXC_SERVICE", "some-name"),
		S3Endpoint:     getEnv("S3_ENDPOINT", "storage.googleapis.com"),
		S3accessKeyID:  getEnv("S3_ACCESS_KEY_ID", ""),
		S3accessKey:    getEnv("S3_ACCESS_KEY", ""),
		S3bucketName:   getEnv("S3_BUCKET_NAME", "binlog-test"),
	}
}

func getEnv(key, defaultVal string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = defaultVal
	}

	return value
}
