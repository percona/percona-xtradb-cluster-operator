#!/bin/bash

set -o errexit
set -o xtrace

function log() {
	set +o xtrace
	echo "[$(date +%Y-%m-%dT%H:%M:%S%z)]" $*
	set -o xtrace
}

function mysql_root_exec() {
	local server="$1"
	local query="$2"
	set +o xtrace
	MYSQL_PWD="${OPERATOR_PASSWORD:-operator}" timeout 600 mysql -h "${server}" -uoperator -s -NB -e "${query}"
	set -o xtrace
}

function wait_for_mysql() {
	local h="$1"
	log "Waiting for host $h to be online..."

	local retry=0
	until [[ "$(mysql_root_exec "$h" 'select 1')" == "1" ]]; do
		log "[retry ${retry}] MySQL is not up yet... sleeping..."
		sleep 1

		let retry+=1
		if [[ $retry -ge 30 ]]; then
			log "${h} is not up after ${retry} attempts!"
			return 1
		fi
	done

	log "MySQL host ${h} is up and running."
}

function proxysql_admin_exec() {
	local server="$1"
	local query="$2"
	set +o xtrace
	MYSQL_PWD="${PROXY_ADMIN_PASSWORD:-admin}" timeout 600 mysql -h "${server}" -P6032 -u "${PROXY_ADMIN_USER:-admin}" -s -NB -e "${query}"
	set -o xtrace
}

function wait_for_proxy() {
	local h=127.0.0.1
	log "Waiting for host $h to be online..."

	local retry=0
	until [[ "$(proxysql_admin_exec "$h" 'select 1')" == "1" ]]; do
		log "[retry ${retry}] ProxySQL is not up yet... sleeping..."
		sleep 1

		let retry+=1
		if [[ $retry -ge 30 ]]; then
			log "ProxySQL is not up after ${retry} attempts!"
			return 1
		fi
	done

	log "ProxySQL is up and running."
}

PERCONA_SCHEDULER_CFG=/tmp/scheduler-config.toml

function main() {
	log "Running $0"

	local service
	local pod_zero
	local update_weights
	local hosts

	sleep 15s # wait for evs.inactive_timeout

	while read host; do
		if [[ -z ${host} ]]; then
			log "No host provided via stdin."
			exit 0
		fi

		service=$(echo $host | cut -d . -f 2-)
		pod_name=$(echo $host | cut -d . -f -1)
		pod_zero=$(echo $pod_name | sed "s/-[0-9]*$/-0/")
		pod_id=$(echo $pod_name | awk -F'-' '{print $NF}')

		wait_for_mysql "${host}"

		hosts=$((hosts + 1))

		write_weight=1000
		read_weight=1000
		case ${pod_id} in
			0)
				write_weight=1000000
				read_weight=600
				;;
			*)
				write_weight=$((write_weight - pod_id))
				read_weight=$((read_weight - pod_id))
				;;
		esac

		update_weights="${update_weights} UPDATE mysql_servers SET weight=${read_weight} WHERE hostgroup_id IN (10, 8010) AND hostname LIKE \"${pod_name}%\"; UPDATE mysql_servers SET weight=${write_weight} WHERE hostgroup_id IN (11, 8011) AND hostname LIKE \"${pod_name}%\";"
	done

	wait_for_proxy

	SSL_ARG=""
	if [ "$(proxysql_admin_exec "127.0.0.1" 'SELECT variable_value FROM global_variables WHERE variable_name="mysql-have_ssl"')" = "true" ]; then
		SSL_ARG="--use-ssl=yes"
		if [ "${SCHEDULER_ENABLED}" == "true" ]; then
			sed -i "s/^useSSL.*=.*$/useSSL=1/" ${PERCONA_SCHEDULER_CFG}
		else
			SSL_ARG="--use-ssl=yes"
		fi
	fi

	if [ "${SCHEDULER_ENABLED}" == "true" ]; then
		if proxysql-admin --config-file=/etc/proxysql-admin.cnf --is-enabled >/dev/null 2>&1; then
			log "Cleaning setup from proxysql-admin..."
			proxysql-admin --config-file=/etc/proxysql-admin.cnf --disable

			log "Cleaning proxysql_servers..."
			proxysql_admin_exec "127.0.0.1" "DELETE FROM proxysql_servers; LOAD PROXYSQL SERVERS TO RUNTIME;"
		fi

		if [ "$(proxysql_admin_exec "127.0.0.1" 'SELECT count(*) FROM runtime_scheduler')" -eq 0 ]; then
			percona-scheduler-admin --config-file=${PERCONA_SCHEDULER_CFG} --enable --force
		fi

		# don't remove and re-add servers if not necessary
		if [[ "$(proxysql_admin_exec 127.0.0.1 'SELECT COUNT(DISTINCT(hostname)) FROM mysql_servers;')" != ${hosts} ]]; then
			percona-scheduler-admin \
				--config-file=${PERCONA_SCHEDULER_CFG} \
				--write-node="${pod_zero}.${service}:3306" \
				--update-cluster \
				--remove-all-servers \
				--force
			proxysql_admin_exec "127.0.0.1" "${update_weights}; LOAD MYSQL SERVERS TO RUNTIME;"
		fi

		# update weights if ProxySQL is restarted
		if [[ "$(proxysql_admin_exec 127.0.0.1 'SELECT COUNT(DISTINCT(hostname)) FROM mysql_servers WHERE weight=1000;')" > 0 ]]; then
			proxysql_admin_exec "127.0.0.1" "${update_weights}; LOAD MYSQL SERVERS TO RUNTIME;"
		fi

		percona-scheduler-admin \
			--config-file=${PERCONA_SCHEDULER_CFG} \
			--sync-multi-cluster-users \
			--add-query-rule

		percona-scheduler-admin \
			--config-file=${PERCONA_SCHEDULER_CFG} \
			--update-mysql-version
	else
		if percona-scheduler-admin --config-file=${PERCONA_SCHEDULER_CFG} --is-enabled >/dev/null 2>&1; then
			log "Cleaning setup from percona-scheduler-admin..."
			percona-scheduler-admin --config-file=${PERCONA_SCHEDULER_CFG} --disable
		fi

		proxysql-admin \
			--config-file=/etc/proxysql-admin.cnf \
			--cluster-hostname="${pod_zero}.${service}" \
			--enable \
			--update-cluster \
			--force \
			--remove-all-servers \
			--disable-updates \
			$SSL_ARG

		proxysql-admin \
			--config-file=/etc/proxysql-admin.cnf \
			--cluster-hostname="${pod_zero}.${service}" \
			--sync-multi-cluster-users \
			--add-query-rule \
			--disable-updates \
			--force

		proxysql-admin \
			--config-file=/etc/proxysql-admin.cnf \
			--cluster-hostname="${pod_zero}.${service}" \
			--update-mysql-version
	fi

	log "All done!"
}

main
exit 0
