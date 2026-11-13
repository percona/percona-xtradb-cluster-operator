#!/bin/bash

# Restores a logical backup into a running PXC cluster: downloads the mydumper
# files from s3://$S3_BUCKET_URL/ to the scratch volume and loads them with
# myloader through $PXC_HOST.

set -o errexit
set -o xtrace

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/backup.sh
. ${LIB_PATH}/backup.sh
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh
# shellcheck source=build/backup/lib/pxc/logical.sh
. ${LIB_PATH}/logical.sh

SCRATCH_DIR=${SCRATCH_DIR:-/scratch}
DUMP_DIR="${SCRATCH_DIR}/dump"
SOURCE="s3://${S3_BUCKET_URL}"

download_dump() {
	rm -rf "${DUMP_DIR}"
	mkdir -p "${DUMP_DIR}"
	# shellcheck disable=SC2086
	aws $AWS_S3_NO_VERIFY_SSL s3 cp --only-show-errors --recursive "${SOURCE}/" "${DUMP_DIR}/"
}

# add_view_placeholder_keys gives the placeholder table of every view a
# primary key.
#
# For each view mydumper writes <db>.<view>-schema.sql, a stand-in table that
# lets views be created in any order, and <db>.<view>-schema-view.sql, which
# drops the stand-in and creates the real view. The stand-in has no primary
# key, and PXC with pxc_strict_mode=ENFORCING refuses to create such a table,
# which makes myloader abort after it has already dropped the existing tables.
add_view_placeholder_keys() {
	local view_file placeholder
	for view_file in "${DUMP_DIR}"/*-schema-view.sql*; do
		if [[ ! -f ${view_file} ]]; then
			continue
		fi
		placeholder="${view_file%-schema-view.sql*}-schema.sql"

		# a compressed placeholder has to be unpacked before it can be edited
		if [[ -f "${placeholder}.zst" ]]; then
			zstd --decompress --quiet --rm "${placeholder}.zst"
		elif [[ -f "${placeholder}.gz" ]]; then
			gzip --decompress "${placeholder}.gz"
		fi
		if [[ ! -f ${placeholder} ]]; then
			continue
		fi

		# shellcheck disable=SC2016
		sed -i 's/^CREATE TABLE IF NOT EXISTS \(.*\)($/&\n`__pk` int NOT NULL PRIMARY KEY,/' "${placeholder}"
	done
}

run_myloader() {
	local args=(
		--user=xtrabackup
		"--host=${PXC_HOST}"
		"--directory=${DUMP_DIR}"
	)

	# Existing tables are dropped and recreated, unless the user chose another mode.
	if ! has_arg "${MYLOADER_EXTRA_ARGS}" --drop-table -o; then
		args+=(--drop-table)
	fi

	# shellcheck disable=SC2086
	myloader "${args[@]}" ${MYLOADER_EXTRA_ARGS}
}

if ! command -v myloader >/dev/null; then
	log 'ERROR' 'myloader not found in image'
	exit 1
fi

use_s3_path_style
s3_add_bucket_dest

log 'INFO' "Logical restore from ${SOURCE} to ${PXC_HOST} was started"

download_dump

if [[ ! -f "${DUMP_DIR}/metadata" ]]; then
	log 'ERROR' "metadata is missing in ${SOURCE}, the backup is incomplete"
	log 'ERROR' 'Restore was finished unsuccessful'
	exit 1
fi

add_view_placeholder_keys
export_mysql_password

set +o errexit
run_myloader
myloader_status=$?
set -o errexit

if [[ ${myloader_status} -ne 0 ]]; then
	log 'ERROR' "myloader exited with ${myloader_status}"
	log 'ERROR' 'Restore was finished unsuccessful'
	exit 1
fi

log 'INFO' 'Restore was finished successfully'
