package main

import (
	"log"
	"os"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/controller"
)

func main() {
	c, err := controller.New(getConfig())
	if err != nil {
		log.Println("ERROR: new controller", err)
		os.Exit(1)
	}
	for {
		err := c.Run()
		if err != nil {
			log.Println("ERROR:", err)
		}
		time.Sleep(60 * time.Second)
	}
}

func getConfig() controller.Config {
	return controller.Config{
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
