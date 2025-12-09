package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMySQLVersionFromVersionStr(t *testing.T) {
	tests := []struct {
		versionStr string
		expected   string
	}{
		{versionStr: "xtrabackup version 8.4.0-12 based on MySQL server 8.4.0 Linux (x86_64) (revision id: c8a25ff9)", expected: "8.4.0"},
		{versionStr: "xtrabackup version 8.0.35-34 based on MySQL server 8.0.35 Linux (x86_64) (revision id: c8a25ff9)", expected: "8.0.35"},
		{versionStr: "xtrabackup version 5.7.40-xy based on MySQL server 5.7.40 Linux (x86_64) (revision id: 25cdf1e)", expected: "5.7.40"},
	}

	for _, tt := range tests {
		t.Run(tt.versionStr, func(t *testing.T) {
			actual := parseMySQLVersionFromVersionStr(tt.versionStr)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
