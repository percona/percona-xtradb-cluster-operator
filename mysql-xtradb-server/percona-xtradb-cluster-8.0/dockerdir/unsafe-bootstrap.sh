#!/bin/bash

set -o errexit
set -o xtrace

function get_synced_count() {
    peer-list -on-start=/usr/bin/get-pxc-state -service="$PXC_SERVICE" 2>&1 \
        | grep -c wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary
}

GRA=/var/lib/mysql/grastate.dat
if hostname -s | grep -- '-pxc-0$'; then
    if grep 'safe_to_bootstrap: 0' "${GRA}"; then
        if [[ $(get_synced_count) = 0 ]]; then
            mysqld --wsrep_recover
            sed "s^safe_to_bootstrap: 0^safe_to_bootstrap: 1^" ${GRA} 1<> ${GRA}
        fi
    fi
fi
