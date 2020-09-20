#! /bin/bash

# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script writes out a mysql galera config using a list of newline seperated
# peer DNS names it accepts through stdin.

# /etc/mysql is assumed to be a shared volume so we can modify my.cnf as required
# to keep the config up to date, without wrapping mysqld in a custom pid1.
# The config location is intentionally not /etc/mysql/my.cnf because the
# standard base image clobbers that location.

set -o errexit
set -o xtrace

function join {
    local IFS="$1"; shift; echo "$*";
}

function mysql_root_exec() {
  local server="$1"
  local query="$2"
  { set +x; } 2>/dev/null
  MYSQL_PWD="${OPERATOR_ADMIN_PASSWORD:-operator}" timeout 600 mysql -h "${server}" -P 33062 -uoperator -s -NB -e "${query}"
  set -x
}

NODE_IP=$(hostname -I | awk ' { print $1 } ')
CLUSTER_NAME="$(hostname -f | cut -d'.' -f2)"
SERVER_ID=${HOSTNAME/$CLUSTER_NAME-}
NODE_NAME=$(hostname -f)
NODE_PORT=3306

while read -ra LINE; do
    echo "read line $LINE"
    LINE_IP=$(getent hosts "$LINE" | awk '{ print $1 }')
    if [ "$LINE_IP" != "$NODE_IP" ]; then
        LINE_HOST=$(mysql_root_exec "$LINE_IP" 'select @@hostname' || :)
        if [ -n "$LINE_HOST" ]; then
            PEERS=("${PEERS[@]}" $LINE_HOST)
            PEERS_FULL=("${PEERS_FULL[@]}" "$LINE_HOST.$CLUSTER_NAME")
        else
            PEERS_FULL=("${PEERS_FULL[@]}" $LINE_IP)
        fi
    fi
done

if [ "${#PEERS[@]}" != 0 ]; then
    DONOR_ADDRESS="$(printf '%s\n' "${PEERS[@]}" "${HOSTNAME}" | sort --version-sort | uniq | grep -v -- '-0$' | sed '$d' | tr '\n' ',' | sed 's/^,$//')"
fi
if [ "${#PEERS_FULL[@]}" != 0 ]; then
    WSREP_CLUSTER_ADDRESS="$(printf '%s\n' "${PEERS_FULL[@]}" | sort --version-sort | tr '\n' ',' | sed 's/,$//')"
fi

CFG=/etc/mysql/node.cnf
MYSQL_VERSION=$(mysqld -V | awk '{print $3}' | awk -F'.' '{print $1"."$2}')
if [ "$MYSQL_VERSION" == '8.0' ]; then
    egrep -q "^[#]?admin-address" "$CFG" || sed '/^\[mysqld\]/a admin-address=\n' ${CFG} 1<> ${CFG}
else
    egrep -q "^[#]?extra_max_connections" "$CFG" || sed '/^\[mysqld\]/a extra_max_connections=\n' ${CFG} 1<> ${CFG}
    egrep -q "^[#]?extra_port" "$CFG" || sed '/^\[mysqld\]/a extra_port=\n' ${CFG} 1<> ${CFG}
fi

egrep -q "^[#]?wsrep_sst_donor" "$CFG" || sed '/^\[mysqld\]/a wsrep_sst_donor=\n' ${CFG} 1<> ${CFG}
egrep -q "^[#]?wsrep_node_incoming_address" "$CFG" || sed '/^\[mysqld\]/a wsrep_node_incoming_address=\n' ${CFG} 1<> ${CFG}
sed -r "s|^[#]?server_id=.*$|server_id=1${SERVER_ID}|" ${CFG} 1<> ${CFG}
sed -r "s|^[#]?wsrep_node_address=.*$|wsrep_node_address=${NODE_IP}|" ${CFG} 1<> ${CFG}
sed -r "s|^[#]?wsrep_cluster_name=.*$|wsrep_cluster_name=${CLUSTER_NAME%'-pxc'}|" ${CFG} 1<> ${CFG}
sed -r "s|^[#]?wsrep_sst_donor=.*$|wsrep_sst_donor=${DONOR_ADDRESS}|" ${CFG} 1<> ${CFG}
sed -r "s|^[#]?wsrep_cluster_address=.*$|wsrep_cluster_address=gcomm://${WSREP_CLUSTER_ADDRESS}|" ${CFG} 1<> ${CFG}
sed -r "s|^[#]?wsrep_node_incoming_address=.*$|wsrep_node_incoming_address=${NODE_NAME}:${NODE_PORT}|" ${CFG} 1<> ${CFG}
{ set +x; } 2>/dev/null
sed -r "s|^[#]?wsrep_sst_auth=.*$|wsrep_sst_auth='xtrabackup:$XTRABACKUP_PASSWORD'|" ${CFG} 1<> ${CFG}
set -x
sed -r "s|^[#]?admin-address=.*$|admin-address=${NODE_IP}|" ${CFG} 1<> ${CFG}
sed -r "s|^[#]?extra_max_connections=.*$|extra_max_connections=100|" ${CFG} 1<> ${CFG}
sed -r "s|^[#]?extra_port=.*$|extra_port=33062|" ${CFG} 1<> ${CFG}

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

if [ -f $CA -a -f $KEY -a -f $CERT ]; then
    sed "/^\[mysqld\]/a pxc-encrypt-cluster-traffic=ON\nssl-ca=$CA\nssl-key=$KEY\nssl-cert=$CERT" ${CFG} 1<> ${CFG}
else
    sed "/^\[mysqld\]/a pxc-encrypt-cluster-traffic=OFF" ${CFG} 1<> ${CFG}
fi

# don't need a restart, we're just writing the conf in case there's an
# unexpected restart on the node.
