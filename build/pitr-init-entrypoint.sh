#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /pitr /opt/percona/pitr
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /peer-list /opt/percona/peer-list
