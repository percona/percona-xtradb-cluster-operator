#!/bin/bash

if [[ $1 == '-h' || $1 == '--help' ]]; then
	echo "Usage: $0 <user> <pass> <log_file>"
	exit
fi

if [ -f /tmp/recovery-case ] || [ -f '/var/lib/mysql/sleep-forever' ]; then
	exit 0
fi

if [[ -f '/var/lib/mysql/sst_in_progress' ]] || [[ -f '/var/lib/mysql/wsrep_recovery_verbose.log' ]]; then
	exit 0
fi

{ set +x; } 2>/dev/null
MYSQL_USERNAME="${MYSQL_USERNAME:-monitor}"
mysql_pass=$(cat /etc/mysql/mysql-users-secret/monitor || :)
MYSQL_PASSWORD="${mysql_pass:-$MONITOR_PASSWORD}"
DEFAULTS_EXTRA_FILE=${DEFAULTS_EXTRA_FILE:-/etc/my.cnf}
NODE_IP=$(hostname -I | awk ' { print $1 } ')
#Timeout exists for instances where mysqld may be hung
TIMEOUT=$((${LIVENESS_CHECK_TIMEOUT:-5} - 1))

EXTRA_ARGS=""
if [[ -n $MYSQL_USERNAME ]]; then
	EXTRA_ARGS="$EXTRA_ARGS -P 33062 -h${NODE_IP} --protocol=TCP --user=${MYSQL_USERNAME}"
fi
if [[ -r $DEFAULTS_EXTRA_FILE ]]; then
	MYSQL_CMDLINE="/usr/bin/timeout $TIMEOUT mysql --defaults-extra-file=$DEFAULTS_EXTRA_FILE -nNE \
        --connect-timeout=$TIMEOUT ${EXTRA_ARGS}"
else
	MYSQL_CMDLINE="/usr/bin/timeout $TIMEOUT mysql -nNE --connect-timeout=$TIMEOUT ${EXTRA_ARGS}"
fi

STATUS=$(MYSQL_PWD="${MYSQL_PASSWORD}" $MYSQL_CMDLINE --init-command="SET SESSION wsrep_sync_wait=0;" -e 'SHOW GLOBAL STATUS LIKE "wsrep_cluster_status";' | sed -n -e '3p')
set -x

if [[ -n ${STATUS} && ${STATUS} == 'Primary' ]]; then
	exit 0
fi

exit 1
