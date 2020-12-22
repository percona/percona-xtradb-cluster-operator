package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/collector"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/recoverer"

	"github.com/caarlos0/env"
	"github.com/pkg/errors"
)

var action string

func main() {
	command := "collect"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "collect":
		runCollector()
	case "recover":
		runRecoverer()
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown command \"%s\".\nCommands:\n  collect - collect binlogs\n  recover - recover from binlogs\n", command)
		os.Exit(1)
	}
}

func runCollector() {
	config, err := getCollectorConfig()
	if err != nil {
		log.Fatalln("ERROR: get config:", err)
	}
	c, err := collector.New(config)
	if err != nil {
		log.Fatalln("ERROR: new controller:", err)
	}
	sleepStr, err := getEnv("COLLECT_SPAN_SEC", "60", false)
	if err != nil {
		log.Fatalln("ERROR: get COLLECT_SPAN_SEC env")
	}
	sleep, err := strconv.ParseInt(sleepStr, 10, 64)
	if err != nil {
		log.Fatalln("ERROR: parse COLLECT_SPAN_SEC env:", err)
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
		log.Fatalln("ERROR: get recoverer config:", err)
	}
	c, err := recoverer.New(config)
	if err != nil {
		log.Fatalln("ERROR: new recoverer controller:", err)
	}
	log.Println("run recover")
	err = c.Run()
	if err != nil {
		log.Fatalln("ERROR: recover:", err)
	}
}

func getCollectorConfig() (collector.Config, error) {
	cfg := collector.Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil

}

func getRecovererConfig() (recoverer.Config, error) {
	cfg := recoverer.Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	cfgBackupS3 := recoverer.BackupS3{}
	if err := env.Parse(&cfgBackupS3); err != nil {
		return cfg, err
	}
	cfg.BackupStorage = cfgBackupS3
	cfgBinlogS3 := recoverer.BinlogS3{}
	if err := env.Parse(&cfgBinlogS3); err != nil {
		return cfg, err
	}
	cfg.BinlogStorage = cfgBinlogS3

	return cfg, nil
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
