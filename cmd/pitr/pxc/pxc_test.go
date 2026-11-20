package pxc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSyncedPrimary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node string
		want bool
	}{
		{
			name: "synced primary",
			node: "pxc-0.pxc:wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary",
			want: true,
		},
		{
			name: "donor",
			node: "pxc-0.pxc:wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Donor/Desynced:wsrep_cluster_status:Primary",
			want: false,
		},
		{
			name: "not primary",
			node: "pxc-0.pxc:wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Non-Primary",
			want: false,
		},
		{
			name: "not ready",
			node: "pxc-0.pxc:wsrep_ready:OFF:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary",
			want: false,
		},
		{
			name: "empty",
			node: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, isSyncedPrimary(tt.node))
		})
	}
}
