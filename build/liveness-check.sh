#!/bin/bash

if [[ $1 == '-h' || $1 == '--help' ]];then
    echo "Usage: $0 <user> <pass> <log_file>"
    exit
fi

if [[ -f '/var/lib/mysql/sst_in_progress' ]] || [[ -f '/var/lib/mysql/wsrep_recovery_verbose.log' ]];  then
    exit 0
fi

{ set +x; } 2>/dev/null
MYSQL_USERNAME="${MYSQL_USERNAME:-clustercheck}"
mysql_pass=$(cat /etc/mysql/mysql-users-secret/clustercheck || :)
MYSQL_PASSWORD="${mysql_pass:-$CLUSTERCHECK_PASSWORD}"
ERR_FILE="${ERR_FILE:-/var/log/mysql/clustercheck.log}"
DEFAULTS_EXTRA_FILE=${DEFAULTS_EXTRA_FILE:-/etc/my.cnf}

#Timeout exists for instances where mysqld may be hung
TIMEOUT=10

EXTRA_ARGS=""
if [[ -n "$MYSQL_USERNAME" ]]; then
    EXTRA_ARGS="$EXTRA_ARGS --user=${MYSQL_USERNAME}"
fi
if [[ -r $DEFAULTS_EXTRA_FILE ]];then
    MYSQL_CMDLINE="mysql --defaults-extra-file=$DEFAULTS_EXTRA_FILE -nNE --connect-timeout=$TIMEOUT \
                    ${EXTRA_ARGS}"
else
    MYSQL_CMDLINE="mysql -nNE --connect-timeout=$TIMEOUT ${EXTRA_ARGS}"
fi

STATUS=$(MYSQL_PWD="${MYSQL_PASSWORD}" $MYSQL_CMDLINE -e 'select 1;' | sed -n -e '2p' | tr '\n' ' ')
set -x

if [[ "${STATUS}" -eq 1 ]]; then
    exit 0
fi

exit 1
