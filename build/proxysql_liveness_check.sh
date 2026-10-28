#!/bin/bash

PROXYSQL_ADMIN_PORT='6032'

ADMIN_USER="${PROXY_ADMIN_USER:-admin}"
TIMEOUT=${LIVENESS_CHECK_TIMEOUT:-10}
MYSQL_CMDLINE="/usr/bin/timeout $TIMEOUT /usr/bin/mysql -nNE -u$ADMIN_USER"

export MYSQL_PWD="${PROXY_ADMIN_PASSWORD}"

STATUS=$($MYSQL_CMDLINE -h127.0.0.1 -P$PROXYSQL_ADMIN_PORT -e 'select 1;' | sed -n -e '2p' | tr '\n' ' ')

if [[ "${STATUS}" -eq 1 ]]; then
	exit 0
fi

exit 1
