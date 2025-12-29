#!/bin/bash

set -o errexit

export AWS_SHARED_CREDENTIALS_FILE='/tmp/aws-credfile'
export AWS_REGION="${DEFAULT_REGION:-us-west-2}"
export AWS_ENDPOINT_URL="${ENDPOINT:-https://s3.${AWS_REGION}.amazonaws.com}"

if [ -n "$VERIFY_TLS" ] && [[ $VERIFY_TLS == "false" ]]; then
	AWS_S3_NO_VERIFY_SSL='--no-verify-ssl'
fi

caBundleDir="/etc/s3/certs"
caBundleFile="$caBundleDir/ca.crt"
if [ -f "$caBundleFile" ]; then
	export AWS_CA_BUNDLE="$caBundleFile"
fi

is_object_exist() {
	local bucket="$1"
	local path="$2"

	# '--summarize' is included to retrieve the 'Total Objects:' count for checking object/folder existence
	# shellcheck disable=SC2086
	res=$(aws $AWS_S3_NO_VERIFY_SSL s3 ls "s3://$bucket/$path" --summarize --recursive)
	if echo "$res" | grep -q 'Total Objects: 0'; then
		return 1 # object/folder does not exist
	fi
	return 0
}

s3_add_bucket_dest() {
	{ set +x; } 2>/dev/null
	if [ -n "$ACCESS_KEY_ID" ] && [ -n "$SECRET_ACCESS_KEY" ]; then
		# Set credentials in AWS credentials file (for AWS CLI)
		aws configure set aws_access_key_id "$ACCESS_KEY_ID"
		aws configure set aws_secret_access_key "$SECRET_ACCESS_KEY"
	fi
	if [ -n "$S3_SESSION_TOKEN" ]; then
		# Set session token for AWS CLI (credentials file)
		aws configure set aws_session_token "$S3_SESSION_TOKEN"
	fi
	set -x
}
