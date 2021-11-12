#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-entrypoint.sh /var/lib/mysql/pxc-entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /unsafe-bootstrap.sh /var/lib/mysql/unsafe-bootstrap.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-configure-pxc.sh /var/lib/mysql/pxc-configure-pxc.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /liveness-check.sh /var/lib/mysql/liveness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /readiness-check.sh /var/lib/mysql/readiness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /peer-list /var/lib/mysql/peer-list
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /get-pxc-state /var/lib/mysql/get-pxc-state
