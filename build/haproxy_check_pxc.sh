#!/bin/bash

PXC_SERVER_IP=$3

PXC_SERVER_PORT='33062'
path_to_haproxy_cfg='/etc/haproxy/pxc'

MONITOR_USER='monitor'
MONITOR_PASSWORD=$(/bin/cat /etc/mysql/mysql-users-secret/monitor)

PATH_TO_SECRET='/etc/mysql/haproxy-env-secret'
if [ -f "$PATH_TO_SECRET/HA_CONNECTION_TIMEOUT" ]; then
	CUSTOM_TIMEOUT=$(/bin/cat $PATH_TO_SECRET/HA_CONNECTION_TIMEOUT)
fi

if [ -f "$PATH_TO_SECRET/OK_IF_DONOR" ]; then
	OK_IF_DONOR=$(/bin/cat $PATH_TO_SECRET/OK_IF_DONOR)
fi

if [ -f "$PATH_TO_SECRET/VERBOSE" ]; then
	VERBOSE=$(/bin/cat $PATH_TO_SECRET/VERBOSE)
fi

VERBOSE=${VERBOSE:-1}
TIMEOUT=${CUSTOM_TIMEOUT:-10}
DONOR_IS_OK=${OK_IF_DONOR:-0}
MYSQL_CMDLINE="/usr/bin/timeout $TIMEOUT /usr/bin/mysql -nNE -u$MONITOR_USER"

AVAILABLE_NODES=1
if [ -f "$path_to_haproxy_cfg/AVAILABLE_NODES" ]; then
	AVAILABLE_NODES=$(/bin/cat $path_to_haproxy_cfg/AVAILABLE_NODES)
fi

log() {
	local address=$1
	local port=$2
	local message=$3
	local should_log=$4

	if [ "$should_log" -eq 1 ]; then
		local date=$(/usr/bin/date +"%d/%b/%Y:%H:%M:%S.%3N")
		echo "{\"time\":\"${date}\", \"backend_source_ip\": \"${address}\", \"backend_source_port\": \"${port}\", \"message\": \"${message}\"}"
	fi
}

PXC_NODE_STATUS=($(MYSQL_PWD="${MONITOR_PASSWORD}" $MYSQL_CMDLINE -h $PXC_SERVER_IP -P $PXC_SERVER_PORT \
	-e "SHOW STATUS LIKE 'wsrep_local_state'; \
        SHOW VARIABLES LIKE 'pxc_maint_mode'; \
        SHOW GLOBAL STATUS LIKE 'wsrep_cluster_status'; \
        SHOW GLOBAL VARIABLES LIKE 'wsrep_reject_queries'; \
        SHOW GLOBAL VARIABLES LIKE 'wsrep_sst_donor_rejects_queries';" \
	| /usr/bin/grep -A 1 -E 'wsrep_local_state$|pxc_maint_mode$|wsrep_cluster_status$|wsrep_reject_queries$|wsrep_sst_donor_rejects_queries$' \
	| /usr/bin/sed -n -e '2p' -e '5p' -e '8p' -e '11p' -e '14p' \
	| /usr/bin/tr '\n' ' '))

# ${PXC_NODE_STATUS[0]} - wsrep_local_state
# ${PXC_NODE_STATUS[1]} - pxc_maint_mod
# ${PXC_NODE_STATUS[2]} - wsrep_cluster_status
# ${PXC_NODE_STATUS[3]} - wsrep_reject_queries
# ${PXC_NODE_STATUS[4]} - wsrep_sst_donor_rejects_queries
status_log="The following values are used for PXC node $PXC_SERVER_IP in backend $HAPROXY_PROXY_NAME: "
status_log+="wsrep_local_state is ${PXC_NODE_STATUS[0]}; pxc_maint_mod is ${PXC_NODE_STATUS[1]}; wsrep_cluster_status is ${PXC_NODE_STATUS[2]}; wsrep_reject_queries is ${PXC_NODE_STATUS[3]}; wsrep_sst_donor_rejects_queries is ${PXC_NODE_STATUS[4]}; $AVAILABLE_NODES nodes are available"

if [[ ${PXC_NODE_STATUS[2]} == 'Primary' &&  ( ${PXC_NODE_STATUS[0]} -eq 4 || \
    ${PXC_NODE_STATUS[0]} -eq 2 && ( "${AVAILABLE_NODES}" -le 1 || "${DONOR_IS_OK}" -eq 1 ) ) \
    && ${PXC_NODE_STATUS[1]} == 'DISABLED' && ${PXC_NODE_STATUS[3]} == 'NONE' && ${PXC_NODE_STATUS[4]} == 'OFF' ]];
then
    log "$PXC_SERVER_IP" "$PXC_SERVER_PORT" "$status_log" "$VERBOSE"
    log "$PXC_SERVER_IP" "$PXC_SERVER_PORT" "PXC node $PXC_SERVER_IP for backend $HAPROXY_PROXY_NAME is ok" "$VERBOSE"
    exit 0
else
	log "$PXC_SERVER_IP" "$PXC_SERVER_PORT" "$status_log" 1
	log "$PXC_SERVER_IP" "$PXC_SERVER_PORT" "PXC node $PXC_SERVER_IP for backend $HAPROXY_PROXY_NAME is not ok" 1
	exit 1
fi
