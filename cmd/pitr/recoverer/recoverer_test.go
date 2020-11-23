package recoverer

import (
	"io"
	"strings"
	"testing"
)

type testStorage struct {
}

func (t *testStorage) GetObject(name string) ([]byte, error) {
	if strings.Contains(name, "gtid-set") {
		return []byte("someset-name:1-15"), nil
	}
	return []byte("some text with GTID of the last some-set:9-13'\n"), nil
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
	err := r.SetBinlogs(1)
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
	lastID, err := r.GetLastBackupGTID()
	if err != nil {
		t.Error("setBinlogs", err.Error())
	}
	if lastID != 13 {
		t.Error("incorrect last gtid")
	}
}
