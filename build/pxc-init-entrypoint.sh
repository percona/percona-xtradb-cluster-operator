#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-entrypoint.sh /var/lib/mysql/pxc-entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /unsafe-bootstrap.sh /var/lib/mysql/unsafe-bootstrap.sh
mkdir /var/lib/mysql/opts
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /opts/* -t /var/lib/mysql/opts
