#!/bin/sh
set -e

log() {
	local message="$1"
	local date=$(/usr/bin/date +"%d/%b/%Y:%H:%M:%S.%3N")

	echo "{\"time\":\"${date}\", \"message\": \"${message}\"}"
}

if [ "$1" = 'haproxy' ]; then
	if [ ! -f '/etc/haproxy/pxc/haproxy.cfg' ]; then
		log /opt/percona/haproxy.cfg /etc/haproxy/pxc
		cp /opt/percona/haproxy.cfg /etc/haproxy/pxc
	fi

	custom_conf='/etc/haproxy-custom/haproxy-global.cfg'
	if [ -f "$custom_conf" ]; then
		log "haproxy -c -f $custom_conf -f /etc/haproxy/pxc/haproxy.cfg"
		haproxy -c -f $custom_conf -f /etc/haproxy/pxc/haproxy.cfg || EC=$?
		if [ -n "$EC" ]; then
			log "The custom config $custom_conf is not valid and will be ignored."
		fi
	fi

	haproxy_opt='-W -db '
	if [ -f "$custom_conf" -a -z "$EC" ]; then
		haproxy_opt+="-f $custom_conf "
	else
		haproxy_opt+='-f /opt/percona/haproxy-global.cfg '
	fi

	haproxy_opt+='-f /etc/haproxy/pxc/haproxy.cfg -p /etc/haproxy/pxc/haproxy.pid -S /etc/haproxy/pxc/haproxy-main.sock '
fi

log 'test -e /opt/percona/hookscript/hook.sh && source /opt/percona/hookscript/hook.sh'
test -e /opt/percona/hookscript/hook.sh && source /opt/percona/hookscript/hook.sh

DEFAULT_RLIMIT_NOFILE=1048576
RLIMIT_NOFILE=${HA_RLIMIT_NOFILE:-${DEFAULT_RLIMIT_NOFILE}}
hard_limit=$(ulimit -Hn)
if ! [[ ${RLIMIT_NOFILE} =~ ^[0-9]+$ ]]; then
	log "HA_RLIMIT_NOFILE is not a valid integer ('${RLIMIT_NOFILE}'), falling back to ${DEFAULT_RLIMIT_NOFILE}."
	RLIMIT_NOFILE=${DEFAULT_RLIMIT_NOFILE}
fi
if [[ ${hard_limit} =~ ^[0-9]+$ ]] && [[ ${RLIMIT_NOFILE} -gt ${hard_limit} ]]; then
	log "Requested open file limit (${RLIMIT_NOFILE}) exceeds hard limit (${hard_limit}), clamping."
	RLIMIT_NOFILE=${hard_limit}
fi
if ! ulimit -n "${RLIMIT_NOFILE}"; then
	log "Failed to set open file limit to ${RLIMIT_NOFILE}, continuing with $(ulimit -n)."
fi

log "$@ $haproxy_opt"
exec "$@" $haproxy_opt
