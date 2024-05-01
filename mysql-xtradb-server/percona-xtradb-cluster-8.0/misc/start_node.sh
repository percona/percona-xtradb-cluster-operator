#!/bin/bash

CLUSTER_NAME=${CLUSTER_NAME:-Theistareykjarbunga}
ETCD_HOST=${ETCD_HOST:-10.20.2.4}
NETWORK_NAME=${CLUSTER_NAME}_net

docker network create -d overlay --attachable $NETWORK_NAME

echo "Starting new node..."
docker run -d -p 3306 --net=$NETWORK_NAME \
	 -e MYSQL_ROOT_PASSWORD=Theistareyk \
	 -e DISCOVERY_SERVICE=${ETCD_HOST}:2379 \
	 -e CLUSTER_NAME=${CLUSTER_NAME} \
	 -e XTRABACKUP_PASSWORD=Theistare \
	 percona/percona-xtradb-cluster:8.0
#--general-log=1 --general_log_file=/var/lib/mysql/general.log
#--wsrep_cluster_address="gcomm://$QCOMM"
echo "Started $(docker ps -l -q)"

docker logs -f $(docker ps -l -q)
