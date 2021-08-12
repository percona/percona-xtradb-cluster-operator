#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /ps-entrypoint.sh /var/lib/mysql/ps-entrypoint.sh
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /peer-list /var/lib/mysql/peer-list
