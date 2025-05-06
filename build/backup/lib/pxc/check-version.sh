#!/bin/bash

normalize_version() {
	local major=0
	local minor=0
	local patch=0

	# Only parses purely numeric version numbers, 1.2.3
	# Everything after the first three values are ignored
	if [[ $1 =~ ^([0-9]+)\.([0-9]+)\.?([0-9]*)([^ ])* ]]; then
		major=${BASH_REMATCH[1]}
		minor=${BASH_REMATCH[2]}
		patch=${BASH_REMATCH[3]}
	fi

	printf %02d%02d%02d "$major" "$minor" "$patch"
}

check_for_version() {
	if [ -z "$1" ]; then
		echo "Error: local version is empty"
		exit 1
	fi
	if [ -z "$2" ]; then
		echo "Error: required version is empty"
		exit 1
	fi
	local local_version_str
	local required_version_str
	local_version_str="$(normalize_version "$1")"
	required_version_str="$(normalize_version "$2")"

	if [[ $local_version_str < $required_version_str ]]; then
		return 1
	else
		return 0
	fi
}
