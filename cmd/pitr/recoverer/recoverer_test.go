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

func TestValidateTransactionGTID(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		desc        string
		targetGTID  string
		startGTID   string
		errContains string
	}{
		{
			desc:       "target after backup range",
			targetGTID: "source-id:41",
			startGTID:  "source-id:1-40",
		},
		{
			desc:       "target matches backup range high end",
			targetGTID: "source-id:40",
			startGTID:  "source-id:1-40",
		},
		{
			desc:       "target after matching range in multi-source GTID set",
			targetGTID: "source-id:41",
			startGTID:  "other-source-id:1-10, source-id:1-40",
		},
		{
			desc:        "target before backup range high end",
			targetGTID:  "source-id:15",
			startGTID:   "source-id:1-40",
			errContains: "already inside the backup",
		},
		{
			desc:        "invalid target GTID format",
			targetGTID:  "source-id",
			startGTID:   "source-id:1-40",
			errContains: "invalid target GTID",
		},
		{
			desc:        "invalid target GTID sequence",
			targetGTID:  "source-id:abc",
			startGTID:   "source-id:1-40",
			errContains: "parse target GTID seqno",
		},
		{
			desc:        "invalid backup range high end",
			targetGTID:  "source-id:41",
			startGTID:   "source-id:1-abc",
			errContains: "parse high end of backup range",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := validateTransactionGTID(ctx, tc.targetGTID, tc.startGTID)
			if tc.errContains == "" {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errContains)
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
			desc: "using xtrabackup_binlog_info",
			mockFn: func(s *mock.Storage) {
				s.On("ListObjects", ctx, "xtrabackup_binlog_info").Return([]string{"xtrabackup_binlog_info.00000000000000000000"}, nil)
				s.On("GetObject", ctx, "xtrabackup_binlog_info.00000000000000000000").Return(newStringReader("binlog.0001\t197\tabc-xyz:1-10\n"), nil)
			},
			expected: "abc-xyz:1-10",
		},
		{
			desc: "using first xtrabackup_binlog_info object",
			mockFn: func(s *mock.Storage) {
				s.On("ListObjects", ctx, "xtrabackup_binlog_info").Return([]string{
					"xtrabackup_binlog_info.00000000000000000001",
					"xtrabackup_binlog_info.00000000000000000000",
				}, nil)
				s.On("GetObject", ctx, "xtrabackup_binlog_info.00000000000000000000").Return(newStringReader("binlog.0001\t197\tabc-xyz:1-10\n"), nil)
			},
			expected: "abc-xyz:1-10",
		},
		{
			desc: "no xtrabackup_binlog_info objects found",
			mockFn: func(s *mock.Storage) {
				s.On("ListObjects", ctx, "xtrabackup_binlog_info").Return([]string{}, nil)
			},
			expected: "",
			wantErr:  true,
		},
		{
			desc: "no gtid in xtrabackup_binlog_info",
			mockFn: func(s *mock.Storage) {
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
