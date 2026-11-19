package pxc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestPodNameFromHost(t *testing.T) {
	tests := map[string]struct {
		host string
		want string
	}{
		"a pod FQDN reduces to its first label": {
			host: "cluster1-pxc-1.cluster1-pxc.pxc.svc.cluster.local",
			want: "cluster1-pxc-1",
		},
		"a short service-scoped name works too": {
			host: "cluster1-pxc-2.cluster1-pxc",
			want: "cluster1-pxc-2",
		},
		"a bare pod name is returned unchanged": {
			host: "cluster1-pxc-0",
			want: "cluster1-pxc-0",
		},
		"an empty host stays empty": {
			host: "",
			want: "",
		},
		// A trailing dot is a legal absolute DNS name.
		"a rooted FQDN still reduces": {
			host: "cluster1-pxc-1.cluster1-pxc.pxc.svc.cluster.local.",
			want: "cluster1-pxc-1",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, podNameFromHost(tt.host))
		})
	}
}

func TestNewSourceNode(t *testing.T) {
	tests := map[string]struct {
		cluster  string
		ns       string
		pod      string
		wantFQDN string
	}{
		"pod of a cluster": {
			cluster:  "cluster1",
			ns:       "pxc",
			pod:      "cluster1-pxc-1",
			wantFQDN: "cluster1-pxc-1.cluster1-pxc.pxc",
		},
		"another namespace": {
			cluster:  "cluster1",
			ns:       "other",
			pod:      "cluster1-pxc-0",
			wantFQDN: "cluster1-pxc-0.cluster1-pxc.other",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBCluster{
				ObjectMeta: metav1.ObjectMeta{Name: tt.cluster, Namespace: tt.ns},
			}

			node := NewSourceNode(cr, tt.pod)

			assert.Equal(t, tt.pod, node.Pod)
			assert.Equal(t, tt.wantFQDN, node.FQDN)
		})
	}
}
