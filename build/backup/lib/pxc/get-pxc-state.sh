#!/bin/bash

function mysql_exec() {
	local server="$1"
	local query="$2"

	mysql_pass=$(cat /etc/mysql/mysql-users-secret/xtrabackup 2>/dev/null || :)
	MYSQL_PASSWORD="${mysql_pass:-$PXC_PASS}"

	MYSQL_PWD=${MYSQL_PASSWORD} timeout 600 mysql -P33062 -h"${server}" -uxtrabackup -s -NB -e "${query}"

}

function wait_for_mysql() {
	local h="$1"
	for _ in {1..10}; do
		if [ "$(mysql_exec "$h" 'select 1')" == "1" ]; then
			return
		fi
		echo "MySQL is not up yet... sleeping ..."
		sleep 1
	done
}

echo
while read -ra LINE; do
	wait_for_mysql "${LINE[0]}"
	STATUS=$(mysql_exec "${LINE[0]}" "SHOW GLOBAL STATUS LIKE 'wsrep_%';")
	READY=$(echo "$STATUS" | grep wsrep_ready | awk '{print$2}')
	ONLINE=$(echo "$STATUS" | grep wsrep_connected | awk '{print$2}')
	STATE=$(echo "$STATUS" | grep wsrep_local_state_comment | awk '{print$2}')
	CLUSTER_STATUS=$(echo "$STATUS" | grep wsrep_cluster_status | awk '{print$2}')
	CLUSTER_SIZE=$(echo "$STATUS" | grep wsrep_cluster_size | awk '{print$2}')

	echo node:"${LINE[0]}":wsrep_ready:"$READY":wsrep_connected:"$ONLINE":wsrep_local_state_comment:"$STATE":wsrep_cluster_status:"$CLUSTER_STATUS":wsrep_cluster_size:"$CLUSTER_SIZE"
done
