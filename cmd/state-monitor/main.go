package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"
)

type MySQLState string

const (
	MySQLReady   MySQLState = "ready"
	MySQLDown    MySQLState = "down"
	MySQLStartup MySQLState = "startup"
	MySQLUnknown MySQLState = "unknown"
)

func parseDatum(datum string) MySQLState {
	lines := strings.Split(datum, "\n")

	if lines[0] == "READY=1" {
		return MySQLReady
	}

	if lines[0] == "STOPPING=1" {
		return MySQLDown
	}

	if strings.HasPrefix(lines[0], "STATUS=") {
		status := strings.TrimPrefix(lines[0], "STATUS=")
		switch status {
		case "Server is operational":
			return MySQLReady
		case "Server shutdown in progress", "Server shutdown complete":
			return MySQLDown
		case "Server startup in progress",
			"Data Dictionary upgrade in progress",
			"Data Dictionary upgrade complete",
			"Server upgrade in progress",
			"Server upgrade complete",
			"Server downgrade in progress",
			"Server downgrade complete",
			"Data Dictionary upgrade from MySQL 5.7 in progress",
			"Data Dictionary upgrade from MySQL 5.7 complete":
			return MySQLStartup
		}
	}

	return MySQLUnknown
}

func main() {
	log.Println("Starting state-monitor")

	socketPath, envDefined := os.LookupEnv("NOTIFY_SOCKET")
	if !envDefined {
		log.Fatalln("NOTIFY_SOCKET env variable is required")
	}

	stateFilePath, envDefined := os.LookupEnv("MYSQL_STATE_FILE")
	if !envDefined {
		log.Fatalln("MYSQL_STATE_FILE env variable is required")
	}

	stateFile, err := os.Create(stateFilePath)
	if err != nil {
		log.Fatalf("Failed create state file: %s", err)
	}

	addr, err := net.ResolveUnixAddr("unixgram", socketPath)
	if err != nil {
		log.Fatalf("Failed resolve unix addr %s: %s", socketPath, err)
	}

	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		log.Fatalf("Failed listen unixgram %s: %s", socketPath, err)
	}
	defer conn.Close()

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, os.Interrupt)

	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			buf := make([]byte, 256)

			n, _, err := conn.ReadFromUnix(buf)
			if err != nil {
				log.Printf("Failed to read from unix socket: %s", err)
				continue
			}
			datum := string(buf[:n])
			mysqlState := parseDatum(datum)

			log.Printf("MySQLState: %s\nReceived: %s", mysqlState, datum)

			err = stateFile.Truncate(0)
			if err != nil {
				log.Printf("Failed to truncate state file: %s", err)
				continue
			}

			n, err = stateFile.Write([]byte(mysqlState))
			if err != nil {
				log.Printf("Failed to write to state file: %s", err)
			}
		case <-sigterm:
			log.Println("Received sigterm")
			os.Exit(0)
		}
	}
}
