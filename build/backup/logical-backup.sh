#!/bin/bash

# Takes a logical backup of a PXC cluster with mydumper and uploads it to S3,
# one object per mydumper file, under s3://$S3_BUCKET/$S3_BUCKET_PATH/.

set -o errexit
set -o xtrace

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/backup.sh
. ${LIB_PATH}/backup.sh
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh
# shellcheck source=build/backup/lib/pxc/backup-source.sh
. ${LIB_PATH}/backup-source.sh

UPLOAD_SCRIPT='/opt/percona/backup/logical-upload.sh'
SCRATCH_DIR=${SCRATCH_DIR:-/scratch}
DUMP_DIR="${SCRATCH_DIR}/dump"
UPLOAD_FAILED_MARKER="${SCRATCH_DIR}/upload-failed"
DEFAULTS_FILE='/tmp/mydumper.cnf'
DESTINATION="s3://${S3_BUCKET}/${S3_BUCKET_PATH}"

DEFAULT_CHUNK_FILESIZE=64
DEFAULT_EXEC_THREADS=4
# mydumper dumps the mysql and sys schemas unless they are filtered out. The
# pattern is matched against 'db.table' for tables and against 'db' alone for
# the CREATE DATABASE file, so it has to cover both forms.
DEFAULT_REGEX='^(?!(mysql|sys)(\.|$))'
# mydumper's own default lock mode leaves a PXC 8.4 node in Donor/Desynced and
# can hang it. GTID mode takes no locks.
DEFAULT_LOCK_MODE='GTID'

export SCRATCH_DIR

# has_user_arg reports whether the user-supplied mydumper arguments contain any
# of the given options.
has_user_arg() {
	local arg option
	# shellcheck disable=SC2086
	for arg in ${MYDUMPER_EXTRA_ARGS}; do
		for option in "$@"; do
			if [[ ${arg} == "${option}" || ${arg} == "${option}="* ]]; then
				return 0
			fi
		done
	done
	return 1
}

write_defaults_file() {
	{ set +x; } 2>/dev/null
	(
		umask 077
		cat >"${DEFAULTS_FILE}" <<-EOF
			[mydumper]
			user=xtrabackup
			password=${PXC_PASS}
		EOF
	)
	set -x
}

clean_destination() {
	# shellcheck disable=SC2086
	aws $AWS_S3_NO_VERIFY_SSL s3 rm --only-show-errors --recursive "${DESTINATION}/"
	rm -rf "${DUMP_DIR}" "${UPLOAD_FAILED_MARKER}"
	mkdir -p "${DUMP_DIR}"
}

run_mydumper() {
	local node=$1
	local args=(
		"--defaults-extra-file=${DEFAULTS_FILE}"
		"--host=${node}.${PXC_SERVICE}"
		"--outputdir=${DUMP_DIR}"
		"--chunk-filesize=${DEFAULT_CHUNK_FILESIZE}"
		--triggers
		--routines
		--events
		"--exec=${UPLOAD_SCRIPT} FILENAME"
		"--exec-threads=${DEFAULT_EXEC_THREADS}"
	)

	# A repeated --regex does not replace the first one, so the defaults are only
	# added when the user did not set these options.
	if ! has_user_arg --regex -x; then
		args+=("--regex=${DEFAULT_REGEX}")
	fi
	if ! has_user_arg --sync-thread-lock-mode; then
		args+=("--sync-thread-lock-mode=${DEFAULT_LOCK_MODE}")
	fi

	# shellcheck disable=SC2086
	mydumper "${args[@]}" ${MYDUMPER_EXTRA_ARGS}
}

# upload_remaining_files uploads what mydumper did not pass to --exec. That is
# the metadata file, which goes last because its presence marks the backup as
# complete.
upload_remaining_files() {
	local file
	for file in "${DUMP_DIR}"/*; do
		if [[ ! -f ${file} || $(basename "${file}") == 'metadata' ]]; then
			continue
		fi
		"${UPLOAD_SCRIPT}" "${file}" || return 1
	done
	if [[ -f "${DUMP_DIR}/metadata" ]]; then
		"${UPLOAD_SCRIPT}" "${DUMP_DIR}/metadata" || return 1
	fi
}

if ! command -v mydumper >/dev/null; then
	log 'ERROR' 'mydumper not found in image'
	exit 1
fi

if [[ ${S3_FORCE_PATH} == 'true' ]]; then
	aws configure set default.s3.addressing_style path
fi
s3_add_bucket_dest

NODE_NAME=$(get_backup_source)
if [ -z "${NODE_NAME}" ]; then
	/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="${PXC_SERVICE}"
	log 'ERROR' 'Cannot find node for backup'
	log 'ERROR' 'Backup was finished unsuccessful'
	exit 1
fi

clean_destination
write_defaults_file

log 'INFO' "Logical backup of ${NODE_NAME} to ${DESTINATION} was started"

set +o errexit
run_mydumper "${NODE_NAME}"
mydumper_status=$?
set -o errexit

if [[ ${mydumper_status} -ne 0 ]]; then
	log 'ERROR' "mydumper exited with ${mydumper_status}"
	log 'ERROR' 'Backup was finished unsuccessful'
	exit 1
fi

if ! upload_remaining_files || [[ -f ${UPLOAD_FAILED_MARKER} ]]; then
	log 'ERROR' 'Some backup files were not uploaded'
	log 'ERROR' 'Backup was finished unsuccessful'
	exit 1
fi

if ! is_object_exist "${S3_BUCKET}" "${S3_BUCKET_PATH}/metadata"; then
	log 'ERROR' "metadata is missing in ${DESTINATION}"
	log 'ERROR' 'Backup was finished unsuccessful'
	exit 1
fi

log 'INFO' 'Backup was finished successfully'
