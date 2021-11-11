package recoverer

import (
	"testing"
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
