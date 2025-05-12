#!/bin/bash

set -o errexit

keyring_vault=/etc/mysql/vault-keyring-secret/keyring_vault.conf

function parse_ini() {
	local key=$1
	local file_path=$2

	awk -F "=[ ]*" "/${key}[ ]*=/ {print \$2}" "$file_path"
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

	export VAULT_TOKEN
	VAULT_TOKEN=$(parse_ini "token" "${keyring_vault}")
	export VAULT_ADDR
	VAULT_ADDR=$(parse_ini "vault_url" "${keyring_vault}")
	local vault_root
	vault_root=$(parse_ini "secret_mount_point" "${keyring_vault}")/backup
	local gtid
	gtid=$(parse_ini "galera-gtid" "${sst_info}")
	local ca_path
	ca_path=$(parse_ini "vault_ca" "${keyring_vault}")

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
	local transition_key
	transition_key=$(parse_ini "transition-key" "${sst_info}")
	if [ -z "${transition_key}" ]; then
		echo "no transition key in the SST info: backup is an unencrypted, or it was already processed"
		return 0
	fi

	export VAULT_TOKEN
	VAULT_TOKEN=$(parse_ini "token" "${keyring_vault}")
	export VAULT_ADDR
	VAULT_ADDR=$(parse_ini "vault_url" "${keyring_vault}")
	local vault_root
	vault_root=$(parse_ini "secret_mount_point" "${keyring_vault}")/backup
	local gtid
	gtid=$(parse_ini "galera-gtid" "${sst_info}")
	local ca_path
	ca_path=$(parse_ini "vault_ca" "${keyring_vault}")

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
