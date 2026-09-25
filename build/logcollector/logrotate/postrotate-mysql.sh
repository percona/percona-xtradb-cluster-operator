#!/bin/bash

set -o errexit

PXC_SERVER_PORT='33062'
MONITOR_USER='monitor'
TIMEOUT=10
NODE_IP=$(hostname -I | awk ' { print $1 } ')

MYSQL_CMDLINE="/usr/bin/timeout $TIMEOUT /usr/bin/mysql -nN -u$MONITOR_USER -h$NODE_IP -P$PXC_SERVER_PORT"

export MYSQL_PWD=${MONITOR_PASSWORD}

audit_component_loaded=$($MYSQL_CMDLINE -e "SELECT COUNT(*) FROM mysql.component WHERE component_urn = 'file://component_audit_log_filter';" 2>/dev/null || echo 0)
audit_plugin_loaded=$($MYSQL_CMDLINE -e "SHOW PLUGINS" | grep -c 'audit_log' || true)
if [ "$audit_component_loaded" -gt 0 ]; then
	$MYSQL_CMDLINE -e 'FLUSH NO_WRITE_TO_BINLOG ERROR LOGS;DO audit_log_rotate();'
elif [ "$audit_plugin_loaded" -gt 0 ]; then
	$MYSQL_CMDLINE -e 'FLUSH NO_WRITE_TO_BINLOG ERROR LOGS;SET GLOBAL audit_log_flush=1;'
else
	$MYSQL_CMDLINE -e 'FLUSH NO_WRITE_TO_BINLOG ERROR LOGS;'
fi
