package recoverer

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type testStorage struct {
}

type reader struct {
	r io.Reader
}

func (r *reader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func (t *testStorage) GetObject(name string) (io.Reader, error) {
	if strings.Contains(name, "gtid-set") {
		buf := bytes.NewBufferString("someset-name:1-15")
		return buf, nil
	}
	buf := bytes.NewBufferString("some text with GTID of the last some-set:9-13'\n")
	return buf, nil
}

func (t *testStorage) PutObject(name string, data io.Reader) error {
	return nil
}

func (t *testStorage) ListObjects(prefix string) []string {
	return []string{
		"binlog.00001",
		"binlog.00001-last-gtid",
		"justbackup",
		"binlog.00002",
	}
}

func TestGetBinlogList(t *testing.T) {
	ts := &testStorage{}
	r := Recoverer{
		storage: ts,
	}
	err := r.setBinlogs(1)
	if err != nil {
		t.Error("setBinlogs", err.Error())
	}
	if r.binlogs[0] != "binlog.00001" && r.binlogs[1] != "binlog.00002" {
		t.Error("incorrect binlog set")
	}
}

func TestGetLastBackupGTID(t *testing.T) {
	ts := &testStorage{}
	r := Recoverer{
		storage: ts,
	}
	lastID, err := r.getLastBackupGTID()
	if err != nil {
		t.Error("setBinlogs", err.Error())
	}
	if lastID != 13 {
		t.Error("incorrect last gtid")
	}
}
