package collector

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

func TestReadBinlog(t *testing.T) {
	ctx := context.Background()

	file, err := os.CreateTemp("", "test-binlog-*.bin")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	pipeReader, pipeWriter := io.Pipe()
	defer pipeReader.Close()
	defer pipeWriter.Close()

	errBuf := &bytes.Buffer{}

	var wg sync.WaitGroup
	wg.Go(func() {
		readBinlog(ctx, file, pipeWriter, errBuf, "test-binlog")
	})

	testData := "foo"
	if _, err := file.Write([]byte(testData)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}

	if err := file.Sync(); err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("failed to seek: %v", err)
	}

	var resultBuf bytes.Buffer
	_, err = io.Copy(&resultBuf, pipeReader)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("error: %v", err)
	}

	pipeWriter.Close()

	wg.Wait()

	if resultBuf.String() != testData {
		t.Errorf("expect %q, got %q", testData, resultBuf.String())
	}
}

func TestFindBinlogWithEndMarker(t *testing.T) {
	t.Parallel()

	binlogs := []pxc.Binlog{
		{Name: "binlog.000001"},
		{Name: "binlog.000002"},
		{Name: "binlog.000003"},
	}

	tests := []struct {
		name    string
		binlogs []pxc.Binlog
		names   []string
		want    string
	}{
		{
			name:    "single match",
			binlogs: binlogs,
			names:   []string{"binlog.000002"},
			want:    "binlog.000002",
		},
		{
			// After a restore the last uploaded set may carry both old and new UUID
			// ranges, whose end markers can be in different binlogs.
			name:    "most recent binlog wins when several match",
			binlogs: binlogs,
			names:   []string{"binlog.000001", "binlog.000003"},
			want:    "binlog.000003",
		},
		{
			name:    "order of names doesn't matter",
			binlogs: binlogs,
			names:   []string{"binlog.000003", "binlog.000001"},
			want:    "binlog.000003",
		},
		{
			name:    "no match means gap",
			binlogs: binlogs,
			names:   nil,
			want:    "",
		},
		{
			name:    "binlog that is not in the list means gap",
			binlogs: binlogs,
			names:   []string{"binlog.000009"},
			want:    "",
		},
		{
			name:    "empty binlog list",
			binlogs: nil,
			names:   []string{"binlog.000001"},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, findBinlogWithEndMarker(tt.binlogs, tt.names))
		})
	}
}

type closeTrackingReader struct {
	io.Reader
	closed bool
}

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

type lastSetStorage struct {
	storage.Storage
	obj *closeTrackingReader
}

func (s *lastSetStorage) GetObject(_ context.Context, _ string) (io.ReadCloser, error) {
	return s.obj, nil
}

func TestLastGTIDSetClosesObject(t *testing.T) {
	t.Parallel()

	const gtidSet = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa:1-10"

	obj := &closeTrackingReader{Reader: bytes.NewBufferString(gtidSet)}
	c := &Collector{storage: &lastSetStorage{obj: obj}}

	set, err := c.lastGTIDSet(context.Background(), "uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, gtidSet, set.Raw())
	assert.True(t, obj.closed, "last set object must be closed")
}
