#!/bin/bash
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pxc-entrypoint.sh /var/lib/mysql/pxc-entrypoint.sh
