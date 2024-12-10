package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
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
		case "Server shutdown in progress",
			"Forceful shutdown of connections in progress",
			"Graceful shutdown of connections in progress",
			"Components initialization unsuccessful",
			"Execution of SQL Commands from Init-file unsuccessful",
			"Initialization of dynamic plugins unsuccessful",
			"Initialization of MySQL system tables unsuccessful",
			"InnoDB crash recovery unsuccessful",
			"InnoDB initialization unsuccessful":
			return MySQLDown
		case "Server startup in progress",
			"Server initialization in progress",
			"Server upgrade in progress",
			"Server upgrade complete",
			"Server downgrade in progress",
			"Server downgrade complete",
			"Data Dictionary upgrade in progress",
			"Data Dictionary upgrade complete",
			"Data Dictionary upgrade from MySQL 5.7 in progress",
			"Data Dictionary upgrade from MySQL 5.7 complete",
			"Components initialization in progress",
			"Components initialization successful",
			"Connection shutdown complete",
			"Execution of SQL Commands from Init-file successful",
			"Initialization of dynamic plugins in progress",
			"Initialization of dynamic plugins successful",
			"Initialization of MySQL system tables in progress",
			"Initialization of MySQL system tables successful",
			"InnoDB crash recovery in progress",
			"InnoDB crash recovery successful",
			"InnoDB initialization in progress",
			"InnoDB initialization successful",
			"Shutdown of plugins complete",
			"Shutdown of components in progress",
			"Shutdown of components successful",
			"Shutdown of plugins in progress",
			"Shutdown of replica threads in progress",
			"Server shutdown complete": // we treat this as startup because during init, MySQL notifies this even if it's up
			return MySQLStartup
		}

		// these statuses have variables in it
		// that's why we're handling them separately
		switch {
		case strings.HasPrefix(status, "Pre DD shutdown of MySQL SE plugin"):
			return MySQLStartup
		case strings.HasPrefix(status, "Server shutdown complete"):
			return MySQLStartup
		case strings.HasPrefix(status, "Server initialization complete"):
			return MySQLStartup
		}

	}

	return MySQLUnknown
}

func main() {
	log.Println("Starting mysql-state-monitor")

	socketPath, ok := os.LookupEnv("NOTIFY_SOCKET")
	if !ok {
		log.Fatalln("NOTIFY_SOCKET env variable is required")
	}

	stateFilePath, ok := os.LookupEnv("MYSQL_STATE_FILE")
	if !ok {
		log.Fatalln("MYSQL_STATE_FILE env variable is required")
	}

	stateFile, err := os.Create(stateFilePath)
	if err != nil {
		log.Fatalf("Failed to create state file: %s", err)
	}
	defer stateFile.Close()

	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			log.Fatalf("Failed to remove %s: %s", socketPath, err)
		}
	}

	addr, err := net.ResolveUnixAddr("unixgram", socketPath)
	if err != nil {
		log.Fatalf("Failed to resolve unix addr %s: %s", socketPath, err)
	}

	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		log.Fatalf("Failed to listen unixgram %s: %s", socketPath, err)
	}
	defer conn.Close()

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, os.Interrupt, os.Kill)

	go func() {
		sig := <-sigterm
		log.Printf("Received signal %v. Exiting mysql-state-monitor", sig)
		os.Exit(0)
	}()

	buf := make([]byte, 256)
	for {
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

		_, err = stateFile.Write([]byte(mysqlState))
		if err != nil {
			log.Printf("Failed to write to state file: %s", err)
		}
	}
}
