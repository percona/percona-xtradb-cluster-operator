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

pmm-admin status --wait=10s
pmm-admin add "$DB_TYPE" --skip-connection-check --metrics-mode=push --username="$DB_USER" --password="$DB_PASSWORD" --cluster="$CLUSTER_NAME" "${pmm_args[@]}"
pmm-admin annotate --service-name="$PMM_AGENT_SETUP_NODE_NAME" 'Service restarted'
