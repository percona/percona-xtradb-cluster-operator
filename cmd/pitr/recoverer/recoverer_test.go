package recoverer

import (
	"bytes"
	"testing"
)

func TestGetBucketAndPrefix(t *testing.T) {
	bucket, prefix, err := getBucketAndPrefix("operator-testing/test")
	if err != nil {
		t.Error("get from 'operator-testing/test'", err.Error())
	}
	if bucket != "operator-testing" && prefix != "test" {
		t.Error("wrong parsing of 'operator-testing/test'")
	}
	bucket, prefix, err = getBucketAndPrefix("s3://operator-testing/test")
	if err != nil {
		t.Error("get from 'operator-testing/test'", err.Error())
	}
	if bucket != "operator-testing" && prefix != "test" {
		t.Error("wrong parsing of 'operator-testing/test'")
	}
	bucket, prefix, err = getBucketAndPrefix("https://somedomain/operator-testing/test")
	if err != nil {
		t.Error("get from 'operator-testing/test'", err.Error())
	}
	if bucket != "operator-testing" && prefix != "test" {
		t.Error("wrong parsing of 'operator-testing/test'")
	}
}

func TestGetLastBackupGTID(t *testing.T) {
	s := `sometext GTID of the last set 'test_set:1-10'
	`
	buf := bytes.NewBuffer([]byte(s))
	set, err := getLastBackupGTID(buf)
	if err != nil {
		t.Error("get last gtid set", err.Error())
	}
	if set != "test_set:1-10" {
		t.Error("set not test_set:1-10 but", set)
	}
}
