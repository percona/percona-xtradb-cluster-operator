#!/bin/bash

set -o errexit

while [ $# -gt 0 ]; do
	case $1 in
	--status)
	STATUS=$2
	shift
	;;
	--uuid)
	CLUSTER_UUID=$2
	shift
	;;
	--primary)
	[ "$2" = "yes" ] && PRIMARY="1" || PRIMARY="0"
	shift
	;;
	--index)
	INDEX=$2
	shift
	;;
	--members)
	MEMBERS=$2
	shift
	;;
	esac

	shift
done

CLUSTER_NAME=$(hostname -f | cut -d'-' -f1)
CLUSTER_FQDN=$(hostname -f | cut -d'.' -f3-)

if [[ "$STATUS" == "joiner" ]]; then
	PITR_HOST="${CLUSTER_NAME}-pitr.${CLUSTER_FQDN}"
	if getent hosts "${PITR_HOST}" >/dev/null 2>&1; then
		curl -d "hostname=$(hostname -f)" "http://${PITR_HOST}:8080/invalidate-cache/"
	fi
fi

exit 0
