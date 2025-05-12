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

MYSQL_VERSION=$(parse_ini 'mysql-version' "$tmp/sst_info")
if check_for_version "$MYSQL_VERSION" '8.0.0'; then
	XBSTREAM_EXTRA_ARGS="$XBSTREAM_EXTRA_ARGS --decompress"
fi

# shellcheck disable=SC2086
xbcloud get --parallel="$(grep -c processor /proc/cpuinfo)" ${XBCLOUD_ARGS} "$(destination)" | xbstream -x -C "${tmp}" --parallel="$(grep -c processor /proc/cpuinfo)" $XBSTREAM_EXTRA_ARGS

set +o xtrace
transition_key=$(vault_get "$tmp/sst_info")
if [[ -n $transition_key && $transition_key != null ]]; then
	if ! check_for_version "$MYSQL_VERSION" '5.7.29' \
		&& [[ $MYSQL_VERSION != '5.7.28-31-57.2' ]]; then

		# shellcheck disable=SC2016
		transition_key='$transition_key'
	fi

	transition_option="--transition-key=$transition_key"
	master_key_options="--generate-new-master-key"
	echo transition-key exists
fi

if ! check_for_version "$MYSQL_VERSION" '8.0.0'; then
	# shellcheck disable=SC2086
	innobackupex ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --parallel="$(grep -c processor /proc/cpuinfo)" ${XB_EXTRA_ARGS} --decompress "$tmp"
	XB_EXTRA_ARGS="$XB_EXTRA_ARGS --binlog-info=ON"
fi

echo "+ xtrabackup ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare ${XB_EXTRA_ARGS} --binlog-info=ON --rollback-prepared-trx \
--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

# shellcheck disable=SC2086
xtrabackup ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare ${XB_EXTRA_ARGS} $transition_option --rollback-prepared-trx \
	--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin "--target-dir=$tmp"

echo "+ xtrabackup --defaults-group=mysqld --datadir=/datadir --move-back ${XB_EXTRA_ARGS} --binlog-info=ON \
--force-non-empty-directories $master_key_options \
--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

# shellcheck disable=SC2086
xtrabackup --defaults-group=mysqld --datadir=/datadir --move-back ${XB_EXTRA_ARGS} \
	--force-non-empty-directories $transition_option $master_key_options \
	--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
	--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin "--target-dir=$tmp"

rm -rf "$tmp"
