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

XTRABACKUP_VERSION=$(get_xtrabackup_version)
if check_for_version "$XTRABACKUP_VERSION" '8.0.0'; then
	XBSTREAM_EXTRA_ARGS="$XBSTREAM_EXTRA_ARGS --decompress"
fi
# shellcheck disable=SC2086
socat -u "$SOCAT_OPTS" stdio | xbstream -x -C "$tmp" --parallel="$(grep -c processor /proc/cpuinfo)" $XBSTREAM_EXTRA_ARGS

PXB_VAULT_PREPARE_ARGS=""
PXB_VAULT_MOVEBACK_ARGS=""
VAULT_CONFIG_FILE=/etc/mysql/vault-keyring-secret/keyring_vault.conf
VAULT_KEYRING_COMPONENT=/opt/percona/component_keyring_vault.cnf
if [[ -f ${VAULT_CONFIG_FILE} ]]; then
	if check_for_version "$XTRABACKUP_VERSION" '8.4.0'; then
		cp ${VAULT_CONFIG_FILE} ${VAULT_KEYRING_COMPONENT}
		PXB_VAULT_MOVEBACK_ARGS="--component-keyring-config=${VAULT_KEYRING_COMPONENT}"
		PXB_VAULT_PREPARE_ARGS="${PXB_VAULT_MOVEBACK_ARGS}"
	else
		PXB_VAULT_MOVEBACK_ARGS="--keyring-vault-config=${VAULT_CONFIG_FILE} --early-plugin-load=keyring_vault.so"
	fi
fi

set +o xtrace
transition_key=$(vault_get "$tmp"/sst_info)
if [[ -n $transition_key && $transition_key != null ]]; then
	MYSQL_VERSION=$(parse_ini 'mysql-version' "$tmp/sst_info")
	if ! check_for_version "$MYSQL_VERSION" '5.7.29' \
		&& [[ $MYSQL_VERSION != '5.7.28-31-57.2' ]]; then
		# shellcheck disable=SC2016
		transition_key='$transition_key'
	fi

	transition_option="--transition-key=$transition_key"
	master_key_options="--generate-new-master-key"
	echo transition-key exists
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

if ! check_for_version "$XTRABACKUP_VERSION" '8.0.0' \
	&& ! check_for_version "$XTRABACKUP_VERSION" '8.4.0'; then
	# shellcheck disable=SC2086
	innobackupex $DEFAULTS_FILE ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --parallel="$(grep -c processor /proc/cpuinfo)" $REMAINING_XB_ARGS --decompress "$tmp"
	XB_EXTRA_ARGS="$XB_EXTRA_ARGS --binlog-info=ON"
fi

echo "+ xtrabackup $DEFAULTS_FILE ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} \
	--prepare $REMAINING_XB_ARGS --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin \
	--target-dir=$tmp ${PXB_VAULT_PREPARE_ARG}"

# shellcheck disable=SC2086
xtrabackup $DEFAULTS_FILE ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} \
	--prepare $REMAINING_XB_ARGS $transition_option --rollback-prepared-trx \
	--xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir="$tmp" ${PXB_VAULT_PREPARE_ARGS}

echo "+ xtrabackup $DEFAULTS_FILE --defaults-group=mysqld --datadir=/datadir --move-back \
	$REMAINING_XB_ARGS --force-non-empty-directories $master_key_options \
	${PXB_VAULT_MOVEBACK_ARGS} --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

# shellcheck disable=SC2086
xtrabackup $DEFAULTS_FILE --defaults-group=mysqld --datadir=/datadir --move-back $REMAINING_XB_ARGS \
	--force-non-empty-directories $transition_option $master_key_options \
	${PXB_VAULT_MOVEBACK_ARGS} --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir="$tmp"

rm -rf "$tmp"
