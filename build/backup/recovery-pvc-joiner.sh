#!/bin/bash

set -o errexit
set -o xtrace

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/check-version.sh
. ${LIB_PATH}/check-version.sh
# shellcheck source=build/backup/lib/pxc/vault.sh
. ${LIB_PATH}/vault.sh

SOCAT_OPTS="TCP:${RESTORE_SRC_SERVICE}:3307,retry=30"
function check_ssl() {
	CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
	if [ -f /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt ]; then
		CA=/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
	fi
	SSL_DIR=${SSL_DIR:-/etc/mysql/ssl}
	if [ -f "${SSL_DIR}"/ca.crt ]; then
		CA=${SSL_DIR}/ca.crt
	fi
	SSL_INTERNAL_DIR=${SSL_INTERNAL_DIR:-/etc/mysql/ssl-internal}
	if [ -f "${SSL_INTERNAL_DIR}"/ca.crt ]; then
		CA=${SSL_INTERNAL_DIR}/ca.crt
	fi

	KEY=${SSL_DIR}/tls.key
	CERT=${SSL_DIR}/tls.crt
	if [ -f "${SSL_INTERNAL_DIR}"/tls.key ] && [ -f "${SSL_INTERNAL_DIR}"/tls.crt ]; then
		KEY=${SSL_INTERNAL_DIR}/tls.key
		CERT=${SSL_INTERNAL_DIR}/tls.crt
	fi

	if [ -f "$CA" ] && [ -f "$KEY" ] && [ -f "$CERT" ]; then
		SOCAT_OPTS="openssl-connect:${RESTORE_SRC_SERVICE}:3307,reuseaddr,cert=${CERT},key=${KEY},cafile=${CA},verify=1,commonname='',retry=30,no-sni=1"
	fi
}

check_ssl
ping -c1 "$RESTORE_SRC_SERVICE" || :
rm -rf /datadir/*
tmp=$(mktemp --directory /datadir/pxc_sst_XXXX)

socat -u "$SOCAT_OPTS" stdio >"$tmp"/sst_info

MYSQL_VERSION=$(parse_ini 'mysql-version' "$tmp/sst_info")
if check_for_version "$MYSQL_VERSION" '8.0.0'; then
	XBSTREAM_EXTRA_ARGS="$XBSTREAM_EXTRA_ARGS --decompress"
fi
# shellcheck disable=SC2086
socat -u "$SOCAT_OPTS" stdio | xbstream -x -C "$tmp" --parallel="$(grep -c processor /proc/cpuinfo)" $XBSTREAM_EXTRA_ARGS

set +o xtrace
transition_key=$(vault_get "$tmp"/sst_info)
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
	innobackupex ${XB_EXTRA_ARGS} ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --parallel="$(grep -c processor /proc/cpuinfo)" --decompress "$tmp"
	XB_EXTRA_ARGS="$XB_EXTRA_ARGS --binlog-info=ON"
fi

# Extract --defaults-file from XB_EXTRA_ARGS if present and place it as the first argument
# This fixes the issue where --defaults-file must be the first argument for xtrabackup
DEFAULTS_FILE=""
REMAINING_XB_ARGS=""
if [[ "$XB_EXTRA_ARGS" =~ --defaults-file=([^[:space:]]+) ]]; then
	DEFAULTS_FILE="--defaults-file=${BASH_REMATCH[1]}"
	REMAINING_XB_ARGS=$(echo "$XB_EXTRA_ARGS" | sed 's/--defaults-file=[^[:space:]]*//g' | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')
else
	REMAINING_XB_ARGS="$XB_EXTRA_ARGS"
fi

echo "+ xtrabackup $DEFAULTS_FILE ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare $REMAINING_XB_ARGS --binlog-info=ON --rollback-prepared-trx \
--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

# shellcheck disable=SC2086
xtrabackup $DEFAULTS_FILE ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare $REMAINING_XB_ARGS $transition_option --rollback-prepared-trx \
	--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir="$tmp"

echo "+ xtrabackup $DEFAULTS_FILE --defaults-group=mysqld --datadir=/datadir --move-back $REMAINING_XB_ARGS --binlog-info=ON \
--force-non-empty-directories $master_key_options \
--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

# shellcheck disable=SC2086
xtrabackup $DEFAULTS_FILE --defaults-group=mysqld --datadir=/datadir --move-back $REMAINING_XB_ARGS \
	--force-non-empty-directories $transition_option $master_key_options \
	--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
	--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir="$tmp"

rm -rf "$tmp"
