package collector

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util/gtid"
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

// fakeLocator mimics the binlog UDFs on a host: binlogs hold per-file GTID sets
// and a GTID is found in the first binlog containing it.
type fakeLocator struct {
	binlogs []pxc.Binlog
	invalid map[string]bool  // binlogs failing with MySQL error 3200
	failing map[string]error // binlogs failing with the given error
	fetched []string         // binlogs GetGTIDSet was called for, in order
}

func (f *fakeLocator) GetGTIDSet(_ context.Context, name string) (string, error) {
	f.fetched = append(f.fetched, name)
	if err := f.failing[name]; err != nil {
		return "", err
	}
	if f.invalid[name] {
		return "", &mysql.MySQLError{Number: 3200, Message: "invalid binlog"}
	}
	for _, b := range f.binlogs {
		if b.Name == name {
			return b.GTIDSet.Raw(), nil
		}
	}
	return "", errors.New("Binary log does not exist")
}

func (f *fakeLocator) GetBinlogByGTID(_ context.Context, gtidText string) (string, error) {
	parsed, err := gtid.New(gtidText)
	if err != nil {
		return "", err
	}
	uuid, seq := parsed.End()
	for _, b := range f.binlogs {
		set, err := gtid.New(b.GTIDSet.Raw())
		if err != nil {
			continue
		}
		if set.ContainsSeq(uuid, seq) {
			return b.Name, nil
		}
	}
	return "", nil
}

type fakeStorage struct {
	storage.Storage
	objects map[string]string
}

func (s *fakeStorage) GetObject(_ context.Context, name string) (io.ReadCloser, error) {
	content, ok := s.objects[name]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewBufferString(content)), nil
}

func TestFindResumePoint(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}

	tests := []struct {
		name     string
		binlogs  []pxc.Binlog
		invalid  map[string]bool
		stored   map[string]string // object name -> content
		sourceID string
		want     resumePoint
	}{
		{
			name:     "nothing uploaded yet starts from the first binlog",
			binlogs:  []pxc.Binlog{binlog("binlog.000001", uuidA+":1-50")},
			sourceID: uuidA,
			want:     resumePoint{},
		},
		{
			name: "no binlog has transactions",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", ""),
				binlog("binlog.000002", ""),
			},
			sourceID: uuidA,
			want:     resumePoint{empty: true},
		},
		{
			name:     "no binlogs",
			binlogs:  nil,
			sourceID: uuidA,
			want:     resumePoint{empty: true},
		},
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
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":1-100"},
			sourceID: uuidA,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-100"), start: 1},
		},
		{
			name: "end marker in the most recent binlog",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":51-100"},
			sourceID: uuidA,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":51-100"), start: 1},
		},
		{
			name: "latest binlogs without transactions are skipped",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
				binlog("binlog.000003", ""),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":51-100"},
			sourceID: uuidA,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":51-100"), start: 1},
		},
		{
			name: "invalid latest binlog is skipped",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			invalid:  map[string]bool{"binlog.000002": true},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":1-50"},
			sourceID: uuidA,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-50"), start: 0},
		},
		{
			name: "end marker already purged from the host is a gap",
			binlogs: []pxc.Binlog{
				binlog("binlog.000005", uuidA+":101-150"),
				binlog("binlog.000006", uuidA+":151-200"),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":1-60"},
			sourceID: uuidA,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-60"), gap: true},
		},
		{
			name: "end marker missing between binlogs is a gap",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":101-150"),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":1-100"},
			sourceID: uuidA,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-100"), gap: true},
		},
		{
			// After a restore the last uploaded set can carry both old and new
			// UUID ranges. The match is found via whichever end marker exists.
			name: "matches via second uuid in the last uploaded set",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidB+":1-25"),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidB: uuidA + ":1-200," + uuidB + ":1-25"},
			sourceID: uuidB,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-200," + uuidB + ":1-25"), start: 1},
		},
		{
			name: "most recent binlog wins when several uuids match",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidB+":1-25"),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidB: uuidA + ":1-50," + uuidB + ":1-25"},
			sourceID: uuidB,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-50," + uuidB + ":1-25"), start: 1},
		},
		{
			name: "malformed last uploaded entry is skipped, valid one still matches",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidA: "not-a-gtid," + uuidA + ":1-50"},
			sourceID: uuidA,
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet("not-a-gtid," + uuidA + ":1-50"), start: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &Collector{storage: &fakeStorage{objects: tt.stored}}
			loc := &fakeLocator{binlogs: tt.binlogs, invalid: tt.invalid}

			got, err := c.findResumePoint(context.Background(), loc, tt.sourceID, tt.binlogs)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasNewTransactions(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	tests := []struct {
		name         string
		lastGTID     string
		lastUploaded string
		want         bool
	}{
		{
			name:         "last gtid is the end of the uploaded set",
			lastGTID:     uuidA + ":100",
			lastUploaded: uuidA + ":51-100",
			want:         false,
		},
		{
			name:         "last gtid is inside the uploaded set",
			lastGTID:     uuidA + ":75",
			lastUploaded: uuidA + ":51-100",
			want:         false,
		},
		{
			name:         "binlog goes beyond the uploaded set",
			lastGTID:     uuidA + ":120",
			lastUploaded: uuidA + ":51-100",
			want:         true,
		},
		{
			name:         "last gtid from another uuid",
			lastGTID:     uuidB + ":10",
			lastUploaded: uuidA + ":1-100",
			want:         true,
		},
		{
			name:         "contained in any uuid of a multi uuid set",
			lastGTID:     uuidB + ":10",
			lastUploaded: uuidA + ":1-100," + uuidB + ":1-25",
			want:         false,
		},
		{
			name:         "binlog without gtids is uploaded",
			lastGTID:     "",
			lastUploaded: uuidA + ":1-100",
			want:         true,
		},
		{
			name:         "unparsable last gtid is uploaded",
			lastGTID:     "garbage",
			lastUploaded: uuidA + ":1-100",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := hasNewTransactions(tt.lastGTID, pxc.NewGTIDSet(tt.lastUploaded))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestForEachBinlogWithGTIDs(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}
	errBoom := errors.New("boom")

	tests := []struct {
		name        string
		binlogs     []pxc.Binlog
		invalid     map[string]bool
		failing     map[string]error
		failFnOn    string // fn returns an error for this binlog
		wantFn      []string
		wantFetched []string
		wantErr     bool
	}{
		{
			name:        "no binlogs",
			binlogs:     nil,
			wantFn:      nil,
			wantFetched: nil,
		},
		{
			name: "calls fn in order with the GTID set filled in",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			wantFn:      []string{"binlog.000001=" + uuidA + ":1-50", "binlog.000002=" + uuidA + ":51-100"},
			wantFetched: []string{"binlog.000001", "binlog.000002"},
		},
		{
			name: "skips empty and invalid binlogs",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", ""),
				binlog("binlog.000002", uuidA+":1-50"),
				binlog("binlog.000003", uuidA+":51-60"),
				binlog("binlog.000004", ""),
			},
			invalid:     map[string]bool{"binlog.000003": true},
			wantFn:      []string{"binlog.000002=" + uuidA + ":1-50"},
			wantFetched: []string{"binlog.000001", "binlog.000002", "binlog.000003", "binlog.000004"},
		},
		{
			// Binlogs handled before the failure were already uploaded and
			// recorded, and the ones after it must not even be looked at.
			name: "failure while fetching a set keeps earlier work and stops",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
				binlog("binlog.000003", uuidA+":101-150"),
			},
			failing:     map[string]error{"binlog.000002": errBoom},
			wantFn:      []string{"binlog.000001=" + uuidA + ":1-50"},
			wantFetched: []string{"binlog.000001", "binlog.000002"},
			wantErr:     true,
		},
		{
			name: "failure in fn stops without fetching the remaining sets",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
				binlog("binlog.000003", uuidA+":101-150"),
			},
			failFnOn:    "binlog.000002",
			wantFn:      []string{"binlog.000001=" + uuidA + ":1-50", "binlog.000002=" + uuidA + ":51-100"},
			wantFetched: []string{"binlog.000001", "binlog.000002"},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loc := &fakeLocator{binlogs: tt.binlogs, invalid: tt.invalid, failing: tt.failing}

			var gotFn []string
			err := forEachBinlogWithGTIDs(context.Background(), loc, tt.binlogs, func(b pxc.Binlog) error {
				gotFn = append(gotFn, b.Name+"="+b.GTIDSet.Raw())
				if b.Name == tt.failFnOn {
					return errBoom
				}
				return nil
			})

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantFn, gotFn)
			assert.Equal(t, tt.wantFetched, loc.fetched)
		})
	}
}
