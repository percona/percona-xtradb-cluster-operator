#!/bin/bash

set -o errexit
set -o xtrace

function proxysql_admin_exec() {
	local server="$1"
	local query="$2"
	set +o xtrace
	MYSQL_PWD="${PROXY_ADMIN_PASSWORD:-admin}" timeout 600 \
		mysql -h "${server}" -P "${PROXY_ADMIN_PORT:-6032}" -u "${PROXY_ADMIN_USER:-admin}" -s -NB -e "${query}"
	set -o xtrace
}

function wait_for_proxysql() {
	local server="$1"
	echo "Waiting for host $server to be online..."
	PROXYSQL_TABLE="runtime_mysql_galera_hostgroups"
	while [ "$(proxysql_admin_exec "$server" "SELECT MAX(active) FROM ${PROXYSQL_TABLE}")" != "1" ]; do
		echo "ProxySQL is not up yet... sleeping ..."
		sleep 1
	done
}

add_proxysql() {
	local dest=$1
	local host=$2
	proxysql_admin_exec "$dest" "
        INSERT INTO proxysql_servers (hostname,port) VALUES ('$host','${PROXY_ADMIN_PORT:-6032}');
    "
}

function main() {
	echo "Running $0"
	wait_for_proxysql "127.0.0.1"

	proxysql_admin_exec "127.0.0.1" "
        DELETE FROM proxysql_servers;
    "
	while read -ra LINE; do
		echo "Read line $LINE"
		add_proxysql "127.0.0.1" "$LINE"
	done
	add_proxysql "127.0.0.1" "$(hostname -f)" || :

	proxysql_admin_exec "127.0.0.1" "
        SELECT * FROM proxysql_servers;
        LOAD PROXYSQL SERVERS TO RUNTIME;
        SAVE PROXYSQL SERVERS TO DISK;
    "

	echo "All done!"
}

main
exit 0
