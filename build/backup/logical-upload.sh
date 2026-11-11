#!/bin/bash

# Uploads one mydumper output file to S3.
#
# mydumper runs this script through --exec once for every file it has finished
# writing. mydumper deletes the local file as soon as this script returns, even
# when it fails, and still exits 0 itself. A failed upload is therefore
# recorded in a marker file that logical-backup.sh checks after the dump.

set -o errexit

LIB_PATH='/opt/percona/backup/lib/pxc'
# shellcheck source=build/backup/lib/pxc/aws.sh
. ${LIB_PATH}/aws.sh

UPLOAD_ATTEMPTS=3

file="$1"
destination="s3://${S3_BUCKET}/${S3_BUCKET_PATH}/$(basename "${file}")"

for attempt in $(seq 1 "${UPLOAD_ATTEMPTS}"); do
	# shellcheck disable=SC2086
	if aws $AWS_S3_NO_VERIFY_SSL s3 cp --only-show-errors "${file}" "${destination}"; then
		exit 0
	fi
	echo "upload of ${file} to ${destination} failed (attempt ${attempt} of ${UPLOAD_ATTEMPTS})" >&2
	sleep $((attempt * 5))
done

touch "${SCRATCH_DIR}/upload-failed"
exit 1
