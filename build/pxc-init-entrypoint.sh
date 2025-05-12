#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-entrypoint.sh /var/lib/mysql/pxc-entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-configure-pxc.sh /var/lib/mysql/pxc-configure-pxc.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /liveness-check.sh /var/lib/mysql/liveness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /readiness-check.sh /var/lib/mysql/readiness-check.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /peer-list /var/lib/mysql/peer-list
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /get-pxc-state /var/lib/mysql/get-pxc-state
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pmm-prerun.sh /var/lib/mysql/pmm-prerun.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /mysql-state-monitor /var/lib/mysql/mysql-state-monitor
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /wsrep_cmd_notify_handler.sh /var/lib/mysql/wsrep_cmd_notify_handler.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /prepare_restored_cluster.sh /var/lib/mysql/prepare_restored_cluster.sh
