#!/bin/bash

set -o errexit

if [[ ${SCHEDULER_ENABLED} != "true" ]]; then
	exec "$@"
else
	# Start zombie reaper to clean up processes spawned by commands
	# This is needed because percona-scheduler-admin may not properly reap all child processes
	# The reaper runs as a background process and continuously reaps zombies
	(
	       while true; do
		       sleep 0.5
		       # Reap any zombie processes that are children of PID 1
		       while wait -n 2>/dev/null; do :; done
	       done
	) &
	REAPER_PID=$!

	# Cleanup function
	cleanup() {
	       kill $REAPER_PID 2>/dev/null || true
	       wait $REAPER_PID 2>/dev/null || true
	}
	trap cleanup EXIT TERM INT

	# Run peer-list in foreground (not exec) so reaper can continue running
	# This allows the reaper to clean up zombies spawned by child processes
	"$@" &
	PEER_LIST_PID=$!

	# Forward signals to proxysql
	forward_signal() {
	       kill -"$1" "$PEER_LIST_PID" 2>/dev/null || true
	}
	trap 'forward_signal TERM' TERM
	trap 'forward_signal INT' INT

	# Wait for peer-list and forward its exit code
	wait $PEER_LIST_PID
	EXIT_CODE=$?

	# Clean up reaper
	cleanup

	exit $EXIT_CODE
fi
