#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_check_pxc.sh /opt/percona/haproxy_check_pxc.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_add_pxc_nodes.sh /opt/percona/haproxy_add_pxc_nodes.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_readiness-check.sh /opt/percona/haproxy_readiness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy_liveness-check.sh /opt/percona/haproxy_liveness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy/haproxy.cfg /opt/percona/haproxy.cfg
