#!/bin/bash

set -o errexit
set -o xtrace

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/check-version.sh
. ${LIB_PATH}/check-version.sh
# shellcheck source=build/backup/lib/pxc/vault.sh
. ${LIB_PATH}/vault.sh
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh

# temporary fix for PXB-2784
XBCLOUD_ARGS="--curl-retriable-errors=7 $XBCLOUD_EXTRA_ARGS"

if [ -n "$VERIFY_TLS" ] && [[ $VERIFY_TLS == "false" ]]; then
	XBCLOUD_ARGS="--insecure ${XBCLOUD_ARGS}"
fi

if [ -n "$S3_BUCKET_URL" ]; then
	{ set +x; } 2>/dev/null
	s3_add_bucket_dest
	set -x
	# shellcheck disable=SC2086
	aws $AWS_S3_NO_VERIFY_SSL s3 ls "${S3_BUCKET_URL}"
elif [ -n "${BACKUP_PATH}" ]; then
	XBCLOUD_ARGS="${XBCLOUD_ARGS} --storage=azure"
fi

if [ -n "${AZURE_CONTAINER_NAME}" ]; then
	XBCLOUD_ARGS="${XBCLOUD_ARGS} --azure-container-name=${AZURE_CONTAINER_NAME}"
fi

rm -rf /datadir/*
tmp=$(mktemp --directory /datadir/pxc_sst_XXXX)

destination() {
	if [ -n "${S3_BUCKET_URL}" ]; then
		echo -n "s3://${S3_BUCKET_URL}"
	elif [ -n "${BACKUP_PATH}" ]; then
		echo -n "${BACKUP_PATH}"
	fi
}

# shellcheck disable=SC2086
xbcloud get --parallel="$(grep -c processor /proc/cpuinfo)" ${XBCLOUD_ARGS} "$(destination).sst_info" | xbstream -x -C "${tmp}" --parallel="$(grep -c processor /proc/cpuinfo)" $XBSTREAM_EXTRA_ARGS

XTRABACKUP_VERSION=$(get_xtrabackup_version)
if check_for_version "$XTRABACKUP_VERSION" '8.0.0'; then
	XBSTREAM_EXTRA_ARGS="$XBSTREAM_EXTRA_ARGS --decompress"
fi

# shellcheck disable=SC2086
xbcloud get --parallel="$(grep -c processor /proc/cpuinfo)" ${XBCLOUD_ARGS} "$(destination)" | xbstream -x -C "${tmp}" --parallel="$(grep -c processor /proc/cpuinfo)" $XBSTREAM_EXTRA_ARGS

set +o xtrace

if [[ -f "${tmp}/sst_info" ]]; then
	transition_key=$(vault_get "$tmp/sst_info")
	if [[ -n $transition_key && $transition_key != null ]]; then
		MYSQL_VERSION=$(parse_ini 'mysql-version' "$tmp/sst_info")
		if ! check_for_version "$MYSQL_VERSION" '5.7.29' \
			&& [[ $MYSQL_VERSION != '5.7.28-31-57.2' ]]; then

			# shellcheck disable=SC2016
			transition_key='$transition_key'
		fi

		transition_option="--transition-key=$transition_key"
		echo transition-key exists
	fi
fi

if [ -f "${tmp}/xtrabackup_keys" ]; then
	master_key_options="--generate-new-master-key"
fi

# Extract --defaults-file from XB_EXTRA_ARGS if present and place it as the first argument
# This fixes the issue where --defaults-file must be the first argument for xtrabackup and innobackupex
DEFAULTS_FILE=""
REMAINING_XB_ARGS=""
if [[ "$XB_EXTRA_ARGS" =~ --defaults-file=([^[:space:]]+) ]]; then
	defaults_file_path="${BASH_REMATCH[1]}"
	# If the path is relative (doesn't start with /), prepend $tmp directory
	if [[ "$defaults_file_path" != /* ]]; then
		# Remove leading ./ if present
		defaults_file_path="${defaults_file_path#./}"
		defaults_file_path="$tmp/$defaults_file_path"
	fi
	DEFAULTS_FILE="--defaults-file=$defaults_file_path"
	REMAINING_XB_ARGS=$(echo "$XB_EXTRA_ARGS" | sed 's/--defaults-file=[^[:space:]]*//g' | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')
else
	REMAINING_XB_ARGS="$XB_EXTRA_ARGS"
fi

if ! check_for_version "$XTRABACKUP_VERSION" '8.0.0'; then
	# shellcheck disable=SC2086
	innobackupex ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --parallel="$(grep -c processor /proc/cpuinfo)" $REMAINING_XB_ARGS --decompress "$tmp"
	XB_EXTRA_ARGS="$XB_EXTRA_ARGS --binlog-info=ON"
fi

DEFAULTS_GROUP="--defaults-group=mysqld"
if [[ "${XTRABACKUP_ENABLED}" == "true" ]]; then
	# these must not be set for pxb
	DEFAULTS_GROUP=""
	DEFAULTS_FILE=""
fi

# If backup-my.cnf does not contian plugin_load, then --prepare will fail if you pass the --keyring-vault-config option.
if [[ -n "$(parse_ini 'plugin_load' "${tmp}/backup-my.cnf")" ]]; then
	KEYRING_VAULT_CONFIG="--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf"
fi

echo "+ xtrabackup $DEFAULTS_FILE ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare $REMAINING_XB_ARGS --binlog-info=ON --rollback-prepared-trx \
	$KEYRING_VAULT_CONFIG --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"


# shellcheck disable=SC2086
xtrabackup $DEFAULTS_FILE ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare $REMAINING_XB_ARGS $transition_option --rollback-prepared-trx \
	$KEYRING_VAULT_CONFIG --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin "--target-dir=$tmp"

echo "+ xtrabackup $DEFAULTS_FILE $DEFAULTS_GROUP --datadir=/datadir --move-back $REMAINING_XB_ARGS --binlog-info=ON \
--force-non-empty-directories $master_key_options \
--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

# shellcheck disable=SC2086
xtrabackup $DEFAULTS_FILE $DEFAULTS_GROUP --datadir=/datadir --move-back $REMAINING_XB_ARGS \
	--force-non-empty-directories $transition_option $master_key_options \
	--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
	--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin "--target-dir=$tmp"

rm -rf "$tmp"