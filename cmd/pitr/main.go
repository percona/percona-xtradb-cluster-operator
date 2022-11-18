package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/collector"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/recoverer"

	"github.com/caarlos0/env"
)

func main() {
	command := "collect"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	switch command {
	case "collect":
		runCollector(ctx)
	case "recover":
		runRecoverer(ctx)
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown command \"%s\".\nCommands:\n  collect - collect binlogs\n  recover - recover from binlogs\n", command)
		os.Exit(1)
	}
}

func runCollector(ctx context.Context) {
	config, err := getCollectorConfig()
	if err != nil {
		log.Fatalln("ERROR: get config:", err)
	}
	c, err := collector.New(config)
	if err != nil {
		log.Fatalln("ERROR: new controller:", err)
	}
	log.Println("run binlog collector")
	for {
		err := c.Run(ctx)
		if err != nil {
			log.Println("ERROR:", err)
		}

		t := time.NewTimer(time.Duration(config.CollectSpanSec) * time.Second)
		select {
		case <-ctx.Done():
			log.Fatalln("ERROR:", ctx.Err().Error())
		case <-t.C:
			break
		}
	}
}

func runRecoverer(ctx context.Context) {
	config, err := getRecovererConfig()
	if err != nil {
		log.Fatalln("ERROR: get recoverer config:", err)
	}
	c, err := recoverer.New(ctx, config)
	if err != nil {
		log.Fatalln("ERROR: new recoverer controller:", err)
	}
	log.Println("run recover")
	err = c.Run(ctx)
	if err != nil {
		log.Fatalln("ERROR: recover:", err)
	}
}

func getCollectorConfig() (collector.Config, error) {
	cfg := collector.Config{}
	err := env.Parse(&cfg)
	switch cfg.StorageType {
	case "s3":
		if err := env.Parse(&cfg.BackupStorageS3); err != nil {
			return cfg, err
		}
	case "azure":
		if err := env.Parse(&cfg.BackupStorageAzure); err != nil {
			return cfg, err
		}
	default:
		return cfg, errors.New("unknown STORAGE_TYPE")
	}

	return cfg, err

}

func getRecovererConfig() (recoverer.Config, error) {
	cfg := recoverer.Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	switch cfg.StorageType {
	case "s3":
		if err := env.Parse(&cfg.BackupStorageS3); err != nil {
			return cfg, err
		}
		if err := env.Parse(&cfg.BinlogStorageS3); err != nil {
			return cfg, err
		}
	case "azure":
		if err := env.Parse(&cfg.BackupStorageAzure); err != nil {
			return cfg, err
		}
		if err := env.Parse(&cfg.BinlogStorageAzure); err != nil {
			return cfg, err
		}
	default:
		return cfg, errors.New("unknown STORAGE_TYPE")
	}

	return cfg, nil
}
