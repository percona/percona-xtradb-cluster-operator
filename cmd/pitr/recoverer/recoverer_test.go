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

	set, err := getGTIDFromContent(c)
	if err != nil {
		t.Error("get last gtid set", err.Error())
	}
	if set != "test_set:1-10" {
		t.Error("set not test_set:1-10 but", set)
	}
}

func TestGetLastGTIDFromSet(t *testing.T) {
	type testCase struct {
		gtidSet      string
		expectedGTID string
	}

	cases := []testCase{
		{
			gtidSet:      "f2c837be-7069-11eb-900d-bb9b8c763e5b:1-17",
			expectedGTID: "f2c837be-7069-11eb-900d-bb9b8c763e5b:17",
		},
		{
			gtidSet:      "f2c837be-7069-11eb-900d-bb9b8c763e5b:1",
			expectedGTID: "f2c837be-7069-11eb-900d-bb9b8c763e5b:1",
		},
		{
			gtidSet:      "f2c837be-7069-11eb-900d-bb9b8c763e5b:17",
			expectedGTID: "f2c837be-7069-11eb-900d-bb9b8c763e5b:17",
		},
	}
	for _, c := range cases {
		t.Run(c.gtidSet, func(t *testing.T) {
			set := getLastGTIDFromSet(c.gtidSet)
			if set != c.expectedGTID {
				t.Errorf("expect '%s', got '%s'", c.expectedGTID, set)
			}
		})
	}
}
