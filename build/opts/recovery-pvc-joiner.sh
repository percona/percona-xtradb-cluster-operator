#!/bin/bash

set -o errexit
set -o xtrace

echo "recovery-pvc-joiner.sh test"

current=$(realpath $(dirname $0))
. ${current}/check-version.sh
. ${current}/vault.sh


SOCAT_OPTS="TCP:${RESTORE_SRC_SERVICE}:3307,retry=30"
function check_ssl() {
    CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    if [ -f /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt ]; then
        CA=/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    fi
    SSL_DIR=${SSL_DIR:-/etc/mysql/ssl}
    if [ -f ${SSL_DIR}/ca.crt ]; then
        CA=${SSL_DIR}/ca.crt
    fi
    SSL_INTERNAL_DIR=${SSL_INTERNAL_DIR:-/etc/mysql/ssl-internal}
    if [ -f ${SSL_INTERNAL_DIR}/ca.crt ]; then
        CA=${SSL_INTERNAL_DIR}/ca.crt
    fi

    KEY=${SSL_DIR}/tls.key
    CERT=${SSL_DIR}/tls.crt
    if [ -f ${SSL_INTERNAL_DIR}/tls.key -a -f ${SSL_INTERNAL_DIR}/tls.crt ]; then
        KEY=${SSL_INTERNAL_DIR}/tls.key
        CERT=${SSL_INTERNAL_DIR}/tls.crt
    fi

    if [ -f "$CA" -a -f "$KEY" -a -f "$CERT" ]; then
        SOCAT_OPTS="openssl-connect:${RESTORE_SRC_SERVICE}:3307,reuseaddr,cert=${CERT},key=${KEY},cafile=${CA},verify=1,commonname='',retry=30"
    fi
}

check_ssl
ping -c1 $RESTORE_SRC_SERVICE || :
rm -rf /datadir/*
tmp=$(mktemp --tmpdir --directory pxc_sst_XXXX)

socat -u "$SOCAT_OPTS" stdio >$tmp/sst_info
socat -u "$SOCAT_OPTS" stdio | xbstream -x -C $tmp --parallel=$(grep -c processor /proc/cpuinfo)

set +o xtrace
transition_key=$(vault_get $tmp/sst_info)
if [[ -n $transition_key && $transition_key != null ]]; then
    MYSQL_VERSION=$(parse_ini 'mysql-version' "$tmp/sst_info")
    if compare_versions "$MYSQL_VERSION" '<' '5.7.29' &&
        [[ $MYSQL_VERSION != '5.7.28-31-57.2' ]]; then
         transition_key="\$transition_key"
    fi

    transition_option="--transition-key=$transition_key"
    master_key_options="--generate-new-master-key"
    echo transition-key exists
fi

echo "+ xtrabackup ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare --binlog-info=ON --rollback-prepared-trx \
    --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

xtrabackup ${XB_USE_MEMORY+--use-memory=$XB_USE_MEMORY} --prepare --binlog-info=ON $transition_option --rollback-prepared-trx \
    --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp

echo "+ xtrabackup --defaults-group=mysqld --datadir=/datadir --move-back --binlog-info=ON \
    --force-non-empty-directories $master_key_options \
    --keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
    --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp"

xtrabackup --defaults-group=mysqld --datadir=/datadir --move-back --binlog-info=ON \
    --force-non-empty-directories $transition_option $master_key_options \
    --keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf --early-plugin-load=keyring_vault.so \
    --xtrabackup-plugin-dir=/usr/lib64/xtrabackup/plugin --target-dir=$tmp

rm -rf $tmp
