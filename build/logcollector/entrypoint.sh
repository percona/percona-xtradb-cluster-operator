#!/bin/bash

set -e
set -o xtrace

export PATH=$PATH:/opt/fluent-bit/bin

LOGROTATE_SCHEDULE="${LOGROTATE_SCHEDULE:-0 0 * * *}"

run_cron() {
	local schedule="$1"
	local cmd="$2"

	if [ -f /usr/bin/supercronic ]; then
        printf '%s %s\n' "$schedule" "$cmd" > /tmp/crontab
        exec supercronic /tmp/crontab
    else
        exec go-cron "$schedule" sh -c "$cmd"
    fi
}

is_logrotate_config_invalid() {
	local config_file="$1"
	if [ -z "$config_file" ] || [ ! -f "$config_file" ]; then
		return 1
	fi
	# Specifying -d runs in debug mode, so even in case of errors, it will exit with 0.
	# We need to check the output for "error" but skip those lines that are related to the missing logrotate.status file.
	# Filter out logrotate.status lines first, then check for remaining errors
	(
		set +e
		logrotate -d "$config_file" 2>&1 | grep -v "logrotate.status" | grep -qi "error"
	)
	return $?
}

run_logrotate() {
	local logrotate_status_file="${LOGROTATE_STATUS_FILE:-"/opt/percona/logcollector/logrotate/logrotate.status"}"
	local logrotate_conf_file="/opt/percona/logcollector/logrotate/logrotate-$SERVICE_TYPE.conf"
	local logrotate_additional_conf_files=()
	local conf_d_dir="/opt/percona/logcollector/logrotate/conf.d"

	# Check if logrotate-mysql.conf exists and validate it
	if [ -f "$conf_d_dir/logrotate-$SERVICE_TYPE.conf" ]; then
		logrotate_conf_file="$conf_d_dir/logrotate-$SERVICE_TYPE.conf"
		if is_logrotate_config_invalid "$logrotate_conf_file"; then
			echo "ERROR: Logrotate configuration is invalid, fallback to default configuration"
			logrotate_conf_file="/opt/percona/logcollector/logrotate/logrotate-$SERVICE_TYPE.conf"
		fi
	fi

	# Process all .conf files in conf.d directory (excluding logrotate-$SERVICE_TYPE.conf which is already handled)
	if [ -d "$conf_d_dir" ]; then
		for conf_file in "$conf_d_dir"/*.conf; do
			# Check if glob matched any files (if no .conf files exist, the glob returns itself)
			[ -f "$conf_file" ] || continue
			# Skip logrotate-$SERVICE_TYPE.conf as it's already processed above
			[ "$(basename "$conf_file")" = "logrotate-$SERVICE_TYPE.conf" ] && continue
			if is_logrotate_config_invalid "$conf_file"; then
				echo "ERROR: Logrotate configuration file $conf_file is invalid, it will be ignored"
			else
				logrotate_additional_conf_files+=("$conf_file")
			fi
		done
	fi
	# Ensure logrotate can run with current UID
	if [[ $EUID != 1001 ]]; then
		# logrotate requires UID in /etc/passwd
		sed -e "s^x:1001:^x:$EUID:^" /etc/passwd >/tmp/passwd
		cat /tmp/passwd >/etc/passwd
		rm -rf /tmp/passwd
	fi

	local logrotate_cmd="logrotate -s \"$logrotate_status_file\" \"$logrotate_conf_file\""
	for additional_conf in "${logrotate_additional_conf_files[@]}"; do
		logrotate_cmd="$logrotate_cmd \"$additional_conf\""
	done
	logrotate_cmd="$logrotate_cmd; /usr/bin/find /var/lib/mysql/ -name GRA_*.log -mtime +7 -delete"

	set -o xtrace
	run_cron "$LOGROTATE_SCHEDULE" "$logrotate_cmd"
}

run_fluentbit() {
	local fluentbit_opt=(-c /opt/percona/logcollector/fluentbit/fluentbit.conf)
	mkdir -p /tmp/fluentbit/custom
	set +e
	local fluentbit_conf_dir="/opt/percona/logcollector/fluentbit/custom"
	for conf_file in "$fluentbit_conf_dir"/*.conf; do
		[ -f "$conf_file" ] || continue
		if ! fluent-bit --dry-run -c "$conf_file" >/dev/null 2>&1; then
			echo "ERROR: Fluentbit configuration file $conf_file is invalid, it will be ignored"
		else
			cp "$conf_file" /tmp/fluentbit/custom/
		fi
	done
	touch /tmp/fluentbit/custom/default.conf || true

	set -e
	set -o xtrace
	# shellcheck disable=SC1091
	test -e /opt/percona/hookscript/hook.sh && source /opt/percona/hookscript/hook.sh
	exec "$@" "${fluentbit_opt[@]}"
}

case "$1" in
	logrotate)
		run_logrotate
		;;
	fluent-bit)
		run_fluentbit "$@"
		;;
	*)
		echo "Invalid argument: $1"
		exit 1
		;;
esac
