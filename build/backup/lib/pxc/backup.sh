#!/bin/bash

set -o errexit

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh

SST_INFO_NAME=sst_info
XBCLOUD_ARGS="--curl-retriable-errors=7 $XBCLOUD_EXTRA_ARGS"

if [ -n "$VERIFY_TLS" ] && [[ $VERIFY_TLS == "false" ]]; then
	XBCLOUD_ARGS="--insecure ${XBCLOUD_ARGS}"
fi

S3_BUCKET_PATH=${S3_BUCKET_PATH:-$PXC_SERVICE-$(date +%F-%H-%M)-xtrabackup.stream}
BACKUP_PATH=${BACKUP_PATH:-$PXC_SERVICE-$(date +%F-%H-%M)-xtrabackup.stream}

log() {
	{ set +x; } 2>/dev/null
	local level=$1
	local message=$2
	local now
	now=$(date '+%F %H:%M:%S')

	echo "${now} [${level}] ${message}"
	set -x
}

clean_backup_s3() {
	s3_add_bucket_dest

	local time=15
	local is_deleted_full=0
	local is_deleted_info=0

	for i in {1..5}; do
		if ((i > 1)); then
			log 'INFO' "Sleeping ${time}s before retry $i..."
			sleep "$time"
		fi

		if is_object_exist "$S3_BUCKET" "$S3_BUCKET_PATH/"; then
			log 'INFO' "Delete (attempt $i)..."

			# shellcheck disable=SC2086
			xbcloud delete ${XBCLOUD_ARGS} --storage=s3 --s3-bucket="$S3_BUCKET" "$S3_BUCKET_PATH"
		else
			is_deleted_full=1
		fi

		if is_object_exist "$S3_BUCKET" "$S3_BUCKET_PATH.$SST_INFO_NAME/"; then
			log 'INFO' "Delete (attempt $i)..."

			# shellcheck disable=SC2086
			xbcloud delete ${XBCLOUD_ARGS} --storage=s3 --s3-bucket="$S3_BUCKET" "$S3_BUCKET_PATH.$SST_INFO_NAME"
		else
			is_deleted_info=1
		fi

		if [[ ${is_deleted_full} == 1 && ${is_deleted_info} == 1 ]]; then
			log 'INFO' "Object deleted successfully before attempt $i. Exiting."
			break
		fi
		((time *= 2))
	done
}

is_object_exist_azure() {
	object="$1"
	{ set +x; } 2>/dev/null

	out=$(
		HOME=/tmp/azurehome az storage blob list \
			--blob-endpoint "$ENDPOINT" \
			--container-name "$AZURE_CONTAINER_NAME" \
			--account-key "$AZURE_ACCESS_KEY" \
			--prefix "$object" \
			--auth-mode key \
			--only-show-errors \
			--num-results 1 \
			-o json
	)

	# shellcheck disable=SC2181
	if [[ $? -ne 0 ]]; then
		echo "Error: Failed to check if blob exists"
		exit 1
	fi

	if [[ $(echo "$out" | jq '. | length') == "1" ]]; then
		set -x
		return 0
	fi
	set -x
	return 1

}

clean_backup_azure() {
	ENDPOINT=${AZURE_ENDPOINT:-"https://$AZURE_STORAGE_ACCOUNT.blob.core.windows.net"}

	local time=15
	local is_deleted_full=0
	local is_deleted_info=0

	for i in {1..5}; do
		if ((i > 1)); then
			log 'INFO' "Sleeping ${time}s before retry $i..."
			sleep "$time"
		fi

		if is_object_exist_azure "$BACKUP_PATH.$SST_INFO_NAME/"; then
			log 'INFO' "Delete (attempt $i)..."
			# shellcheck disable=SC2086
			xbcloud delete ${XBCLOUD_ARGS} --storage=azure "$BACKUP_PATH.$SST_INFO_NAME"
		else
			is_deleted_info=1
		fi

		if is_object_exist_azure "$BACKUP_PATH/"; then
			log 'INFO' "Delete (attempt $i)..."
			# shellcheck disable=SC2086
			xbcloud delete ${XBCLOUD_ARGS} --storage=azure "$BACKUP_PATH"
		else
			is_deleted_full=1
		fi

		if [[ ${is_deleted_full} == 1 && ${is_deleted_info} == 1 ]]; then
			log 'INFO' "Object deleted successfully before attempt $i. Exiting."
			break
		fi
		((time *= 2))
	done
}
