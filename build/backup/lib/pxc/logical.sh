#!/bin/bash

# Helpers shared by the logical backup and restore scripts.

# has_arg reports whether a string of command-line arguments contains any of
# the given options, either on their own or as option=value.
#
# Usage: has_arg "$MYDUMPER_EXTRA_ARGS" --regex -x
has_arg() {
	local args=$1
	shift

	local arg option
	# shellcheck disable=SC2086
	for arg in ${args}; do
		for option in "$@"; do
			if [[ ${arg} == "${option}" || ${arg} == "${option}="* ]]; then
				return 0
			fi
		done
	done
	return 1
}

# use_s3_path_style makes the aws CLI address buckets as endpoint/bucket when
# the storage asks for it.
use_s3_path_style() {
	if [[ ${S3_FORCE_PATH} == 'true' ]]; then
		aws configure set default.s3.addressing_style path
	fi
}

export_mysql_password() {
	{ set +x; } 2>/dev/null
	export MYSQL_PWD="${PXC_PASS}"
	set -x
}
