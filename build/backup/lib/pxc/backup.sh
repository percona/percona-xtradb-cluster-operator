#!/bin/bash

set -o errexit

LIB_PATH='/opt/percona/backup/lib/pxc'
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
	local now=$(date '+%F %H:%M:%S')

	echo "${now} [${level}] ${message}"
	set -x
}

clean_backup_s3() {
	s3_add_bucket_dest

	local time=15
	local is_deleted_full=0
	local is_deleted_info=0
	local exit_code=0

	for i in {1..5}; do
		if ((i > 1)); then
			log 'INFO' "Sleeping ${time}s before retry $i..."
			sleep "$time"
		fi

		if is_object_exist "$S3_BUCKET" "$S3_BUCKET_PATH/"; then
			log 'INFO' "Delete (attempt $i)..."

			xbcloud delete ${XBCLOUD_ARGS} --storage=s3 --s3-bucket="$S3_BUCKET" "$S3_BUCKET_PATH"
		else
			is_deleted_full=1
		fi

		if is_object_exist "$S3_BUCKET" "$S3_BUCKET_PATH.$SST_INFO_NAME/"; then
			log 'INFO' "Delete (attempt $i)..."

			xbcloud delete ${XBCLOUD_ARGS} --storage=s3 --s3-bucket="$S3_BUCKET" "$S3_BUCKET_PATH.$SST_INFO_NAME"
		else
			is_deleted_info=1
		fi

		if [[ ${is_deleted_full} == 1 && ${is_deleted_info} == 1 ]]; then
			log 'INFO' "Object deleted successfully before attempt $i. Exiting."
			break
		fi
		let time*=2
	done
}

azure_auth_header_file() {
	local params="$1"
	local request_date="$2"
	local hex_tmp
	local signature_tmp
	local auth_header_tmp
	local resource
	local string_to_sign
	local decoded_key

	hex_tmp=$(mktemp)
	signature_tmp=$(mktemp)
	auth_header_tmp=$(mktemp)

	decoded_key=$(echo -n "$AZURE_ACCESS_KEY" | base64 -d | hexdump -ve '1/1 "%02x"')
	echo -n "$decoded_key" >"$hex_tmp"

	resource="/$AZURE_STORAGE_ACCOUNT/$AZURE_CONTAINER_NAME"

	string_to_sign=$(printf "GET\n\n\n\n\n\n\n\n\n\n\n\nx-ms-date:%s\nx-ms-version:2021-06-08\n%s\n%s" \
		"$request_date" \
		"$resource" \
		"$params")

	printf "%s" "$string_to_sign" | openssl dgst -sha256 -mac HMAC -macopt "hexkey:$(cat "$hex_tmp")" -binary | base64 >"$signature_tmp"

	echo -n "Authorization: SharedKey $AZURE_STORAGE_ACCOUNT:$(cat "$signature_tmp")" >"$auth_header_tmp"

	echo "$auth_header_tmp"
}

is_object_exist_azure() {
	object="$1"
	{ set +x; } 2>/dev/null
	connection_string="$ENDPOINT/$AZURE_CONTAINER_NAME?comp=list&restype=container&prefix=$object"
	request_date=$(LC_ALL=en_US.utf8 TZ=GMT date "+%a, %d %h %Y %H:%M:%S %Z")
	header_version="x-ms-version: 2021-06-08"
	header_date="x-ms-date: $request_date"
	header_auth_file=$(azure_auth_header_file "$(printf 'comp:list\nprefix:%s\nrestype:container' "$object")" "$request_date")

	response=$(curl -s -H "$header_version" -H "$header_date" -H "@$header_auth_file" "${connection_string}")
	res=$(echo "$response" | grep "<Blob>")
	set -x

	if [[ ${#res} -ne 0 ]]; then
		return 0
	fi
	return 1
}

clean_backup_azure() {
	ENDPOINT=${AZURE_ENDPOINT:-"https://$AZURE_STORAGE_ACCOUNT.blob.core.windows.net"}

	local time=15
	local is_deleted_full=0
	local is_deleted_info=0
	local exit_code=0

	for i in {1..5}; do
		if ((i > 1)); then
			log 'INFO' "Sleeping ${time}s before retry $i..."
			sleep "$time"
		fi

		if is_object_exist_azure "$BACKUP_PATH.$SST_INFO_NAME/"; then
			log 'INFO' "Delete (attempt $i)..."
			xbcloud delete ${XBCLOUD_ARGS} --storage=azure "$BACKUP_PATH.$SST_INFO_NAME"
		else
			is_deleted_info=1
		fi

		if is_object_exist_azure "$BACKUP_PATH/"; then
			log 'INFO' "Delete (attempt $i)..."
			xbcloud delete ${XBCLOUD_ARGS} --storage=azure "$BACKUP_PATH"
		else
			is_deleted_full=1
		fi

		if [[ ${is_deleted_full} == 1 && ${is_deleted_info} == 1 ]]; then
			log 'INFO' "Object deleted successfully before attempt $i. Exiting."
			break
		fi
		let time*=2
	done
}
