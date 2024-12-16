#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy-entrypoint.sh /opt/percona/haproxy-entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_check_pxc.sh /opt/percona/haproxy_check_pxc.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_add_pxc_nodes.sh /opt/percona/haproxy_add_pxc_nodes.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_readiness_check.sh /opt/percona/haproxy_readiness_check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_liveness_check.sh /opt/percona/haproxy_liveness_check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy.cfg /opt/percona/haproxy.cfg
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy-global.cfg /opt/percona/haproxy-global.cfg

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /peer-list /opt/percona/peer-list
