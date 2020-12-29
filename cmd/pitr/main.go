package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/collector"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/recoverer"

	"github.com/caarlos0/env"
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
	log.Println("run collector")
	for {
		err := c.Run()
		if err != nil {
			log.Println("ERROR:", err)
		}

		time.Sleep(time.Duration(config.CollectSpanSec) * time.Second)
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
	err := env.Parse(&cfg)

	return cfg, err

}

func getRecovererConfig() (recoverer.Config, error) {
	cfg := recoverer.Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	if err := env.Parse(&cfg.BackupStorage); err != nil {
		return cfg, err
	}
	if err := env.Parse(&cfg.BinlogStorage); err != nil {
		return cfg, err
	}

	return cfg, nil
}
