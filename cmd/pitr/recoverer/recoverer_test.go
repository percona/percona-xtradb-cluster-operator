package recoverer

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/mock"
	"github.com/stretchr/testify/assert"
)

func TestGetBucketAndPrefix(t *testing.T) {
	type testCase struct {
		address        string
		expecteBucket  string
		expectedPrefix string
	}
	cases := []testCase{
		{
			address:        "operator-testing/test",
			expecteBucket:  "operator-testing",
			expectedPrefix: "test/",
		},
		{
			address:        "s3://operator-testing/test",
			expecteBucket:  "operator-testing",
			expectedPrefix: "test/",
		},
		{
			address:        "https://somedomain/operator-testing/test",
			expecteBucket:  "operator-testing",
			expectedPrefix: "test/",
		},
		{
			address:        "operator-testing/test/",
			expecteBucket:  "operator-testing",
			expectedPrefix: "test/",
		},
		{
			address:        "operator-testing/test/pitr",
			expecteBucket:  "operator-testing",
			expectedPrefix: "test/pitr/",
		},
		{
			address:        "https://somedomain/operator-testing",
			expecteBucket:  "operator-testing",
			expectedPrefix: "",
		},
		{
			address:        "operator-testing",
			expecteBucket:  "operator-testing",
			expectedPrefix: "",
		},
	}
	for _, c := range cases {
		t.Run(c.address, func(t *testing.T) {
			bucket, prefix, err := getBucketAndPrefix(c.address)
			if err != nil {
				t.Errorf("get from '%s': %s", c.address, err.Error())
			}
			if bucket != c.expecteBucket || prefix != c.expectedPrefix {
				t.Errorf("%s: bucket expect '%s', got '%s'; prefix expect '%s', got '%s'", c.address, c.expecteBucket, bucket, c.expectedPrefix, prefix)
			}
		})
	}
}

func TestGetGTIDFromContent(t *testing.T) {
	c := []byte(`sometext GTID of the last set 'test_set:1-10'
	`)

	set, err := getGTIDFromXtrabackup(c)
	if err != nil {
		t.Error("get last gtid set", err.Error())
	}
	if set != "test_set:1-10" {
		t.Error("set not test_set:1-10 but", set)
	}
}

func TestGetExtendGTIDSet(t *testing.T) {
	type testCase struct {
		gtidSet         string
		gtid            string
		expectedGTIDSet string
	}
	cases := []testCase{
		{
			gtidSet:         "source-id:1-40",
			gtid:            "source-id:15",
			expectedGTIDSet: "source-id:15-40",
		},
		{
			gtidSet:         "source-id:1-40",
			gtid:            "source-id:11-15",
			expectedGTIDSet: "source-id:11-40",
		},
	}
	for _, c := range cases {
		t.Run(c.gtid, func(t *testing.T) {
			set, err := getExtendGTIDSet(c.gtidSet, c.gtid)
			if err != nil {
				t.Errorf("get from '%s': %s", c.gtid, err.Error())
			}
			if set != c.expectedGTIDSet {
				t.Errorf("%s: expect '%s', got '%s'", c.gtid, c.expectedGTIDSet, set)
			}
		})
	}
}

func newStringReader(s string) io.Reader {
	return io.NopCloser(bytes.NewReader([]byte(s)))
}

func TestGetStartGTID(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey{}, true)
	testCases := []struct {
		desc     string
		mockFn   func(*mock.Storage)
		expected string
		wantErr  bool
	}{
		{
			desc: "using sst_info",
			mockFn: func(s *mock.Storage) {
				s.On("ListObjects", ctx, ".sst_info/sst_info").Return([]string{".sst_info/sst_info"}, nil)
				s.On("GetObject", ctx, ".sst_info/sst_info").Return(newStringReader("[sst]\ngalera-gtid=abc-xyz:1-10\n"), nil)
				s.On("ListObjects", ctx, "xtrabackup_info").Return([]string{"xtrabackup_info.00000000000000000000"}, nil)
				s.On("GetObject", ctx, "xtrabackup_info.00000000000000000000").Return(newStringReader("binlog_pos = filename 'binlog.000111', position '237', GTID of the last change 'abc-xyz:1-10'\n"), nil)
			},
			expected: "abc-xyz:1-10",
		},
		{
			desc: "using xtrabackup_binlog_info",
			mockFn: func(s *mock.Storage) {
				s.On("ListObjects", ctx, ".sst_info/sst_info").Return([]string{}, nil)
				s.On("ListObjects", ctx, "xtrabackup_binlog_info").Return([]string{"xtrabackup_binlog_info.00000000000000000000"}, nil)
				s.On("GetObject", ctx, "xtrabackup_binlog_info.00000000000000000000").Return(newStringReader("binlog.0001\t197\tabc-xyz:1-10\n"), nil)
				s.On("ListObjects", ctx, "xtrabackup_info").Return([]string{"xtrabackup_info.00000000000000000000"}, nil)
				s.On("GetObject", ctx, "xtrabackup_info.00000000000000000000").Return(newStringReader("binlog_pos = filename 'binlog.000111', position '237', GTID of the last change 'abc-xyz:1-10'\n"), nil)
			},
			expected: "abc-xyz:1-10",
		},
		{
			desc: "no sst_info or xtrabackup_binlog_info objects found",
			mockFn: func(s *mock.Storage) {
				s.On("ListObjects", ctx, ".sst_info/sst_info").Return([]string{}, nil)
				s.On("ListObjects", ctx, "xtrabackup_binlog_info").Return([]string{}, nil)
			},
			expected: "",
			wantErr:  true,
		},
		{
			desc: "no gtid in xtrabackup_binlog_info",
			mockFn: func(s *mock.Storage) {
				s.On("ListObjects", ctx, ".sst_info/sst_info").Return([]string{}, nil)
				s.On("ListObjects", ctx, "xtrabackup_binlog_info").Return([]string{"xtrabackup_binlog_info.00000000000000000000"}, nil)
				s.On("GetObject", ctx, "xtrabackup_binlog_info.00000000000000000000").Return(newStringReader("binlog.0001\t197\n"), nil)
			},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			mockStorage := mock.NewStorage(t)
			tc.mockFn(mockStorage)

			got, err := getStartGTIDSet(ctx, mockStorage)
			if (err != nil) != tc.wantErr {
				t.Errorf("getStartGTIDSet() error = %v, wantErr %v", err, tc.wantErr)
			}
			assert.Equal(t, tc.expected, got)
		})
	}
}
