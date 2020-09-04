#!/bin/bash

set -o xtrace

if [[ $1 == '-h' || $1 == '--help' ]];then
    echo "Usage: $0 <user> <pass>"
    exit
fi


MYSQL_USERNAME="${MYSQL_USERNAME:-clustercheck}"
MYSQL_PASSWORD="${CLUSTERCHECK_PASSWORD:-clustercheckpassword!}"
DEFAULTS_EXTRA_FILE=${DEFAULTS_EXTRA_FILE:-/etc/my.cnf}
AVAILABLE_WHEN_DONOR=${AVAILABLE_WHEN_DONOR:-1}

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

WSREP_STATUS=($(MYSQL_PWD="${MYSQL_PASSWORD}" $MYSQL_CMDLINE -e "SHOW GLOBAL STATUS LIKE 'wsrep_%';"  \
    | grep -A 1 -E 'wsrep_local_state$|wsrep_cluster_status$' \
    | sed -n -e '2p'  -e '5p' | tr '\n' ' '))

if [[ ${WSREP_STATUS[1]} == 'Primary' && ( ${WSREP_STATUS[0]} -eq 4 || \
    ( ${WSREP_STATUS[0]} -eq 2 && $AVAILABLE_WHEN_DONOR -eq 1 ) ) ]]; then
    exit 0
else
    exit 1
fi
