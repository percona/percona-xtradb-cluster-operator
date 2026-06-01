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

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}

	tests := []struct {
		name         string
		binlogs      []pxc.Binlog
		lastUploaded string
		want         string
	}{
		{
			// The scenario from #2286: another node's binlogs split the GTID
			// range that one node uploaded as a single entry. The end marker
			// (100) lives in the second of two binlogs.
			name: "end marker in later of two split binlogs",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
				binlog("binlog.000003", uuidA+":101-150"),
			},
			lastUploaded: uuidA + ":1-100",
			want:         "binlog.000002",
		},
		{
			name: "end marker in most recent binlog",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			lastUploaded: uuidA + ":1-50",
			want:         "binlog.000001",
		},
		{
			name: "end marker missing means gap",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-99"),
				binlog("binlog.000003", uuidA+":101-150"),
			},
			lastUploaded: uuidA + ":1-100",
			want:         "",
		},
		{
			name: "binlog with non-contiguous intervals contains end marker",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50:80-100"),
				binlog("binlog.000002", uuidA+":101-150"),
			},
			lastUploaded: uuidA + ":1-100",
			want:         "binlog.000001",
		},
		{
			// After a restore the cluster gets a new UUID, so the last uploaded
			// set may carry both old and new UUID ranges. The match should be
			// found via whichever UUID's end marker exists in a binlog.
			name: "matches via second uuid in lastUploadedSet",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidB+":1-25"),
			},
			lastUploaded: uuidA + ":1-200," + uuidB + ":1-25",
			want:         "binlog.000002",
		},
		{
			name: "binlog carrying multiple uuids matches on the relevant one",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50,"+uuidB+":1-10"),
				binlog("binlog.000002", uuidB+":11-30"),
			},
			lastUploaded: uuidB + ":1-10",
			want:         "binlog.000001",
		},
		{
			// Most-recent-first iteration: when more than one binlog contains
			// the end marker, the most recent one wins.
			name: "most recent binlog wins when multiple contain the end marker",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-100"),
				binlog("binlog.000002", uuidA+":50-150"),
			},
			lastUploaded: uuidA + ":1-100",
			want:         "binlog.000002",
		},
		{
			name:         "empty binlog list returns empty",
			binlogs:      nil,
			lastUploaded: uuidA + ":1-100",
			want:         "",
		},
		{
			name: "malformed lastUploaded entry is skipped, valid one still matches",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
			},
			lastUploaded: "not-a-gtid," + uuidA + ":1-50",
			want:         "binlog.000001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := findBinlogWithEndMarker(tt.binlogs, pxc.NewGTIDSet(tt.lastUploaded))
			assert.Equal(t, tt.want, got)
		})
	}
}
