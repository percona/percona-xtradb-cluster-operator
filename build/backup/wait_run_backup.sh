#!/bin/bash

set -o errexit
set -o xtrace

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/vault.sh
. ${LIB_PATH}/vault.sh
# shellcheck source=build/backup/lib/pxc/backup.sh
. ${LIB_PATH}/backup.sh
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh

until [ -f /tmp/sst-is-done ]
do
	if [ -f /tmp/backup-is-failed ]; then
		log 'ERROR' 'Backup is failed, interrupting the script'
		exit 1
	fi
	log 'INFO' 'waiting for SST transfer to be completed..'
	sleep 5
done
