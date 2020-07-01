#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-entrypoint.sh /var/lib/mysql/pxc-entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /unsafe-bootstrap.sh /var/lib/mysql/unsafe-bootstrap.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-configure-pxc.sh /var/lib/mysql/pxc-configure-pxc.sh
