package collector

import "testing"

func TestGTIDEndMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		wantUUID string
		wantEnd  int64
		wantOK   bool
	}{
		{
			name:     "simple range",
			in:       "uuid:6093289-6093543",
			wantUUID: "uuid",
			wantEnd:  6093543,
			wantOK:   true,
		},
		{
			name:     "single number",
			in:       "uuid:42",
			wantUUID: "uuid",
			wantEnd:  42,
			wantOK:   true,
		},
		{
			name:     "multiple intervals chooses highest",
			in:       "uuid:1-5:7-9",
			wantUUID: "uuid",
			wantEnd:  9,
			wantOK:   true,
		},
		{
			name:     "multiple intervals with singletons",
			in:       "uuid:10:3-7:8",
			wantUUID: "uuid",
			wantEnd:  10,
			wantOK:   true,
		},
		{
			name:     "whitespace is tolerated",
			in:       "  uuid  :  1-2 :  9-11 ",
			wantUUID: "uuid",
			wantEnd:  11,
			wantOK:   true,
		},
		{
			name:   "missing colon is invalid",
			in:     "uuid6093289-6093543",
			wantOK: false,
		},
		{
			name:   "empty uuid is invalid",
			in:     ":1-2",
			wantOK: false,
		},
		{
			name:   "no numeric intervals is invalid",
			in:     "uuid:abc-def",
			wantOK: false,
		},
		{
			name:     "skips invalid interval but still finds valid one",
			in:       "uuid:abc-def:5-7",
			wantUUID: "uuid",
			wantEnd:  7,
			wantOK:   true,
		},
		{
			name:   "empty right side is invalid",
			in:     "uuid:",
			wantOK: false,
		},
		{
			name:   "only separators is invalid",
			in:     "uuid::::",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotUUID, gotEnd, gotOK := gtidEndMarker(tt.in)

			if gotOK != tt.wantOK {
				t.Fatalf("ok: got %v, want %v (uuid=%q end=%d)", gotOK, tt.wantOK, gotUUID, gotEnd)
			}
			if !tt.wantOK {
				return
			}
			if gotUUID != tt.wantUUID {
				t.Fatalf("uuid: got %q, want %q", gotUUID, tt.wantUUID)
			}
			if gotEnd != tt.wantEnd {
				t.Fatalf("endSeq: got %d, want %d", gotEnd, tt.wantEnd)
			}
		})
	}
}

func TestGTIDContainsSeq(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry string
		uuid  string
		seq   int64
		want  bool
	}{
		{
			name:  "contains inside range",
			entry: "uuid:1-10",
			uuid:  "uuid",
			seq:   5,
			want:  true,
		},
		{
			name:  "contains at range start",
			entry: "uuid:1-10",
			uuid:  "uuid",
			seq:   1,
			want:  true,
		},
		{
			name:  "contains at range end",
			entry: "uuid:1-10",
			uuid:  "uuid",
			seq:   10,
			want:  true,
		},
		{
			name:  "does not contain outside range",
			entry: "uuid:1-10",
			uuid:  "uuid",
			seq:   11,
			want:  false,
		},
		{
			name:  "contains single number",
			entry: "uuid:42",
			uuid:  "uuid",
			seq:   42,
			want:  true,
		},
		{
			name:  "does not contain different single number",
			entry: "uuid:42",
			uuid:  "uuid",
			seq:   43,
			want:  false,
		},
		{
			name:  "contains in later interval",
			entry: "uuid:1-5:7-9",
			uuid:  "uuid",
			seq:   8,
			want:  true,
		},
		{
			name:  "does not contain gap between intervals",
			entry: "uuid:1-5:7-9",
			uuid:  "uuid",
			seq:   6,
			want:  false,
		},
		{
			name:  "wrong uuid",
			entry: "uuid:1-10",
			uuid:  "other",
			seq:   5,
			want:  false,
		},
		{
			name:  "tolerates whitespace",
			entry: " uuid : 1-2 : 9-11 ",
			uuid:  "uuid",
			seq:   10,
			want:  true,
		},
		{
			name:  "invalid entry no colon",
			entry: "uuid1-10",
			uuid:  "uuid",
			seq:   5,
			want:  false,
		},
		{
			name:  "invalid intervals are skipped",
			entry: "uuid:abc-def:5-7",
			uuid:  "uuid",
			seq:   6,
			want:  true,
		},
		{
			name:  "all intervals invalid means false",
			entry: "uuid:abc-def:ghi",
			uuid:  "uuid",
			seq:   1,
			want:  false,
		},
		{
			name:  "empty interval segments are ignored",
			entry: "uuid:1-2:::4-5",
			uuid:  "uuid",
			seq:   4,
			want:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := gtidContainsSeq(tt.entry, tt.uuid, tt.seq)
			if got != tt.want {
				t.Fatalf("got %v, want %v (entry=%q uuid=%q seq=%d)", got, tt.want, tt.entry, tt.uuid, tt.seq)
			}
		})
	}
}
