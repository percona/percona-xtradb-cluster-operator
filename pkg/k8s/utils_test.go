package k8s

import "testing"

func TestImageWithTag(t *testing.T) {
	tests := map[string]struct {
		image string
		want  string
	}{
		"tag": {
			image: "percona/percona-xtradb-cluster-operator:1.18.0",
			want:  "percona/percona-xtradb-cluster-operator:1.19.0",
		},
		"no tag": {
			image: "percona/percona-xtradb-cluster-operator",
			want:  "percona/percona-xtradb-cluster-operator:1.19.0",
		},
		"digest": {
			image: "percona/percona-xtradb-cluster-operator@sha256:6f7d8d4e472b8c4d166573cc7bb714bbb0fdf1535142b6138c62fdecbf881df9",
			want:  "percona/percona-xtradb-cluster-operator:1.19.0",
		},
		"tag and digest": {
			image: "percona/percona-xtradb-cluster-operator:1.18.0@sha256:6f7d8d4e472b8c4d166573cc7bb714bbb0fdf1535142b6138c62fdecbf881df9",
			want:  "percona/percona-xtradb-cluster-operator:1.19.0",
		},
		"registry with port": {
			image: "registry:5000/percona/percona-xtradb-cluster-operator:1.18.0",
			want:  "registry:5000/percona/percona-xtradb-cluster-operator:1.19.0",
		},
		"registry with port, no tag": {
			image: "registry:5000/percona/percona-xtradb-cluster-operator",
			want:  "registry:5000/percona/percona-xtradb-cluster-operator:1.19.0",
		},
		"registry with port, tag and digest": {
			image: "registry:5000/percona/percona-xtradb-cluster-operator:1.18.0@sha256:6f7d8d4e472b8c4d166573cc7bb714bbb0fdf1535142b6138c62fdecbf881df9",
			want:  "registry:5000/percona/percona-xtradb-cluster-operator:1.19.0",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := imageWithTag(tt.image, "1.19.0"); got != tt.want {
				t.Errorf("imageWithTag(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}
