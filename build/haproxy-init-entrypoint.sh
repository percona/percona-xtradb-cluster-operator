#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /check_pxc.sh /usr/local/bin/check_pxc.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /add_pxc_nodes.sh /usr/bin/add_pxc_nodes.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /readiness-check.sh /usr/local/bin/readiness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /liveness-check.sh /usr/local/bin/liveness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy.cfg /etc/haproxy/haproxy.cfg
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /haproxy-global.cfg /etc/haproxy/haproxy-global.cfg
