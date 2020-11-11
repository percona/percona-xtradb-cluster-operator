package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/collector"
	"github.com/pkg/errors"
)

func main() {
	config, err := getConfig()
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

func getConfig() (collector.Config, error) {
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

func getEnv(key, defaultVal string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = defaultVal
	}

	return value
}
