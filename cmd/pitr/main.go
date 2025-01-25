package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/collector"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/recoverer"

	"github.com/caarlos0/env"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	command := "collect"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	srv := &http.Server{Addr: ":8080"}
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/health", healthHandler)
		http.HandleFunc("/invalidate-cache/", cacheInvalidationHandler)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("ERROR: HTTP server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("ERROR: HTTP server shutdown: %v", err)
		}
	}()

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

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Println("ERROR: writing health response:", err)
	}
}

func cacheInvalidationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		if _, err := w.Write([]byte("only POST method is allowed")); err != nil {
			log.Println("ERROR: writing response:", err)
		}
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("failed to parse form")); err != nil {
			log.Println("ERROR: writing response:", err)
		}
		return
	}

	hostname := r.FormValue("hostname")
	if hostname == "" {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("hostname is required")); err != nil {
			log.Println("ERROR: writing response:", err)
		}
		return
	}

	ctx := r.Context()

	config, err := getCollectorConfig()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("ERROR: get collector config:", err)
		return
	}

	c, err := collector.New(ctx, config)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("ERROR: get new collector:", err)
		return
	}

	if err := collector.InvalidateCache(ctx, c, hostname); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("ERROR: invalidate cache:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(fmt.Sprintf("cache invalidated for host: %s", hostname))); err != nil {
		log.Println("ERROR: writing response:", err)
	}
}

func runCollector(ctx context.Context) {
	config, err := getCollectorConfig()
	if err != nil {
		log.Fatalln("ERROR: get config:", err)
	}
	c, err := collector.New(ctx, config)
	if err != nil {
		log.Fatalln("ERROR: new collector:", err)
	}

	log.Println("initializing collector")
	if err := c.Init(ctx); err != nil {
		log.Fatalln("ERROR: init collector:", err)
	}

	log.Println("running binlog collector")
	for {
		timeout, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds)*time.Second)
		defer cancel()

		err := c.Run(timeout)
		if err != nil {
			log.Fatalln("ERROR:", err)
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
