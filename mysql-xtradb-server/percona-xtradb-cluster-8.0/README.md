Percona XtraDB Cluster docker image
===================================

The docker image is available right now at `percona/percona-xtradb-cluster:8.0`.
The image supports work in Docker Network, including overlay networks,
so that you can install Percona XtraDB Cluster nodes on different boxes.
There is an initial support for the etcd discovery service.

Basic usage
-----------

For an example, see the `start_node.sh` script.

The `CLUSTER_NAME` environment variable should be set, and the easiest to do it is:
`export CLUSTER_NAME=cluster1`

The script will try to create an overlay network `${CLUSTER_NAME}_net`.
If you want to have a bridge network or network with a specific parameter,
create it in advance.
For example:
`docker network create -d bridge ${CLUSTER_NAME}_net`

The Docker image accepts the following parameters:
* One of `MYSQL_ROOT_PASSWORD`, `MYSQL_ALLOW_EMPTY_PASSWORD` or `MYSQL_RANDOM_ROOT_PASSWORD` must be defined
* The image will create the user `xtrabackup@localhost` for the XtraBackup SST method. If you want to use a password for the `xtrabackup` user, set `XTRABACKUP_PASSWORD`. 
* If you want to use the discovery service (right now only `etcd` is supported), set the address to `DISCOVERY_SERVICE`. The image will automatically find a running cluser by `CLUSTER_NAME` and join to the existing cluster (or start a new one).
* If you want to start without the discovery service, use the `CLUSTER_JOIN` variable. Empty variables will start a new cluster, To join an existing cluster, set `CLUSTER_JOIN` to the list of IP addresses running cluster nodes.


Discovery service
-----------------

The cluster will try to register itself in the discovery service, so that new nodes or ProxySQL can easily find running nodes.

Assuming you have the variable `ETCD_HOST` set to `IP:PORT` of the running etcd (e.g., `export ETCD_HOST=10.20.2.4:2379`), you can explore the current settings by  using
`curl http://$ETCD_HOST/v2/keys/pxc-cluster/$CLUSTER_NAME/?recursive=true  | jq`.

Example output:
```
{
  "action": "get",
  "node": {
    "key": "/pxc-cluster/cluster4",
    "dir": true,
    "nodes": [
      {
        "key": "/pxc-cluster/cluster4/10.0.5.2",
        "dir": true,
        "nodes": [
          {
            "key": "/pxc-cluster/cluster4/10.0.5.2/ipaddr",
            "value": "10.0.5.2",
            "modifiedIndex": 19600,
            "createdIndex": 19600
          },
          {
            "key": "/pxc-cluster/cluster4/10.0.5.2/hostname",
            "value": "2af0a75ce0cb",
            "modifiedIndex": 19601,
            "createdIndex": 19601
          }
        ],
        "modifiedIndex": 19600,
        "createdIndex": 19600
      },
      {
        "key": "/pxc-cluster/cluster4/10.0.5.3",
        "dir": true,
        "nodes": [
          {
            "key": "/pxc-cluster/cluster4/10.0.5.3/ipaddr",
            "value": "10.0.5.3",
            "modifiedIndex": 26420,
            "createdIndex": 26420
          },
          {
            "key": "/pxc-cluster/cluster4/10.0.5.3/hostname",
            "value": "cfb29833f1d6",
            "modifiedIndex": 26421,
            "createdIndex": 26421
          }
        ],
        "modifiedIndex": 26420,
        "createdIndex": 26420
      }
    ],
    "modifiedIndex": 19600,
    "createdIndex": 19600
  }
}
```

Currently there is no automatic cleanup for the discovery service registry. You can remove all entries using
`curl http://$ETCD_HOST/v2/keys/pxc-cluster/$CLUSTER_NAME?recursive=true -XDELETE`.

Starting a discovery service
--------------------------

For the full documentation, please check https://coreos.com/etcd/docs/latest/docker_guide.html.

A simple script to start 1-node etcd (assuming `ETCD_HOST` variable is defined) is:

```
ETCD_HOST=${ETCD_HOST:-10.20.2.4:2379}
docker run -d -v /usr/share/ca-certificates/:/etc/ssl/certs -p 4001:4001 -p 2380:2380 -p 2379:2379 \
 --name etcd quay.io/coreos/etcd \
 -name etcd0 \
 -advertise-client-urls http://${ETCD_HOST}:2379,http://${ETCD_HOST}:4001 \
 -listen-client-urls http://0.0.0.0:2379,http://0.0.0.0:4001 \
 -initial-advertise-peer-urls http://${ETCD_HOST}:2380 \
 -listen-peer-urls http://0.0.0.0:2380 \
 -initial-cluster-token etcd-cluster-1 \
 -initial-cluster etcd0=http://${ETCD_HOST}:2380 \
 -initial-cluster-state new
``` 

Running a Docker overlay network
------------------------------

The following link is a great introduction with easy steps on how to run a Docker overlay network: http://chunqi.li/2015/11/09/docker-multi-host-networking/


Running with ProxySQL
---------------------

The ProxySQL image https://hub.docker.com/r/perconalab/proxysql/
provides an integration with Percona XtraDB Cluster and discovery service.

You can start proxysql image by
```
docker run -d -p 3306:3306 -p 6032:6032 --net=$NETWORK_NAME --name=${CLUSTER_NAME}_proxysql \
        -e CLUSTER_NAME=$CLUSTER_NAME \
        -e ETCD_HOST=$ETCD_HOST \
        -e MYSQL_ROOT_PASSWORD=Theistareyk \
        -e MYSQL_PROXY_USER=proxyuser \
        -e MYSQL_PROXY_PASSWORD=s3cret \
        perconalab/proxysql
```

where `MYSQL_ROOT_PASSWORD` is the root password for the MySQL nodes. The password is needed to register the proxy user. The user `MYSQL_PROXY_USER` with password `MYSQL_PROXY_PASSWORD` will be registered on all Percona XtraDB Cluster nodes.


Running `docker exec -it ${CLUSTER_NAME}_proxysql add_cluster_nodes.sh` will register all nodes in the ProxySQL.

