package v1

import (
	"strings"
	"testing"
)

func TestPITRValidate(t *testing.T) {
	tests := []struct {
		name    string
		pitr    PITR
		wantErr string
	}{
		// --- valid cases ---
		{
			name: "type latest without date and gtid",
			pitr: PITR{Type: "latest"},
		},
		{
			name: "type date with valid format",
			pitr: PITR{Type: "date", Date: "2024-01-15 12:30:00"},
		},
		{
			name: "type transaction with gtid",
			pitr: PITR{Type: "transaction", GTID: "12345678-1234-1234-1234-123456789abc:1"},
		},
		{
			name: "type skip with gtid",
			pitr: PITR{Type: "skip", GTID: "12345678-1234-1234-1234-123456789abc:1"},
		},

		// --- type latest must not have date or gtid ---
		{
			name:    "type latest with date",
			pitr:    PITR{Type: "latest", Date: "2024-01-15 12:30:00"},
			wantErr: "date should not be set when type is 'latest'",
		},
		{
			name:    "type latest with gtid",
			pitr:    PITR{Type: "latest", GTID: "some-gtid"},
			wantErr: "gtid should not be set when type is 'latest'",
		},

		// --- type date validation ---
		{
			name:    "type date without date field",
			pitr:    PITR{Type: "date"},
			wantErr: "date is required for type 'date'",
		},
		{
			name:    "type date with wrong format",
			pitr:    PITR{Type: "date", Date: "15-01-2024 12:30:00"},
			wantErr: "date should be in format YYYY-MM-DD HH:MM:SS",
		},
		// RFC 3339 date-time format with T separator is not accepted (we use MySQL format with space)
		{
			name:    "type date with T separator instead of space",
			pitr:    PITR{Type: "date", Date: "2024-01-15T12:30:00"},
			wantErr: "date should be in format YYYY-MM-DD HH:MM:SS",
		},

		// --- type transaction/skip require gtid ---
		{
			name:    "type transaction without gtid",
			pitr:    PITR{Type: "transaction"},
			wantErr: `gtid is required for type "transaction"`,
		},
		{
			name:    "type skip without gtid",
			pitr:    PITR{Type: "skip"},
			wantErr: `gtid is required for type "skip"`,
		},

		// --- unknown type ---
		{
			name:    "unknown type",
			pitr:    PITR{Type: "unknown"},
			wantErr: `unknown type "unknown"`,
		},
		{
			name:    "empty type",
			pitr:    PITR{Type: ""},
			wantErr: `unknown type ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pitr.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if got := err.Error(); !strings.Contains(got, tt.wantErr) {
				t.Errorf("expected error containing %q, got: %q", tt.wantErr, got)
			}
		})
	}
}
