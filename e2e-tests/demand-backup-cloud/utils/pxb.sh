#!/bin/bash

delete_backup_pod() {
	local backup_name=$1

	desc "Delete ${backup_name} pod during backup"
	echo "Waiting for ${backup_name} pod to become Running"
	sleep 1
	kubectl_bin wait --for=jsonpath='{.status.phase}'=Running pod --selector=percona.com/backup-job-name=xb-${backup_name} --timeout=120s

	backup_pod=$(kubectl_bin get pods --selector=percona.com/backup-job-name=xb-${backup_name} -o jsonpath='{.items[].metadata.name}')

	# sleep for 25 seconds so that an upload is started
	sleep 25

	echo "Deleting pod/${backup_pod} during backup"
	kubectl logs -f ${backup_pod} | while IFS= read -r line; do
		if [[ $line =~ 'Backup requested' ]]; then
			kubectl delete pod --force ${backup_pod}
			break
		fi
	done

}

check_cloud_storage_cleanup() {
	local backup_name=$1

	desc "Check storage cleanup of ${backup_name}"
	if [[ $(kubectl_bin get events --field-selector involvedObject.kind=Job,involvedObject.name=xb-${backup_name} | grep -c "Created pod") == '1' ]]; then
		echo "There should be 2+ pods started by job. First backup finished too quick"
		exit 1
	fi

    local cluster_name=$(kubectl_bin get pxc-backup ${backup_name} -o jsonpath='{.spec.pxcCluster}')
    if [[ -z $cluster_name ]]; then
        echo "Cluster name is not set on backup ${backup_name}"
        exit 1
    fi

    local pxc_pod="${cluster_name}-pxc-0"
    if kubectl_bin logs ${pxc_pod} -c xtrabackup | grep 'Deleting Backup'; then
        echo "Cleanup was performed."
    else
        echo "Something went wrong. Delete was not performed."
        kubectl_bin logs ${pxc_pod} -c xtrabackup
        exit 1
    fi

}