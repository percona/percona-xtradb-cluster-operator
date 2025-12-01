#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql-entrypoint.sh /opt/percona/proxysql-entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql.cnf /opt/percona/proxysql.cnf
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql-admin.cnf /opt/percona/proxysql-admin.cnf
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql_add_cluster_nodes.sh /opt/percona/proxysql_add_cluster_nodes.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql_add_proxysql_nodes.sh /opt/percona/proxysql_add_proxysql_nodes.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql_add_pxc_nodes.sh /opt/percona/proxysql_add_pxc_nodes.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql_scheduler_config.tmpl /opt/percona/proxysql_scheduler_config.tmpl

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /proxysql_peer_list_entrypoint.sh /opt/percona/proxysql_peer_list_entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /peer-list /opt/percona/peer-list
