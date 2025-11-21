#!/bin/bash

set -o errexit

log() {
	local message=$1
	local date=$(/usr/bin/date +"%d/%b/%Y:%H:%M:%S.%3N")

	echo "{\"time\":\"${date}\", \"message\": \"${message}\"}"
}

function main() {
	log "Running $0"

	NODE_LIST=()
	NODE_LIST_REPL=()
	NODE_LIST_MYSQLX=()
	NODE_LIST_ADMIN=()
	NODE_LIST_BACKUP=()
	firs_node=''
	firs_node_admin=''
	firs_node_replica=''
	main_node=''

	SERVER_OPTIONS=${HA_SERVER_OPTIONS:-'resolvers kubernetes check inter 10000 rise 1 fall 2 weight 1'}
	send_proxy=''
	shutdown_on_mark_down=''
	path_to_haproxy_cfg='/etc/haproxy/pxc'
	if [[ "${IS_PROXY_PROTOCOL}" = "yes" ]]; then
		send_proxy='send-proxy-v2'
	fi
	if [[ "${HA_SHUTDOWN_ON_MARK_DOWN}" = "yes" ]]; then
		shutdown_on_mark_down='on-marked-down shutdown-sessions'
	fi

	while read pxc_host; do
		if [ -z "$pxc_host" ]; then
			log 'Could not find PEERS ...'
			exit 0
		fi

		node_name=$(echo "$pxc_host" | cut -d . -f -1)
		node_id=$(echo $node_name | awk -F'-' '{print $NF}')
		NODE_LIST_REPL+=("server $node_name $pxc_host:3306 $send_proxy $SERVER_OPTIONS${shutdown_on_mark_down:+ }$shutdown_on_mark_down")
		if [ "x$node_id" == 'x0' ]; then
			firs_node_replica="$pxc_host"
			main_node="$pxc_host"
			firs_node="server $node_name $pxc_host:3306 $send_proxy $SERVER_OPTIONS on-marked-up shutdown-backup-sessions${shutdown_on_mark_down:+ }$shutdown_on_mark_down"
			firs_node_admin="server $node_name $pxc_host:33062 $SERVER_OPTIONS on-marked-up shutdown-backup-sessions${shutdown_on_mark_down:+ }$shutdown_on_mark_down"
			firs_node_mysqlx="server $node_name $pxc_host:33060 $SERVER_OPTIONS on-marked-up shutdown-backup-sessions${shutdown_on_mark_down:+ }$shutdown_on_mark_down"
			continue
		fi
		NODE_LIST_BACKUP+=("galera-nodes/$node_name" "galera-admin-nodes/$node_name")
		NODE_LIST+=("server $node_name $pxc_host:3306 $send_proxy $SERVER_OPTIONS backup${shutdown_on_mark_down:+ }$shutdown_on_mark_down")
		NODE_LIST_ADMIN+=("server $node_name $pxc_host:33062 $SERVER_OPTIONS backup${shutdown_on_mark_down:+ }$shutdown_on_mark_down")
		NODE_LIST_MYSQLX+=("server $node_name $pxc_host:33060 $send_proxy $SERVER_OPTIONS backup${shutdown_on_mark_down:+ }$shutdown_on_mark_down")
	done

	if [ -n "$firs_node" ]; then
		if [[ "${#NODE_LIST[@]}" -ne 0 ]]; then
			NODE_LIST=("$firs_node" "$(printf '%s\n' "${NODE_LIST[@]}" | sort --version-sort -r | uniq)")
			NODE_LIST_ADMIN=("$firs_node_admin" "$(printf '%s\n' "${NODE_LIST_ADMIN[@]}" | sort --version-sort -r | uniq)")
			NODE_LIST_MYSQLX=("$firs_node_mysqlx" "$(printf '%s\n' "${NODE_LIST_MYSQLX[@]}" | sort --version-sort -r | uniq)")
		else
			NODE_LIST=("$firs_node")
			NODE_LIST_ADMIN=("$firs_node_admin")
			NODE_LIST_MYSQLX=("$firs_node_mysqlx")
		fi
	else
		if [[ "${#NODE_LIST[@]}" -ne 0 ]]; then
			NODE_LIST=("$(printf '%s\n' "${NODE_LIST[@]}" | sort --version-sort -r | uniq)")
			NODE_LIST_ADMIN=("$(printf '%s\n' "${NODE_LIST_ADMIN[@]}" | sort --version-sort -r | uniq)")
			NODE_LIST_MYSQLX=("$(printf '%s\n' "${NODE_LIST_MYSQLX[@]}" | sort --version-sort -r | uniq)")
		fi
	fi

	cat <<-EOF >"$path_to_haproxy_cfg/haproxy.cfg"
		    backend galera-nodes
		      mode tcp
		      option srvtcpka
		      balance roundrobin
		      option external-check
		      external-check command /opt/percona/haproxy_check_pxc.sh
	EOF

	log "number of available nodes are ${#NODE_LIST_REPL[@]}"
	echo "${#NODE_LIST_REPL[@]}" >$path_to_haproxy_cfg/AVAILABLE_NODES
	(
		IFS=$'\n'
		echo "${NODE_LIST[*]}"
	) >>"$path_to_haproxy_cfg/haproxy.cfg"

	cat <<-EOF >>"$path_to_haproxy_cfg/haproxy.cfg"
		    backend galera-admin-nodes
		      mode tcp
		      option srvtcpka
		      balance roundrobin
		      option external-check
		      external-check command /opt/percona/haproxy_check_pxc.sh
	EOF

	(
		IFS=$'\n'
		echo "${NODE_LIST_ADMIN[*]}"
	) >>"$path_to_haproxy_cfg/haproxy.cfg"

	cat <<-EOF >>"$path_to_haproxy_cfg/haproxy.cfg"
		    backend galera-replica-nodes
		      mode tcp
		      option srvtcpka
		      balance roundrobin
		      option external-check
		      external-check command /opt/percona/haproxy_check_pxc.sh
	EOF
	if [ "${REPLICAS_SVC_ONLY_READERS}" == "false" ]; then
		(
			IFS=$'\n'
			echo "${NODE_LIST_REPL[*]}"
		) >>"$path_to_haproxy_cfg/haproxy.cfg"
	else
		if [ -n "$firs_node_replica" ]; then
			(
				IFS=$'\n'
				echo "${NODE_LIST_REPL[*]:1}"
			) >>"$path_to_haproxy_cfg/haproxy.cfg"
		else
			NODE_LIST_REPL=("$(printf "%s\n" "${NODE_LIST_REPL[@]}" | sort -r | tail -n +2)")
			(
				IFS=$'\n'
				echo "${NODE_LIST_REPL[*]}"
			) >>"$path_to_haproxy_cfg/haproxy.cfg"
		fi
	fi

	cat <<-EOF >>"$path_to_haproxy_cfg/haproxy.cfg"
		    backend galera-mysqlx-nodes
		      mode tcp
		      option srvtcpka
		      balance roundrobin
		      option external-check
		      external-check command /opt/percona/haproxy_check_pxc.sh
	EOF
	(
		IFS=$'\n'
		echo "${NODE_LIST_MYSQLX[*]}"
	) >>"$path_to_haproxy_cfg/haproxy.cfg"

	SOCKET='/etc/haproxy/pxc/haproxy.sock'
	path_to_custom_global_cnf='/etc/haproxy-custom'
	if [ -f "$path_to_custom_global_cnf/haproxy-global.cfg" ]; then
		haproxy -c -f "$path_to_custom_global_cnf/haproxy-global.cfg" -f $path_to_haproxy_cfg/haproxy.cfg || EC=$?
	fi

	if [ -f "$path_to_custom_global_cnf/haproxy-global.cfg" -a -z "$EC" ]; then
		SOCKET_CUSTOM=$(grep 'stats socket' "$path_to_custom_global_cnf/haproxy-global.cfg" | awk '{print $3}')
		if [ -S "$SOCKET_CUSTOM" ]; then
			SOCKET="$SOCKET_CUSTOM"
		fi
	else
		haproxy -c -f /opt/percona/haproxy-global.cfg -f $path_to_haproxy_cfg/haproxy.cfg
	fi

	if [ -n "$main_node" ]; then
		if /opt/percona/haproxy_check_pxc.sh '' '' "$main_node"; then
			for backup_server in "${NODE_LIST_BACKUP[@]}"; do
				log "shutdown sessions server $backup_server | socat stdio ${SOCKET}"
				echo "shutdown sessions server $backup_server" | socat stdio "${SOCKET}"
			done
		fi
	fi

	if [ -S "$path_to_haproxy_cfg/haproxy-main.sock" ]; then
		log "reload | socat stdio $path_to_haproxy_cfg/haproxy-main.sock"
		echo 'reload' | socat stdio "$path_to_haproxy_cfg/haproxy-main.sock"
	fi
}

main
exit 0
