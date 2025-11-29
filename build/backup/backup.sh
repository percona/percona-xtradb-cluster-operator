#!/bin/bash

set -o errexit
set -o xtrace

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/backup.sh
. ${LIB_PATH}/backup.sh
# shellcheck source=build/backup/lib/pxc/vault.sh
. ${LIB_PATH}/vault.sh
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh
# shellcheck source=build/backup/lib/pxc/check-version.sh
. ${LIB_PATH}/check-version.sh

GARBD_OPTS=""

function check_ssl() {
	CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
	if [ -f /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt ]; then
		CA=/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
	fi
	SSL_DIR=${SSL_DIR:-/etc/mysql/ssl}
	if [ -f "${SSL_DIR}"/ca.crt ]; then
		CA=${SSL_DIR}/ca.crt
	fi
	SSL_INTERNAL_DIR=${SSL_INTERNAL_DIR:-/etc/mysql/ssl-internal}
	if [ -f "${SSL_INTERNAL_DIR}"/ca.crt ]; then
		CA=${SSL_INTERNAL_DIR}/ca.crt
	fi

	KEY=${SSL_DIR}/tls.key
	CERT=${SSL_DIR}/tls.crt
	if [ -f "${SSL_INTERNAL_DIR}"/tls.key ] && [ -f "${SSL_INTERNAL_DIR}"/tls.crt ]; then
		KEY=${SSL_INTERNAL_DIR}/tls.key
		CERT=${SSL_INTERNAL_DIR}/tls.crt
	fi

	if [ -f "$CA" ] && [ -f "$KEY" ] && [ -f "$CERT" ]; then
		GARBD_OPTS="socket.ssl_ca=${CA};socket.ssl_cert=${CERT};socket.ssl_key=${KEY};socket.ssl_cipher=;pc.weight=0;${GARBD_OPTS}"
	fi
}

function get_backup_source() {
	CLUSTER_SIZE=$(/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="$PXC_SERVICE" 2>&1 \
		| grep wsrep_cluster_size \
		| sort \
		| tail -1 \
		| cut -d : -f 12)

	if [ -z "${CLUSTER_SIZE}" ]; then
		exit 1
	fi

	FIRST_NODE=$(/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="$PXC_SERVICE" 2>&1 \
		| grep wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary \
		| sort -r \
		| tail -1 \
		| cut -d : -f 2 \
		| cut -d . -f 1)

	SKIP_FIRST_POD='|'
	if ((${CLUSTER_SIZE:-0} > 1)); then
		SKIP_FIRST_POD="$FIRST_NODE"
	fi
	/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="$PXC_SERVICE" 2>&1 \
		| grep wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary \
		| grep -v "$SKIP_FIRST_POD" \
		| sort \
		| tail -1 \
		| cut -d : -f 2 \
		| cut -d . -f 1
}

# The general idea of the backup is the following:
# - We start a script listening on port 4444 for sst and a snapshot of all databases. This is the script handling the backup (downloading it and then saving it where expected).
#   - This script will receive things in 2 batches: sst first, then the compressed snapshot of databases. For this reason, it will invoke socat 2 times.
# - We start garbd, joining the cluster and asking for sst and backup. Notice that garbd will terminate before we have the database snapshots because it ends as soon as sst is
#   transferred (even if we would wait for the snapshot inside the recv script, garbd would exit with error).
# - When garbd is completed, we expect the first stream (sst) to be completed. If it is not, we have failed somehow and the script needs to kill the background job.
# - Even if we have sst locally, we double check that garbd logs do not contain unexpected log messages. If they do, we stop everything and, again, kill the background job.
# - If sst is completed, we simply wait for the second stream to complete (or fail) on its own. Notice that here we could wait forever, in case the sender does not close the tcp connection.
function request_streaming() {
	local LOCAL_IP
	local NODE_NAME
	local RUN_BACKUP_PID
	LOCAL_IP=$(hostname -i | sed -E 's/.*\b([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})\b.*/\1/')
	NODE_NAME=$(get_backup_source)

	if [ -z "$NODE_NAME" ]; then
		/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="$PXC_SERVICE"
		log 'ERROR' 'Cannot find node for backup'
		log 'ERROR' 'Backup was finished unsuccessful'
		exit 1
	fi

	set +o errexit

	log 'INFO' 'Starting backup listening script in background'
	/opt/percona/backup/run_backup.sh &
	RUN_BACKUP_PID=$!

	log 'INFO' 'Garbd was started'
	garbd \
		--address "gcomm://$NODE_NAME.$PXC_SERVICE?gmcast.listen_addr=tcp://0.0.0.0:4567" \
		--donor "$NODE_NAME" \
		--group "$PXC_SERVICE" \
		--options "$GARBD_OPTS" \
		--sst "xtrabackup-v2:$LOCAL_IP:4444/xtrabackup_sst//1" \
		--recv-script="/opt/percona/backup/wait_run_backup.sh" 2>&1 | tee /tmp/garbd.log

	# If sst is not done, we do not have sst info locally, we need to abort and kill the background socat job.
	if [ ! -f /tmp/sst-is-done ]; then
		log 'ERROR' 'Garbd is done, but we are still awaiting SST data and stuck with socat open, unexpected'
		kill "$RUN_BACKUP_PID"
		exit 1
	fi

	# If sst is done, we check the logs. In case something is wrong in garbd logs, there is no point waiting for
	# snapshot transfer. In that case, we want to kill any running socat and exit.
	local sst_info_path
	if [[ -n $S3_BUCKET || -n $AZURE_CONTAINER_NAME ]]; then
		sst_info_path="/tmp/${SST_INFO_NAME}"
	else
		sst_info_path="${BACKUP_DIR}/${SST_INFO_NAME}"
	fi
	MYSQL_VERSION=$(parse_ini 'mysql-version' "$sst_info_path")
	if ! check_for_version "$MYSQL_VERSION" '8.0.0'; then
		if grep 'State transfer request failed' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
		if grep 'WARN: Protocol violation. JOIN message sender ... (garb) is not in state transfer' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
		if grep 'WARN: Rejecting JOIN message from ... (garb): new State Transfer required.' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
		if grep 'INFO: Shifting CLOSED -> DESTROYED (TO: -1)' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
		if ! grep 'INFO: Sending state transfer request' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
	else
		if grep 'Will never receive state. Need to abort' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
		if grep 'Donor is no longer in the cluster, interrupting script' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
		if grep 'failed: Invalid argument' /tmp/garbd.log; then
			kill "$RUN_BACKUP_PID"
			exit 1
		fi
	fi

	log 'INFO' 'Garbd is done. Waiting for the main transfer'
	wait $RUN_BACKUP_PID

	if [ -f /tmp/backup-is-completed ]; then
		log 'INFO' 'Backup was finished successfully'
		exit 0
	fi

	log 'ERROR' 'Backup was finished unsuccessful'
	exit 1
}

check_ssl

if [ -n "${S3_BUCKET}" ]; then
	clean_backup_s3
elif [ -n "$AZURE_CONTAINER_NAME" ]; then
	clean_backup_azure
fi

request_streaming
