#!/bin/bash

set -o errexit
tmp_dir=$(mktemp -d)
ctrl=""

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
	EOF
    exit 1
}

get_backup_pvc() {
    local backup=$1

    if $ctrl get "pxc-backup/$backup" 1>/dev/null 2>/dev/null; then
        $ctrl get "pxc-backup/$backup" -o jsonpath='{.status.volume}'
    else
        # support direct PVC name here
        echo -n "$backup"
    fi
}

enable_logging() {
    BASH_VER=$(echo "$BASH_VERSION" | cut -d . -f 1,2)
    if (( $(echo "$BASH_VER >= 4.1" |bc -l) )); then
        exec 5>"$tmp_dir/log"
        BASH_XTRACEFD=5
        set -o xtrace
        echo "Log: $tmp_dir/log"
    fi
}

check_input() {
    local backup_pvc=$1
    local dest_dir=$2

    if [ -z "$backup_pvc" ] || [ -z "$dest_dir" ]; then
        usage
    fi

    if [ ! -e "$dest_dir" ]; then
        mkdir -p "$dest_dir"
    fi

    if ! $ctrl get "pvc/$backup_pvc" 1>/dev/null; then
        printf "[ERROR] '%s' PVC doesn't exists.\n\n" "$backup_pvc"
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
		          claimName: $backup_pvc
	EOF

    echo -n Starting pod.
    until $ctrl get pod/backup-access -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null | grep -q 'true'; do
        sleep 1
        echo -n .
    done
    echo "[done]"
}

copy_files() {
    local dest_dir=$1

    echo "Downloading started"
    $ctrl cp backup-access:/backup/ "${dest_dir%/}/"
    echo "Downloading finished"
}

stop_tmp_pod() {
    $ctrl delete pod/backup-access
}

check_md5() {
    local dest_dir=$1

    cd "${dest_dir}"
        md5sum -c md5sum.txt
    cd -
}

main() {
    local backup=$1
    local dest_dir=$2
    local backup_pvc

    check_ctrl
    enable_logging
    backup_pvc=$(get_backup_pvc "$backup")
    check_input "$backup_pvc" "$dest_dir"

    start_tmp_pod "$backup_pvc"
    copy_files "$dest_dir"
    stop_tmp_pod
    check_md5 "$dest_dir"

    cat - <<-EOF

		You can recover data locally with following commands:
		    $ service mysqld stop
		    $ rm -rf /var/lib/mysql/*
		    $ cat xtrabackup.stream | xbstream -x -C /var/lib/mysql
		    $ xtrabackup --prepare --target-dir=/var/lib/mysql
		    $ chown -R mysql:mysql /var/lib/mysql
		    $ service mysqld start

	EOF
}

main "$@"
exit 0
