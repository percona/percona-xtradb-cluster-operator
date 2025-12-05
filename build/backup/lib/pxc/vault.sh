#!/bin/bash

set -o errexit

keyring_vault=/etc/mysql/vault-keyring-secret/keyring_vault.conf

function parse_ini() {
	local key=$1
	local file_path=$2

	if [ ! -f $file_path ]; then
		echo "File $file_path does not exist" >&2
		exit 0
	fi
	awk -F "=[ ]*" "/${key}[ ]*=/ {print \$2}" "$file_path"
}

function parse_json() {
	local key=$1
	local file_path=$2

	jq -r ".${key}" ${file_path}
}

function get_vault_option() {
	local key=$1

	# Vault config is json for 8.4
	if jq . ${keyring_vault} 1>&2 2>/dev/null; then
		parse_json ${key} ${keyring_vault}
	else
		parse_ini ${key} ${keyring_vault}
	fi
}

function vault_get() {
	local sst_info=$1

	if [ ! -f "${keyring_vault}" ]; then
		echo "vault configuration not found" >&2
		return 0
	fi

	if [ ! -f "${sst_info}" ]; then
		echo "SST info not found" >&2
		exit 1
	fi

	export VAULT_TOKEN=$(get_vault_option "token")
	export VAULT_ADDR=$(get_vault_option "vault_url")
	local vault_root=$(get_vault_option "secret_mount_point")/backup
	local ca_path=$(get_vault_option "vault_ca")

	local gtid=$(parse_ini "galera-gtid" "${sst_info}")

	curl ${ca_path:+--cacert $ca_path} \
		-H "X-Vault-Request: true" \
		-H "X-Vault-Token: ${VAULT_TOKEN}" \
		-H "Content-Type: application/json" \
		"${VAULT_ADDR}/v1/${vault_root}/${gtid}" \
		| jq -r '.data.transition_key'
}

function vault_store() {
	local sst_info=$1

	if [ ! -f "${keyring_vault}" ]; then
		echo "vault configuration not found" >&2
		return 0
	fi

	if [ ! -f "${sst_info}" ]; then
		echo "SST info not found" >&2
		exit 1
	fi

	set +o xtrace # hide sensitive information
	local transition_key=$(parse_ini "transition-key" "${sst_info}")
	if [ -z "${transition_key}" ]; then
		echo "no transition key in the SST info: backup is an unencrypted, or it was already processed"
		return 0
	fi

	export VAULT_TOKEN=$(get_vault_option "token")
	export VAULT_ADDR=$(get_vault_option "vault_url")
	local vault_root=$(get_vault_option "secret_mount_point")/backup
	local ca_path=$(get_vault_option "vault_ca")

	local gtid=$(parse_ini "galera-gtid" "${sst_info}")

	curl ${ca_path:+--cacert $ca_path} \
		-X PUT \
		-H "X-Vault-Request: true" \
		-H "X-Vault-Token: ${VAULT_TOKEN}" \
		-H "Content-Type: application/json" \
		-d "{\"transition_key\":\"${transition_key}\"}" \
		"${VAULT_ADDR}/v1/${vault_root}/${gtid}"

	set -o xtrace
	sed -i '/transition-key/d' "$sst_info" >/dev/null
}
