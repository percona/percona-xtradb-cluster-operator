#!/bin/bash

# get_backup_source prints the name of the PXC pod a backup should be taken from:
# a synced node of the primary component, avoiding the first node when the
# cluster has more than one. It prints nothing when no node qualifies.
function get_backup_source() {
	CLUSTER_SIZE=$(/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="$PXC_SERVICE" 2>&1 \
		| grep wsrep_cluster_size \
		| sort \
		| tail -1 \
		| cut -d : -f 12)

	if [ -z "${CLUSTER_SIZE}" ]; then
		exit 1
	fi

	FIRST_NODE=$(/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="$PXC_SERVICE" 2>&1 \
		| grep wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary \
		| sort -r \
		| tail -1 \
		| cut -d : -f 2 \
		| cut -d . -f 1)

	SKIP_FIRST_POD='|'
	if ((${CLUSTER_SIZE:-0} > 1)); then
		SKIP_FIRST_POD="$FIRST_NODE"
	fi
	/opt/percona/peer-list -on-start=/opt/percona/backup/lib/pxc/get-pxc-state.sh -service="$PXC_SERVICE" 2>&1 \
		| grep wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary \
		| grep -v "$SKIP_FIRST_POD" \
		| sort \
		| tail -1 \
		| cut -d : -f 2 \
		| cut -d . -f 1
}
