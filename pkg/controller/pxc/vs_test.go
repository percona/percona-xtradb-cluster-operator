package pxc

import (
	"fmt"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/version/client/models"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetPMMVersion(t *testing.T) {
	tests := map[string]struct {
		versions map[string]models.VersionVersion
		expected string
		err      error
	}{
		"empty map returns error": {
			versions: map[string]models.VersionVersion{},
			err:      fmt.Errorf("response has zero versions"),
		},
		"single 3.x version": {
			versions: map[string]models.VersionVersion{
				"3.0.1": {},
			},
			expected: "3.0.1",
		},
		"2.x next to 3.x picks 3.x": {
			versions: map[string]models.VersionVersion{
				"2.27.0": {},
				"3.1.0":  {},
			},
			expected: "3.1.0",
		},
		"only 2.x versions": {
			versions: map[string]models.VersionVersion{
				"2.27.0": {},
				"2.29.0": {},
			},
			err: fmt.Errorf("no PMM 3.x version found"),
		},
		"more than one 3.x version": {
			versions: map[string]models.VersionVersion{
				"3.0.1": {},
				"3.1.0": {},
			},
			err: fmt.Errorf("response has more than one PMM 3.x version"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			version, err := getPMMVersion(tt.versions)

			if tt.err != nil {
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, version)
			}
		})
	}
}
