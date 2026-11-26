package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPXCBackupStatusIsLogical(t *testing.T) {
	tests := map[string]struct {
		status   *PXCBackupStatus
		expected bool
	}{
		"nil status": {
			status:   nil,
			expected: false,
		},
		"empty type is read as full": {
			status:   &PXCBackupStatus{},
			expected: false,
		},
		"full": {
			status:   &PXCBackupStatus{Type: BackupTypeFull},
			expected: false,
		},
		"logical": {
			status:   &PXCBackupStatus{Type: BackupTypeLogical},
			expected: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.IsLogical())
		})
	}
}
