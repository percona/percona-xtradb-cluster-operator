#!/bin/bash

delete_backup_pod() {
	local backup_name=$1

	desc "Delete ${backup_name} pod during SST"
	echo "Waiting for ${backup_name} pod to become Running"
	sleep 1
	kubectl_bin wait --for=jsonpath='{.status.phase}'=Running pod --selector=percona.com/backup-job-name=xb-${backup_name} --timeout=120s

	backup_pod=$(kubectl_bin get pods --selector=percona.com/backup-job-name=xb-${backup_name} -o jsonpath='{.items[].metadata.name}')

	echo "Deleting pod/${backup_pod} during SST upload"
	kubectl logs -f ${backup_pod} | while IFS= read -r line; do
		if [[ $line =~ \.ibd\. ]]; then
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
	local backup_pod=$(kubectl_bin get pods --selector=percona.com/backup-job-name=xb-${backup_name} -o jsonpath='{.items[].metadata.name}')
	if [[ $IMAGE_PXC =~ 5\.7 ]]; then
		# There are 2 deletes during backup: $backup_dir_sst_info & $backup_dir
		deletes_num=$(kubectl_bin logs ${backup_pod} | grep -c 'Delete completed.')
		if [[ ${deletes_num} -ge '2' ]]; then
			echo "Bucket cleanup was successful"
		else
			echo "Something went wrong. Delete was performed for $deletes_num. Expected: 2."
			kubectl_bin logs ${backup_pod}
			exit 1
		fi
	else
		if kubectl_bin logs ${backup_pod} | grep 'Object deleted successfully before attempt 1. Exiting.'; then
			echo "Something went wrong. Delete was not performed."
			kubectl_bin logs ${backup_pod}
			exit 1
		else
			echo "Clenup was performed."
		fi
	fi
}