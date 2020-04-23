#!/bin/bash

set -o errexit
tmp_dir=$(mktemp -d)
ctrl=""
ACCESS_KEY_ID=${ACCESS_KEY_ID:-}
SECRET_ACCESS_KEY=${SECRET_ACCESS_KEY:-}
ENDPOINT=${ENDPOINT:-}
DEFAULT_REGION=${DEFAULT_REGION:-us-east-1}

check_ctrl() {
    if [[ -x "$(command -v kubectl)" ]]; then
        ctrl="kubectl"
    elif [[ -x "$(command -v oc)" ]]; then
        ctrl="oc"
    else
        echo "[ERROR] Neither <oc> nor <kubectl> client found"
        exit 1
    fi 
}

usage() {
    cat - <<-EOF
		usage: $0 <backup-name> <cluster-name>

		OPTIONS:
		    <backup-name>  the backup name
		                   it can be obtained with the "$ctrl get pxc-backup" command
		    <cluster-name> the name of an existing Percona XtraDB Cluster
		                   it can be obtained with the "$ctrl get pxc" command
	EOF
    exit 1
}

get_backup_dest() {
    local backup=$1

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

        local secret=$($ctrl get "pxc-backup/$backup" -o 'jsonpath={.status.s3.credentialsSecret}' 2>/dev/null)
        export ENDPOINT=$($ctrl get "pxc-backup/$backup" -o'jsonpath={.status.s3.endpointUrl}' 2>/dev/null)
        export ACCESS_KEY_ID=$($ctrl get "secret/$secret" -o 'jsonpath={.data.AWS_ACCESS_KEY_ID}' 2>/dev/null | eval ${BASE64_DECODE_CMD})
        export SECRET_ACCESS_KEY=$($ctrl get "secret/$secret" -o 'jsonpath={.data.AWS_SECRET_ACCESS_KEY}' 2>/dev/null | eval ${BASE64_DECODE_CMD})

        $ctrl get "pxc-backup/$backup" -o jsonpath='{.status.destination}'
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
    local backup_dest=$1
    local cluster=$2

    echo
    if [[ -z "$backup_dest" ]] || [[ -z "$cluster" ]]; then
        usage
    fi

    if ! $ctrl get "pxc/$cluster" 1>/dev/null; then
        printf "[ERROR] '%s' Percona XtraDB Cluster doesn't exists.\n\n" "$cluster"
        usage
    fi

    if [[ "${backup_dest:0:4}" = "pvc/" ]]; then
        if ! $ctrl get "$backup_dest" 1>/dev/null; then
            printf "[ERROR] '%s' PVC doesn't exists.\n\n" "$backup_dest"
            usage
        fi
    elif [[ "${backup_dest:0:5}" = "s3://" ]]; then
        echo [INFO] please check file: aws s3 ls --endpoint-url "${ENDPOINT:-https://s3.amazonaws.com}" "$backup_dest"
    else
        usage
    fi

    echo
    echo "All data in '$cluster' Percona XtraDB Cluster will be deleted during the backup restoration."
    while read -r -p "Are you sure? [y/N] " answer; do
        case "$answer" in
            [Yy]|[Yy][Ee][Ss]) break;;
            ""|[Nn]|[Nn][Oo])  exit 0;;
        esac
    done
}

stop_pxc() {
    local cluster=$1
    local size
    size=$($ctrl get "pxc/$cluster" -o jsonpath='{.spec.pxc.size}')

    $ctrl get "pxc/$cluster" -o yaml > "$tmp_dir/cluster.yaml"
    $ctrl delete -f "$tmp_dir/cluster.yaml"

    for i in $(seq 0 "$((size-1))" | sort -r); do
        echo -n "Deleting $cluster-pxc-$i."
        until ($ctrl get "pod/$cluster-pxc-$i" || :) 2>&1 | grep -q NotFound; do
            sleep 1
            echo -n .
        done
        echo "[done]"
    done

    if [[ "$size" -gt 1 ]]; then
        for i in $(seq 1 "$((size-1))" | sort -r); do
            $ctrl delete "pvc/datadir-$cluster-pxc-$i"
        done
    fi
}

start_pxc() {
    $ctrl apply -f "$tmp_dir/cluster.yaml"
}

recover_pvc() {
    local backup_pvc=$1
    local cluster=$2

    $ctrl delete "job/restore-job-$cluster" 2>/dev/null || :
    $ctrl delete "svc/restore-src-$cluster" 2>/dev/null || :
    $ctrl delete "pod/restore-src-$cluster" 2>/dev/null || :
    cat - <<-EOF | $ctrl apply -f -
		apiVersion: v1
		kind: Service
		metadata:
		  name: restore-src-$cluster
		spec:
		  selector:
		    name: restore-src-$cluster
		  type: ClusterIP
		  ports:
		  - name: ncat
		    port: 3307
		---
		apiVersion: v1
		kind: Pod
		metadata:
		  name: restore-src-$cluster
		  labels:
		    name: restore-src-$cluster
		spec:
		  containers:
		  - name: ncat
		    image: percona/percona-xtradb-cluster-operator:0.3.0-backup
		    imagePullPolicy: Always
		    command:
		        - bash
		        - "-exc"
		        - "cat /backup/xtrabackup.stream | ncat -l --send-only 3307"
		    volumeMounts:
		    - name: backup
		      mountPath: /backup
		  restartPolicy: Always
		  volumes:
		  - name: backup
		    persistentVolumeClaim:
		      claimName: ${backup_pvc#pvc/}
	EOF
    cat - <<-EOF | $ctrl apply -f -
		apiVersion: batch/v1
		kind: Job
		metadata:
		  name: restore-job-$cluster
		spec:
		  template:
		    spec:
		      containers:
		      - name: xtrabackup
		        image: percona/percona-xtradb-cluster-operator:0.3.0-backup
		        imagePullPolicy: Always
		        command:
		        - bash
		        - "-exc"
		        - |
		          ping -c1 restore-src-$cluster || :
		          rm -rf /datadir/*
		          ncat restore-src-$cluster 3307 | xbstream -x -C /datadir
		          xtrabackup --prepare --target-dir=/datadir
		        volumeMounts:
		        - name: datadir
		          mountPath: /datadir
		      restartPolicy: Never
		      volumes:
		      - name: datadir
		        persistentVolumeClaim:
		          claimName: datadir-$cluster-pxc-0
		  backoffLimit: 4
	EOF

    echo -n "Recovering."
    until $ctrl get "job/restore-job-$cluster" -o jsonpath='{.status.completionTime}' 2>/dev/null | grep -q 'T'; do
        sleep 1
        echo -n .
    done
    echo "[done]"
    $ctrl delete "svc/restore-src-$cluster" 2>/dev/null || :
    $ctrl delete "pod/restore-src-$cluster" 2>/dev/null || :
}

recover_s3() {
    local backup_path=$1
    local cluster=$2
    local backup_bucket=$( echo "${backup_path#s3://}" | cut -d '/' -f 1)
    local backup_key=$( echo "${backup_path#s3://}" | cut -d '/' -f 2-)

    $ctrl delete "job/restore-job-$cluster" 2>/dev/null || :
    cat - <<-EOF | $ctrl apply -f -
		apiVersion: batch/v1
		kind: Job
		metadata:
		  name: restore-job-$cluster
		spec:
		  template:
		    spec:
		      containers:
		      - name: xtrabackup
		        image: percona/percona-xtradb-cluster-operator:0.3.0-backup
		        imagePullPolicy: Always
		        command:
		        - bash
		        - "-exc"
		        - |
		          mc -C /tmp/mc config host add dest "${ENDPOINT:-https://s3.amazonaws.com}" "$ACCESS_KEY_ID" "$SECRET_ACCESS_KEY"
		          mc -C /tmp/mc ls dest/$backup_bucket/$backup_key
		          rm -rf /datadir/*
		          mc -C /tmp/mc cat dest/$backup_bucket/$backup_key | xbstream -x -C /datadir
		          xtrabackup --prepare --target-dir=/datadir
		        volumeMounts:
		        - name: datadir
		          mountPath: /datadir
		      restartPolicy: Never
		      volumes:
		      - name: datadir
		        persistentVolumeClaim:
		          claimName: datadir-$cluster-pxc-0
		  backoffLimit: 4
	EOF

    echo -n "Recovering."
    until $ctrl get "job/restore-job-$cluster" -o jsonpath='{.status.completionTime}' 2>/dev/null | grep -q 'T'; do
        sleep 1
        echo -n .
    done
    echo "[done]"
    $ctrl delete "svc/restore-src-$cluster" 2>/dev/null || :
    $ctrl delete "pod/restore-src-$cluster" 2>/dev/null || :
}

main() {
    local backup=$1
    local cluster=$2
    local backup_dest

    check_ctrl
    enable_logging
    get_backup_dest "$backup"
    backup_dest=$(get_backup_dest "$backup")
    check_input "$backup_dest" "$cluster"

    stop_pxc "$cluster"
    if [[ "${backup_dest:0:4}" = "pvc/" ]]; then
        recover_pvc "$backup_dest" "$cluster"
    elif [[ "${backup_dest:0:5}" = "s3://" ]]; then
        recover_s3 "$backup_dest" "$cluster"
    fi
    start_pxc

    cat - <<-EOF

		You can view xtrabackup log:
		    $ $ctrl logs job/restore-job-$cluster
		If everything is fine, you can cleanup the job:
		    $ $ctrl delete job/restore-job-$cluster

	EOF
}

main "$@"
exit 0
