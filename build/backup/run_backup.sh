#!/bin/bash

set -o errexit
set -o xtrace
set -m

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/vault.sh
. ${LIB_PATH}/vault.sh
# shellcheck source=build/backup/lib/pxc/backup.sh
. ${LIB_PATH}/backup.sh
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh

SOCAT_OPTS="TCP-LISTEN:4444,reuseaddr,retry=30"

check_ssl() {
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
		SOCAT_OPTS="openssl-listen:4444,reuseaddr,cert=${CERT},key=${KEY},cafile=${CA},verify=1,retry=30"
	fi
}

# shellcheck disable=SC2317
handle_sigterm() {
	pid_s=$(ps -C socat -o pid= || true)
	if [ -n "${pid_s}" ]; then
		kill "$pid_s"
		log 'ERROR' 'Sigterm during a socat'
		touch /tmp/backup-is-failed
		exit 1
	fi
}

backup_volume() {
	BACKUP_DIR=${BACKUP_DIR:-/backup/$PXC_SERVICE-$(date +%F-%H-%M)}
	if [ -d "$BACKUP_DIR" ]; then
		rm -rf "$BACKUP_DIR"/{xtrabackup.*,sst_info}
	fi

	mkdir -p "$BACKUP_DIR"
	cd "$BACKUP_DIR" || exit

	log 'INFO' "Backup to $BACKUP_DIR was started"

	local socat_status

	log 'INFO' 'Starting first socat for SST'
	# shellcheck disable=SC2086
	socat -u "$SOCAT_OPTS" stdio | xbstream -x $XBSTREAM_EXTRA_ARGS &
	wait $!
	socat_status=$?

	if [[ ${socat_status} -ne 0 ]]; then
		log 'ERROR' 'Socat(1) failed'
		log 'ERROR' 'Backup was finished unsuccessfully'
		touch /tmp/backup-is-failed
		exit 1
	fi

	if [ ! -f "$BACKUP_DIR/${SST_INFO_NAME}" ]; then
		log 'ERROR' 'SST file not found, while Socat was successful, unexpected'
		touch /tmp/backup-is-failed
		exit 1
	fi
	log 'INFO' 'SST transfer completed'
	touch /tmp/sst-is-done

	vault_store "$BACKUP_DIR/${SST_INFO_NAME}"

	if ! socat -u "$SOCAT_OPTS" stdio >xtrabackup.stream; then
		log 'ERROR' 'Socat(2) failed'
		log 'ERROR' 'Backup was finished unsuccessfully'
		touch /tmp/backup-is-failed
		exit 1
	fi
	log 'INFO' 'Snapshot transfer completed'

	stat xtrabackup.stream
	if (($(stat -c%s xtrabackup.stream) < 5000000)); then
		log 'ERROR' 'Backup is empty'
		log 'ERROR' 'Backup was finished unsuccessfully'
		touch /tmp/backup-is-failed
		exit 1
	fi
	md5sum xtrabackup.stream | tee md5sum.txt

	log 'INFO' 'Backup finished'
	touch /tmp/backup-is-completed
}

backup_s3() {
	#S3_BUCKET_PATH=${S3_BUCKET_PATH:-$PXC_SERVICE-$(date +%F-%H-%M)-xtrabackup.stream}

	s3_add_bucket_dest
	local socat_status

	log 'INFO' 'Starting first socat for SST'
	# shellcheck disable=SC2086
	socat -u "$SOCAT_OPTS" stdio | xbstream -x -C /tmp $XBSTREAM_EXTRA_ARGS &
	wait $!
	socat_status=$?

	if [[ ${socat_status} -ne 0 ]]; then
		log 'ERROR' 'Socat(1) failed'
		log 'ERROR' 'Backup was finished unsuccessfully'
		touch /tmp/backup-is-failed
		exit 1
	fi

	if [ ! -f /tmp/"${SST_INFO_NAME}" ]; then
		log 'ERROR' 'SST file not found, while Socat was successful, unexpected'
		touch /tmp/backup-is-failed
		exit 1
	fi
	log 'INFO' 'SST transfer completed'
	touch /tmp/sst-is-done

	vault_store /tmp/${SST_INFO_NAME}

	# shellcheck disable=SC2086
	xbstream -C /tmp -c ${SST_INFO_NAME} $XBSTREAM_EXTRA_ARGS \
		| xbcloud put --storage=s3 --parallel="$(grep -c processor /proc/cpuinfo)" --md5 $XBCLOUD_ARGS --s3-bucket="$S3_BUCKET" "$S3_BUCKET_PATH.$SST_INFO_NAME" 2>&1 \
		| (grep -v "error: http request failed: Couldn't resolve host name" || exit 1)

	# shellcheck disable=SC2086
	socat -u "$SOCAT_OPTS" stdio \
		| xbcloud put --storage=s3 --parallel="$(grep -c processor /proc/cpuinfo)" --md5 $XBCLOUD_ARGS --s3-bucket="$S3_BUCKET" "$S3_BUCKET_PATH" 2>&1 \
		| (grep -v "error: http request failed: Couldn't resolve host name" || exit 1) \
		| tee /tmp/xbcloud-snapshot.log

	if grep 'Upload failed: backup is incomplete' /tmp/xbcloud-snapshot.log; then
		log 'ERROR' 'Upload is incomplete, maybe the donor interrupted the connection?'
		touch /tmp/backup-is-failed
		exit 1
	fi
	log 'INFO' 'Snapshot transfer completed'

	# shellcheck disable=SC2086
	aws $AWS_S3_NO_VERIFY_SSL s3 ls "s3://$S3_BUCKET/$S3_BUCKET_PATH.md5"
	# shellcheck disable=SC2086
	md5_size=$(aws $AWS_S3_NO_VERIFY_SSL --output json s3api list-objects --bucket "$S3_BUCKET" --prefix "$S3_BUCKET_PATH.md5" --query 'Contents[0].Size' | sed -e 's/.*"size":\([0-9]*\).*/\1/')
	if [[ $md5_size =~ "Object does not exist" ]] || ((md5_size < 23000)); then
		log 'ERROR' 'Backup is empty'
		log 'ERROR' 'Backup was finished unsuccessfull'
		touch /tmp/backup-is-failed
		exit 1
	fi

	log 'INFO' 'Backup finished'
	touch /tmp/backup-is-completed
}

backup_azure() {
	#BACKUP_PATH=${BACKUP_PATH:-$PXC_SERVICE-$(date +%F-%H-%M)-xtrabackup.stream}
	ENDPOINT=${AZURE_ENDPOINT:-"https://$AZURE_STORAGE_ACCOUNT.blob.core.windows.net"}

	log 'INFO' "Backup to $ENDPOINT/$AZURE_CONTAINER_NAME/$BACKUP_PATH"

	local socat_status
	log 'INFO' 'Starting first socat for SST'
	# shellcheck disable=SC2086
	socat -u "$SOCAT_OPTS" stdio | xbstream -x -C /tmp $XBSTREAM_EXTRA_ARGS &
	wait $!
	socat_status=$?
	log 'INFO' 'Socat was started'

	if [[ ${socat_status} -ne 0 ]]; then
		log 'ERROR' 'Socat(1) failed'
		log 'ERROR' 'Backup was finished unsuccessfully'
		touch /tmp/backup-is-failed
		exit 1
	fi

	if [ ! -f /tmp/${SST_INFO_NAME} ]; then
		log 'ERROR' 'SST file not found, while Socat was successful, unexpected'
		touch /tmp/backup-is-failed
		exit 1
	fi
	log 'INFO' 'SST transfer completed'
	touch /tmp/sst-is-done

	vault_store /tmp/${SST_INFO_NAME}

	# shellcheck disable=SC2086
	xbstream -C /tmp -c ${SST_INFO_NAME} $XBSTREAM_EXTRA_ARGS \
		| xbcloud put --storage=azure --parallel="$(grep -c processor /proc/cpuinfo)" $XBCLOUD_ARGS "$BACKUP_PATH.$SST_INFO_NAME" 2>&1 \
		| (grep -v "error: http request failed: Couldn't resolve host name" || exit 1)

	# shellcheck disable=SC2086
	socat -u "$SOCAT_OPTS" stdio \
		| xbcloud put --storage=azure --parallel="$(grep -c processor /proc/cpuinfo)" $XBCLOUD_ARGS "$BACKUP_PATH" 2>&1 \
		| (grep -v "error: http request failed: Couldn't resolve host name" || exit 1) \
		| tee /tmp/xbcloud-snapshot.log

	if grep 'Upload failed: backup is incomplete' /tmp/xbcloud-snapshot.log; then
		log 'ERROR' 'Upload is incomplete, maybe the donor interrupted the connection?'
		touch /tmp/backup-is-failed
		exit 1
	fi
	log 'INFO' 'Snapshot transfer completed'

	log 'INFO' 'Backup finished'
	touch /tmp/backup-is-completed
}

check_ssl

trap handle_sigterm 15

if [ -n "$S3_BUCKET" ]; then
	backup_s3
elif [ -n "$AZURE_CONTAINER_NAME" ]; then
	backup_azure
else
	backup_volume
fi
