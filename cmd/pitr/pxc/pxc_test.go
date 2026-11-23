package pxc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHostSyncedPrimary(t *testing.T) {
	t.Parallel()

	const state = ":wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary"

	tests := []struct {
		name  string
		nodes []string
		host  string
		want  bool
	}{
		{
			name:  "synced and primary",
			nodes: []string{"pxc-0" + state},
			host:  "pxc-0",
			want:  true,
		},
		{
			name:  "donor",
			nodes: []string{"pxc-0:wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Donor/Desynced:wsrep_cluster_status:Primary"},
			host:  "pxc-0",
			want:  false,
		},
		{
			name:  "non-primary component",
			nodes: []string{"pxc-0:wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Non-Primary"},
			host:  "pxc-0",
			want:  false,
		},
		{
			name:  "not ready",
			nodes: []string{"pxc-0:wsrep_ready:OFF:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary"},
			host:  "pxc-0",
			want:  false,
		},
		{
			name:  "the state of another host does not count",
			nodes: []string{"pxc-0:wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Donor/Desynced:wsrep_cluster_status:Primary", "pxc-1" + state},
			host:  "pxc-0",
			want:  false,
		},
		{
			name:  "host that is not listed is not healthy",
			nodes: []string{"pxc-1" + state, "pxc-2" + state},
			host:  "pxc-0",
			want:  false,
		},
		{
			name:  "host name must match exactly, not as a prefix",
			nodes: []string{"pxc-10" + state},
			host:  "pxc-1",
			want:  false,
		},
		{
			name: "no nodes",
			host: "pxc-0",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, hostSyncedPrimary(tt.nodes, tt.host))
		})
	}
}
