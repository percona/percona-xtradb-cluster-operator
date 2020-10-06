package main

import (
	"log"
	"os"
	"strings"
	"time"
)

type config struct {
	pxcHosts      []string
	pxcUser       string
	pxcPass       string
	s3Endpoint    string
	s3accessKeyID string
	s3accessKey   string
	s3bucketName  string
}

func main() {
	for {
		err := manageBinlogs(getConfig())
		if err != nil {
			log.Println("ERROR:", err)
		}

		time.Sleep(60 * time.Second)
	}
}

func getConfig() config {
	hostsString := getEnv("PXC_HOSTS", "127.0.0.1")

	return config{
		pxcHosts:      getHosts(hostsString),
		pxcUser:       getEnv("PXC_USER", "root"),
		pxcPass:       getEnv("PXC_PASS", "root"),
		s3Endpoint:    getEnv("S3_ENDPOINT", "storage.googleapis.com"),
		s3accessKeyID: getEnv("S3_ACCESS_KEY_ID", ""),
		s3accessKey:   getEnv("S3_ACCESS_KEY", ""),
		s3bucketName:  getEnv("S3_BUCKET_NAME", "binlog-test"),
	}
}

func getEnv(key, defaultVal string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = defaultVal
	}

	return value
}

func getHosts(hosts string) []string {
	return strings.Split(hosts, ",")
}
