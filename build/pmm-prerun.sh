#!/bin/bash

set -o errexit

CLUSTER_NAME="${PMM_PREFIX}${CLUSTER_NAME}"

pmm_args=()

read -ra PMM_ADMIN_CUSTOM_PARAMS_ARRAY <<<"$PMM_ADMIN_CUSTOM_PARAMS"
pmm_args+=(
	"${PMM_ADMIN_CUSTOM_PARAMS_ARRAY[@]}"
)

if [[ $DB_TYPE != "haproxy" ]]; then
	pmm_args+=(
		--service-name="$PMM_AGENT_SETUP_NODE_NAME"
		--host="$POD_NAME"
		--port="$DB_PORT"
	)
fi

if [[ $DB_TYPE == "mysql" ]]; then
	read -ra DB_ARGS_ARRAY <<<"$DB_ARGS"
	pmm_args+=(
		"${DB_ARGS_ARRAY[@]}"
	)
fi

if [[ $DB_TYPE == "haproxy" ]]; then
	pmm_args+=(
		"$PMM_AGENT_SETUP_NODE_NAME"
	)
fi

SECONDS=0
last_report=-60
until pmm-admin status --json --wait=10s | grep -q '"connected": *true'; do
	if ((SECONDS - last_report >= 60)); then
		msg="pmm-agent is not connected to PMM Server yet, retrying... (waiting for ${SECONDS}s)"
		if ((SECONDS >= 300)); then
			msg="ERROR: ${msg}; the $DB_TYPE service is not registered in PMM until the agent connects, check that PMM Server ${PMM_AGENT_SERVER_ADDRESS} is reachable from this pod"
		fi
		echo "$msg"
		last_report=$SECONDS
	fi
	sleep 5
done

pmm-admin status
pmm-admin add "$DB_TYPE" --skip-connection-check --metrics-mode=push --username="$DB_USER" --password="$DB_PASSWORD" --cluster="$CLUSTER_NAME" "${pmm_args[@]}"
pmm-admin annotate --service-name="$PMM_AGENT_SETUP_NODE_NAME" 'Service restarted'
