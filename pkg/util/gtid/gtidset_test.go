package gtid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGTIDSetInterval(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		wantEndUUID   string
		wantEndSeq    int64
		wantStartUUID string
		wantStartSeq  int64
		wantErr       bool
		filters       []SegmentFilter
	}{
		{
			name:          "simple range",
			wantEndUUID:   "uuid",
			wantEndSeq:    6093543,
			wantStartUUID: "uuid",
			wantStartSeq:  6093289,
			in:            "uuid:6093289-6093543",
		},
		{
			name:          "single number",
			wantEndUUID:   "uuid",
			wantEndSeq:    42,
			wantStartUUID: "uuid",
			wantStartSeq:  42,
			in:            "uuid:42",
		},
		{
			name:          "multiple intervals chooses highest",
			wantEndUUID:   "uuid",
			wantEndSeq:    9,
			wantStartUUID: "uuid",
			wantStartSeq:  1,
			in:            "uuid:1-5:7-9",
		},
		{
			name:          "multiple intervals with singletons",
			wantEndUUID:   "uuid",
			wantEndSeq:    10,
			wantStartUUID: "uuid",
			wantStartSeq:  3,
			in:            "uuid:10:3-7:8",
		},
		{
			name:          "whitespace is tolerated",
			in:            "  uuid  :  1-2 :  9-11 ",
			wantStartUUID: "uuid",
			wantStartSeq:  1,
			wantEndUUID:   "uuid",
			wantEndSeq:    11,
		},
		{
			name:    "missing colon is invalid",
			in:      "uuid6093289-6093543",
			wantErr: true,
		},
		{
			name:    "empty uuid is invalid",
			in:      ":1-2",
			wantErr: true,
		},
		{
			name:    "no numeric intervals is invalid",
			in:      "uuid:abc-def",
			wantErr: true,
		},
		{
			name:          "skips invalid interval but still finds valid one",
			in:            "uuid:abc-def:5-7",
			wantStartUUID: "uuid",
			wantStartSeq:  5,
			wantEndUUID:   "uuid",
			wantEndSeq:    7,
		},
		{
			name:    "empty right side is invalid",
			in:      "uuid:",
			wantErr: true,
		},
		{
			name:    "only separators is invalid",
			in:      "uuid::::",
			wantErr: true,
		},

		{
			name:          "multiple uuids",
			in:            "uuid1:1-10,uuid2:11-20",
			wantStartUUID: "uuid1",
			wantStartSeq:  1,
			wantEndUUID:   "uuid2",
			wantEndSeq:    20,
		},

		{
			name:          "multiple uuids with filters",
			in:            "uuid1:1-10,uuid2:11-20",
			filters:       []SegmentFilter{MatchesUUID("uuid1")},
			wantStartUUID: "uuid1",
			wantStartSeq:  1,
			wantEndUUID:   "uuid1",
			wantEndSeq:    10,
		},

		{
			name:          "empty gtid",
			in:            "",
			wantStartUUID: "",
			wantStartSeq:  0,
			wantEndUUID:   "",
			wantEndSeq:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			gtidset, err := New(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			startUUID, startSeq := gtidset.Start(tt.filters...)
			endUUID, endSeq := gtidset.End(tt.filters...)
			assert.Equal(t, tt.wantStartUUID, startUUID)
			assert.Equal(t, tt.wantStartSeq, startSeq)
			assert.Equal(t, tt.wantEndUUID, endUUID)
			assert.Equal(t, tt.wantEndSeq, endSeq)

		})
	}
}

func TestGTIDSetContainsSeq(t *testing.T) {
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
			name:  "invalid intervals are skipped",
			entry: "uuid:abc-def:5-7",
			uuid:  "uuid",
			seq:   6,
			want:  true,
		},
		{
			name:  "empty interval segments are ignored",
			entry: "uuid:1-2:::4-5",
			uuid:  "uuid",
			seq:   4,
			want:  true,
		},

		{
			name:  "empty",
			entry: "",
			uuid:  "uuid",
			seq:   1,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gtidset, err := New(tt.entry)
			require.NoError(t, err)
			assert.Equal(t, tt.want, gtidset.ContainsSeq(tt.uuid, tt.seq))
		})
	}
}

func TestGTIDSetContainsUUID(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		uuid  string
		want  bool
	}{
		{
			name:  "single segment, contains uuid",
			entry: "uuid:1-10",
			uuid:  "uuid",
			want:  true,
		},
		{
			name:  "single segment, does not contain uuid",
			entry: "uuid:1-10",
			uuid:  "other",
			want:  false,
		},
		{
			name:  "multiple segments, contains uuid",
			entry: "uuid:1-10,other:1-10",
			uuid:  "uuid",
			want:  true,
		},
		{
			name:  "multiple segments, does not contain uuid",
			entry: "uuid:1-10,other:1-10",
			uuid:  "missing",
			want:  false,
		},

		{
			name:  "empty",
			entry: "",
			uuid:  "uuid",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gtidset, err := New(tt.entry)
			require.NoError(t, err)
			assert.Equal(t, tt.want, gtidset.ContainsUUID(tt.uuid))
		})
	}
}

func TestGTIDSetEqual(t *testing.T) {
	testCases := []struct {
		desc string
		a    string
		b    string
		want bool
	}{
		{
			desc: "identical single-source sets",
			a:    "uuid-a:1-10",
			b:    "uuid-a:1-10",
			want: true,
		},
		{
			desc: "multi-source set with newline after comma matches single-line",
			a:    "uuid-a:1-15,\nuuid-b:1-304",
			b:    "uuid-a:1-15,uuid-b:1-304",
			want: true,
		},
		{
			desc: "multi-source set with leading/trailing whitespace",
			a:    "  uuid-a:1-15, uuid-b:1-304 ",
			b:    "uuid-a:1-15,uuid-b:1-304",
			want: true,
		},
		{
			desc: "different segment order",
			a:    "uuid-b:1-304,uuid-a:1-15",
			b:    "uuid-a:1-15,uuid-b:1-304",
			want: true,
		},
		{
			desc: "different ranges",
			a:    "uuid-a:1-15,uuid-b:1-304",
			b:    "uuid-a:1-16,uuid-b:1-304",
			want: false,
		},
		{
			desc: "missing segment",
			a:    "uuid-a:1-15",
			b:    "uuid-a:1-15,uuid-b:1-304",
			want: false,
		},
		{
			desc: "empty",
			a:    "",
			b:    "",
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			aGTIDSet, err := New(tc.a)
			require.NoError(t, err)

			bGTIDSet, err := New(tc.b)
			require.NoError(t, err)

			assert.Equal(t, tc.want, aGTIDSet.Equal(bGTIDSet))
		})
	}
}
