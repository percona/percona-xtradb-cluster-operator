#!/bin/bash

PROXYSQL_ADMIN_PORT='6032'

ADMIN_USER="${PROXY_ADMIN_USER:-admin}"
TIMEOUT=${READINESS_CHECK_TIMEOUT:-10}
MYSQL_CMDLINE="/usr/bin/timeout $TIMEOUT /usr/bin/mysql -nNE -u$ADMIN_USER"

proxyadmin_pass=$(cat /etc/mysql/mysql-users-secret/proxyadmin 2>/dev/null || :)
export MYSQL_PWD="${proxyadmin_pass:-$PROXY_ADMIN_PASSWORD}"

#   Hostgroups (see build/proxysql-admin.cnf):
#     10 = READER_HOSTGROUP_ID,
#     11 = WRITER_HOSTGROUP_ID
# checking if there are any readers/writers alive
STATUS=$($MYSQL_CMDLINE -h127.0.0.1 -P$PROXYSQL_ADMIN_PORT -e "SELECT COUNT(*) FROM runtime_mysql_servers WHERE hostgroup_id IN (10, 11) AND status='ONLINE';" | sed -n -e '2p' | tr '\n' ' ')

if [[ ${STATUS} -ge 1 ]]; then
	exit 0
fi

exit 1
