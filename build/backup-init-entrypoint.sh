#!/bin/bash

set -o errexit
set -o xtrace

install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /peer-list /opt/percona/peer-list
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /xtrabackup-run-backup /opt/percona/xtrabackup-run-backup

mkdir -p /opt/percona/backup/lib/pxc
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /backup/lib/pxc/* /opt/percona/backup/lib/pxc/
install -o "$(id -u)" -g "$(id -g)" -m 0755 -D /backup/recovery-*.sh backup/wait_run_backup.sh backup/run_backup.sh backup/backup.sh /opt/percona/backup/
