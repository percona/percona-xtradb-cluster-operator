#!/bin/bash

set -o errexit
tmp_dir=$(mktemp -d)
ctrl=""
ACCESS_KEY_ID=${ACCESS_KEY_ID:-}
SECRET_ACCESS_KEY=${SECRET_ACCESS_KEY:-}
ENDPOINT=${ENDPOINT:-}
DEFAULT_REGION=${DEFAULT_REGION:-us-east-1}

# it is needed to have full path to xbcloud on some platforms
if ! xbcloud=$(which xbcloud); then
	echo "No xtrabackup binaries found, please install them:"
	echo "https://www.percona.com/downloads/Percona-XtraBackup-LATEST"
	echo "https://formulae.brew.sh/formula/percona-xtrabackup"
	exit 1
fi

check_ctrl() {
	if [ -x "$(command -v kubectl)" ]; then
		ctrl="kubectl"
	elif [ -x "$(command -v oc)" ]; then
		ctrl="oc"
	else
		echo "[ERROR] Neither <oc> nor <kubectl> client found"
		exit 1
	fi
}

usage() {
	cat - <<-EOF
		usage: $0 <backup-name> <local/dir>

		OPTIONS:
		    <backup-name>  the backup name
		                   it can be obtained with the "$ctrl get pxc-backup" command
		    <local/dir>    the name of destination directory on local machine
		    <namespace>    optionally specify a namespace
	EOF
	exit 1
}

get_backup_dest() {
	local backup=$1
	local secret

	if $ctrl get "pxc-backup/$backup" 1>/dev/null 2>/dev/null; then
		BASE64_DECODE_CMD=""
		if echo eWVz | base64 -d 1>/dev/null 2>/dev/null; then
			BASE64_DECODE_CMD="base64 -d"
		elif echo eWVz | base64 -D 1>/dev/null 2>/dev/null; then
			BASE64_DECODE_CMD="base64 -D"
		else
			echo "base64 decode error."
			exit 1
		fi

		secret=$($ctrl get "pxc-backup/$backup" -o 'jsonpath={.status.s3.credentialsSecret}' 2>/dev/null)
		if [ -n "$secret" ]; then
			ENDPOINT=$($ctrl get "pxc-backup/$backup" -o 'jsonpath={.status.s3.endpointUrl}' 2>/dev/null)
			ACCESS_KEY_ID=$($ctrl get "secret/$secret" -o 'jsonpath={.data.AWS_ACCESS_KEY_ID}' 2>/dev/null | eval "${BASE64_DECODE_CMD}")
			SECRET_ACCESS_KEY=$($ctrl get "secret/$secret" -o 'jsonpath={.data.AWS_SECRET_ACCESS_KEY}' 2>/dev/null | eval "${BASE64_DECODE_CMD}")
			export CREDENTIALS="ENDPOINT=$ENDPOINT ACCESS_KEY_ID=$ACCESS_KEY_ID SECRET_ACCESS_KEY=$SECRET_ACCESS_KEY DEFAULT_REGION=$DEFAULT_REGION"

			$ctrl get "pxc-backup/$backup" -o jsonpath='{.status.destination}'
			return
		fi

		secret=$($ctrl get "pxc-backup/$backup" -o 'jsonpath={.status.azure.credentialsSecret}' 2>/dev/null)
		if [ -n "$secret" ]; then
			AZURE_STORAGE_ACCOUNT=$($ctrl get "secret/$secret" -o 'jsonpath={.data.AZURE_STORAGE_ACCOUNT_NAME}' 2>/dev/null | eval "${BASE64_DECODE_CMD}")
			AZURE_ACCESS_KEY=$($ctrl get "secret/$secret" -o 'jsonpath={.data.AZURE_STORAGE_ACCOUNT_KEY}' 2>/dev/null | eval "${BASE64_DECODE_CMD}")
			AZURE_STORAGE_CLASS=$($ctrl get "pxc-backup/$backup" -o 'jsonpath={.data.storageClass}' 2>/dev/null | eval "${BASE64_DECODE_CMD}")
			export CREDENTIALS="AZURE_STORAGE_ACCOUNT=$AZURE_STORAGE_ACCOUNT AZURE_ACCESS_KEY=$AZURE_ACCESS_KEY AZURE_STORAGE_CLASS=$AZURE_STORAGE_CLASS"
			echo "azure://$($ctrl get "pxc-backup/$backup" -o jsonpath='{.status.destination}')"
			return
		fi

		$ctrl get "pxc-backup/$backup" -o jsonpath='{.status.destination}'
	else
		# support direct PVC name here
		echo -n "$backup"
	fi
}

enable_logging() {
	BASH_VER=$(echo "$BASH_VERSION" | cut -d . -f 1,2)
	if (($(echo "$BASH_VER >= 4.1" | bc -l))); then
		exec 5>"$tmp_dir/log"
		BASH_XTRACEFD=5
		set -o xtrace
		echo "Log: $tmp_dir/log"
	fi
}

check_input_namespace() {
	local namespace=${1}

	if [ -n "$namespace" ]; then
		ctrl="$ctrl -n $namespace"
	fi
}

check_input_destination() {
	local backup_dest=$1
	local dest_dir=$2

	if [ -z "$backup_dest" ] || [ -z "$dest_dir" ]; then
		usage
	fi

	if [ ! -e "$dest_dir" ]; then
		mkdir -p "$dest_dir"
	fi

	if [ "${backup_dest:0:4}" = "pvc/" ]; then
		if ! $ctrl get "$backup_dest" 1>/dev/null; then
			printf "[ERROR] '%s' PVC doesn't exists.\n\n" "$backup_dest"
			usage
		fi
	elif [ "${backup_dest:0:5}" = "s3://" ] || [ "${backup_dest:0:8}" = "azure://" ]; then
		env -i "${CREDENTIALS} ${xbcloud} get ${backup_dest} xtrabackup_info" 1>/dev/null
	else
		echo "Can't find $backup_dest backup"
		usage
	fi

	if [ ! -d "$dest_dir" ]; then
		printf "[ERROR] '%s' is not local directory.\n\n" "$dest_dir"
		usage
	fi

}

start_tmp_pod() {
	local backup_pvc=$1

	$ctrl delete pod/backup-access 2>/dev/null || :

	cat - <<-EOF | $ctrl apply -f -
		apiVersion: v1
		kind: Pod
		metadata:
		  name: backup-access
		spec:
		  containers:
		  - name: xtrabackup
		    image: percona/percona-xtradb-cluster-operator:0.3.0-backup
		    volumeMounts:
		    - name: backup
		      mountPath: /backup
		  restartPolicy: Never
		  volumes:
		  - name: backup
		    persistentVolumeClaim:
		      claimName: ${backup_pvc#pvc/}
	EOF

	echo -n Starting pod.
	until $ctrl get pod/backup-access -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null | grep -q 'true'; do
		sleep 1
		echo -n .
	done
	echo "[done]"
}

copy_files_pvc() {
	local dest_dir=$1
	local real_dest_dir
	real_dest_dir=$(
		cd "$dest_dir"
		pwd -P
	)

	echo ""
	echo "Downloading started"
	$ctrl cp backup-access:/backup/ "${real_dest_dir%/}/"
	echo "Downloading finished"
}

copy_files_xbcloud() {
	local backup_path=$1
	local dest_dir=$2

	echo ""
	echo "Downloading started"
	env -i "${CREDENTIALS} ${xbcloud} get ${backup_path} --parallel=10" 1>"$dest_dir/xtrabackup.stream" 2>"$dest_dir/transfer.log"
	echo "Downloading finished"
}

stop_tmp_pod() {
	$ctrl delete pod/backup-access
}

main() {
	local backup=$1
	local dest_dir=$2
	local namespace=$3
	local backup_dest

	check_ctrl
	enable_logging
	check_input_namespace "$namespace"
	get_backup_dest "$backup"
	backup_dest=$(get_backup_dest "$backup")
	check_input_destination "$backup_dest" "$dest_dir"

	if [ "${backup_dest:0:4}" = "pvc/" ]; then
		start_tmp_pod "$backup_dest"
		copy_files_pvc "$dest_dir"
		stop_tmp_pod
	elif [ "${backup_dest:0:5}" = "s3://" ] || [ "${backup_dest:0:8}" = "azure://" ]; then
		copy_files_xbcloud "$backup_dest" "$dest_dir"
	fi

	cat - <<-EOF

		You can recover data locally with following commands:
		    $ service mysqld stop
		    $ rm -rf /var/lib/mysql/*
		    $ cat $dest_dir/xtrabackup.stream | xbstream --decompress -x -C /var/lib/mysql
		    $ xtrabackup --prepare --target-dir=/var/lib/mysql
		    $ chown -R mysql:mysql /var/lib/mysql
		    $ service mysqld start

	EOF
}

main "$@"
exit 0
