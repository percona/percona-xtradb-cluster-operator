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
		isPMM3   bool
		expected string
		err      error
	}{
		"empty map returns error": {
			versions: map[string]models.VersionVersion{},
			isPMM3:   false,
			err:      fmt.Errorf("response has zero versions"),
		},
		"single 2.x version, PMM3 disabled": {
			versions: map[string]models.VersionVersion{
				"2.27.0": {},
			},
			isPMM3:   false,
			expected: "2.27.0",
		},
		"single 3.x version, PMM3 enabled": {
			versions: map[string]models.VersionVersion{
				"3.0.1": {},
			},
			isPMM3:   true,
			expected: "3.0.1",
		},
		"multiple versions, PMM3 enabled, has 3.x": {
			versions: map[string]models.VersionVersion{
				"2.27.0": {},
				"3.1.0":  {},
			},
			isPMM3:   true,
			expected: "3.1.0",
		},
		"multiple versions, PMM3 enabled, no 3.x": {
			versions: map[string]models.VersionVersion{
				"2.27.0": {},
				"2.29.0": {},
			},
			isPMM3: true,
			err:    fmt.Errorf("pmm3 is configured, but no pmm3 version exists"),
		},
		"multiple versions, PMM3 disabled": {
			versions: map[string]models.VersionVersion{
				"2.27.0": {},
				"3.1.0":  {},
			},
			isPMM3:   false,
			expected: "2.27.0",
		},
		"multiple versions, no 3.x": {
			versions: map[string]models.VersionVersion{
				"2.27.0": {},
				"2.29.0": {},
				"2.31.0": {},
			},
			isPMM3: false,
			err:    fmt.Errorf("response has more than 2 versions"),
		},
		"single 1.x version, PMM3 disabled": {
			versions: map[string]models.VersionVersion{
				"1.27.0": {},
			},
			isPMM3: false,
			err:    fmt.Errorf("no recognizable PMM version found"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			version, err := getPMMVersion(tt.versions, tt.isPMM3)

			if tt.err != nil {
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, version)
			}
		})
	}
}
