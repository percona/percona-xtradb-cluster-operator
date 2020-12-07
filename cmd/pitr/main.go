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
	flag.StringVar(&action, "action", "c", "'c' - binlogs collection, 'r' - recovery")
	flag.Parse()

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
	sleepStr, err := getEnv("COLLECT_SPAN_SEC", "60", false)
	if err != nil {
		log.Println("ERROR: get sllep env")
	}
	sleep, err := strconv.ParseInt(sleepStr, 10, 64)
	if err != nil {
		log.Println("ERROR: parse sleep env:", err)
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
	pxcUser, err := getEnv("PXC_USER", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get PXC_USER env")
	}
	pxcPass, err := getEnv("PXC_PASS", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get PXC_PASS env")
	}
	pxcServiceName, err := getEnv("PXC_SERVICE", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get PXC_SERVICEPXC_SERVICE env")
	}
	s3Endpoint, err := getEnv("ENDPOINT", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get ENDPOINT env")
	}
	s3AccessKeyID, err := getEnv("ACCESS_KEY_ID", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get ACCESS_KEY_ID env")
	}
	s3AccessKey, err := getEnv("SECRET_ACCESS_KEY", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get SECRET_ACCESS_KEY env")
	}
	s3BucketURL, err := getEnv("S3_BUCKET_URL", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get S3_BUCKET_URL env")
	}
	s3Region, err := getEnv("DEFAULT_REGION", "", true)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get DEFAULT_REGION env")
	}
	bufferSizeStr, err := getEnv("BUFFER_SIZE", "0", false)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get BUFFER_SIZE env")
	}
	bufferSize, err := strconv.ParseInt(bufferSizeStr, 10, 64)
	if err != nil {
		return collector.Config{}, errors.Wrap(err, "get buffer size")
	}

	return collector.Config{
		PXCUser:        pxcUser,
		PXCPass:        pxcPass,
		PXCServiceName: pxcServiceName,
		S3Endpoint:     s3Endpoint,
		S3AccessKeyID:  s3AccessKeyID,
		S3AccessKey:    s3AccessKey,
		S3BucketURL:    s3BucketURL,
		S3Region:       s3Region,
		BufferSize:     bufferSize,
	}, nil
}

func getRecovererConfig() (recoverer.Config, error) {
	pxcUser, err := getEnv("PXC_USER", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get PXC_USER env")
	}
	pxcPass, err := getEnv("PXC_PASS", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get PXC_PASS env")
	}
	pxcServiceName, err := getEnv("PXC_SERVICE", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get PXC_SERVICE env")
	}
	backupS3Endpoint, err := getEnv("ENDPOINT", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get ENDPOINT env")
	}
	backupS3AccessKeyID, err := getEnv("ACCESS_KEY_ID", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get ACCESS_KEY_ID env")
	}
	backupS3AccessKey, err := getEnv("SECRET_ACCESS_KEY", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get SECRET_ACCESS_KEY env")
	}
	backupS3BackupDest, err := getEnv("S3_BUCKET_URL", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get S3_BUCKET_URL env")
	}
	backupS3Region, err := getEnv("DEFAULT_REGION", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get DEFAULT_REGION env")
	}

	binlogS3Endpoint, err := getEnv("BINLOG_S3_ENDPOINT", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get BINLOG_S3_ENDPOINT env")
	}
	binlogS3AccessKeyID, err := getEnv("BINLOG_ACCESS_KEY_ID", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get BINLOG_ACCESS_KEY_ID env")
	}
	binlogS3AccessKey, err := getEnv("BINLOG_SECRET_ACCESS_KEY", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get BINLOG_SECRET_ACCESS_KEY env")
	}
	binlogS3Region, err := getEnv("BINLOG_S3_REGION", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get BINLOG_S3_REGION env")
	}
	binlogS3BucketURL, err := getEnv("BINLOG_S3_BUCKET_URL", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get BINLOG_S3_BUCKET_URL env")
	}
	recoverTime, err := getEnv("PITR_DATE", "", false)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get PITR_DATE env")
	}
	recoverType, err := getEnv("PITR_RECOVERY_TYPE", "", true)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get PITR_RECOVERY_TYPE env")
	}
	gtidSet, err := getEnv("PITR_GTID_SET", "", false)
	if err != nil {
		return recoverer.Config{}, errors.Wrap(err, "get PITR_GTID_SET env")
	}

	return recoverer.Config{
		PXCUser:        pxcUser,
		PXCPass:        pxcPass,
		PXCServiceName: pxcServiceName,
		BackupStorage: recoverer.S3{
			Endpoint:    backupS3Endpoint,
			AccessKeyID: backupS3AccessKeyID,
			AccessKey:   backupS3AccessKey,
			BackupDest:  backupS3BackupDest,
			Region:      backupS3Region,
		},
		BinlogStorage: recoverer.S3{
			Endpoint:    binlogS3Endpoint,
			AccessKeyID: binlogS3AccessKeyID,
			AccessKey:   binlogS3AccessKey,
			Region:      binlogS3Region,
			BucketURL:   binlogS3BucketURL,
		},
		RecoverTime: recoverTime,
		RecoverType: recoverType,
		GTIDSet:     gtidSet,
	}, nil
}

func getEnv(key, defaultVal string, required bool) (string, error) {
	value, exists := os.LookupEnv(key)
	if !exists && !required {
		value = defaultVal
	}
	if len(value) == 0 && required {
		return "", errors.Errorf("env %s is empty or not exist", key)
	}
	return value, nil
}
