package collector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
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

	purged  string // GTIDs executed before the first binlog, like gtid_purged
	prevErr error  // returned by GetPreviousGTIDs

	subsetErr   error // returned by GTIDSubset
	subsetCalls int
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

// GTIDSubset mimics GTID_SUBSET() for sets made of "uuid:from-to" entries.
func (f *fakeLocator) GTIDSubset(_ context.Context, set1, set2 string) (bool, error) {
	f.subsetCalls++
	if f.subsetErr != nil {
		return false, f.subsetErr
	}

	other, err := gtid.New(set2)
	if err != nil {
		return false, err
	}

	for _, entry := range strings.Split(set1, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		for _, interval := range parts[1:] {
			from, to, found := strings.Cut(interval, "-")
			if !found {
				to = from
			}
			lo, err := strconv.ParseInt(from, 10, 64)
			if err != nil {
				return false, err
			}
			hi, err := strconv.ParseInt(to, 10, 64)
			if err != nil {
				return false, err
			}
			for seq := lo; seq <= hi; seq++ {
				if !other.ContainsSeq(parts[0], seq) {
					return false, nil
				}
			}
		}
	}
	return true, nil
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

// GetPreviousGTIDs mimics the Previous_gtids event: the purged GTIDs plus the
// sets of all earlier binlogs, with the intervals of a UUID in a single entry.
func (f *fakeLocator) GetPreviousGTIDs(_ context.Context, name string) (string, error) {
	if f.prevErr != nil {
		return "", f.prevErr
	}

	i := slices.IndexFunc(f.binlogs, func(b pxc.Binlog) bool { return b.Name == name })
	if i < 0 {
		return "", errors.New("Binary log does not exist")
	}

	sets := []string{f.purged}
	for _, b := range f.binlogs[:i] {
		sets = append(sets, b.GTIDSet.Raw())
	}

	intervals := map[string][]string{}
	for _, set := range sets {
		for entry := range strings.SplitSeq(set, ",") {
			uuid, rest, found := strings.Cut(strings.TrimSpace(entry), ":")
			if found {
				intervals[uuid] = append(intervals[uuid], rest)
			}
		}
	}

	var entries []string
	for uuid, list := range intervals {
		entries = append(entries, uuid+":"+strings.Join(list, ":"))
	}
	slices.Sort(entries)
	return strings.Join(entries, ","), nil
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
			err := forEachBinlogWithGTIDs(context.Background(), loc, tt.binlogs, &emptyBinlogs{}, func(b pxc.Binlog) error {
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

func TestEmptyBinlogsAreNotAskedAgain(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	binlog := func(name string, size int64, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, Size: size, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}

	// every run mirrors one collector run: the host in use, the binlog list the
	// server returned, and which binlogs are expected to be asked about.
	type run struct {
		host        string
		binlogs     []pxc.Binlog
		invalid     map[string]bool
		wantFetched []string
	}

	tests := []struct {
		name string
		runs []run
	}{
		{
			name: "idle cluster only asks about new binlogs",
			runs: []run{
				{
					host: "pxc-0",
					binlogs: []pxc.Binlog{
						binlog("binlog.000001", 100, uuidA+":1-50"),
						binlog("binlog.000002", 50, ""),
						binlog("binlog.000003", 50, ""),
					},
					wantFetched: []string{"binlog.000001", "binlog.000002", "binlog.000003"},
				},
				{
					host: "pxc-0",
					binlogs: []pxc.Binlog{
						binlog("binlog.000001", 100, uuidA+":1-50"),
						binlog("binlog.000002", 50, ""),
						binlog("binlog.000003", 50, ""),
						binlog("binlog.000004", 50, ""),
					},
					wantFetched: []string{"binlog.000001", "binlog.000004"},
				},
			},
		},
		{
			name: "binlog with transactions is always asked about",
			runs: []run{
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 100, uuidA+":1-50")},
					wantFetched: []string{"binlog.000001"},
				},
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 100, uuidA+":1-50")},
					wantFetched: []string{"binlog.000001"},
				},
			},
		},
		{
			name: "binlog whose size changed is asked about again",
			runs: []run{
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 50, "")},
					wantFetched: []string{"binlog.000001"},
				},
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 200, uuidA+":1-5")},
					wantFetched: []string{"binlog.000001"},
				},
			},
		},
		{
			name: "everything is asked about again after the host changes",
			runs: []run{
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 50, "")},
					wantFetched: []string{"binlog.000001"},
				},
				{
					host:        "pxc-1",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 50, "")},
					wantFetched: []string{"binlog.000001"},
				},
			},
		},
		{
			name: "purged binlogs are forgotten",
			runs: []run{
				{
					host: "pxc-0",
					binlogs: []pxc.Binlog{
						binlog("binlog.000001", 50, ""),
						binlog("binlog.000002", 50, ""),
					},
					wantFetched: []string{"binlog.000001", "binlog.000002"},
				},
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000002", 50, "")},
					wantFetched: nil,
				},
				{
					host: "pxc-0",
					binlogs: []pxc.Binlog{
						binlog("binlog.000001", 50, ""),
						binlog("binlog.000002", 50, ""),
					},
					wantFetched: []string{"binlog.000001"},
				},
			},
		},
		{
			name: "invalid binlogs are asked about again",
			runs: []run{
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 50, uuidA+":1-5")},
					invalid:     map[string]bool{"binlog.000001": true},
					wantFetched: []string{"binlog.000001"},
				},
				{
					host:        "pxc-0",
					binlogs:     []pxc.Binlog{binlog("binlog.000001", 50, uuidA+":1-5")},
					invalid:     map[string]bool{"binlog.000001": true},
					wantFetched: []string{"binlog.000001"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			empties := &emptyBinlogs{}
			for i, r := range tt.runs {
				empties.use(r.host)
				empties.retain(r.binlogs)

				loc := &fakeLocator{binlogs: r.binlogs, invalid: r.invalid}
				err := forEachBinlogWithGTIDs(context.Background(), loc, r.binlogs, empties, func(pxc.Binlog) error { return nil })
				require.NoError(t, err)
				assert.Equal(t, r.wantFetched, loc.fetched, "run %d", i+1)
			}
		})
	}
}

func TestLatestGTIDSets(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}

	tests := []struct {
		name        string
		binlogs     []pxc.Binlog
		purged      string
		invalid     map[string]bool
		failing     map[string]error
		prevErr     error
		want        []string
		wantFetched []string
		wantErr     string
	}{
		{
			name: "no binlogs",
		},
		{
			name: "last binlog has transactions",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			want:        []string{uuidA + ":51-100"},
			wantFetched: []string{"binlog.000002"},
		},
		{
			name: "empty binlogs at the end are not walked",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
				binlog("binlog.000003", ""),
				binlog("binlog.000004", ""),
			},
			want:        []string{uuidA + ":1-50:51-100"},
			wantFetched: []string{"binlog.000004"},
		},
		{
			name: "only purged transactions",
			binlogs: []pxc.Binlog{
				binlog("binlog.000005", ""),
				binlog("binlog.000006", ""),
			},
			purged:      uuidA + ":1-50",
			wantFetched: []string{"binlog.000006"},
		},
		{
			name: "no transactions at all",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", ""),
				binlog("binlog.000002", ""),
			},
			wantFetched: []string{"binlog.000002"},
		},
		{
			name: "uuid of an older binlog is left out",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidB+":1-25"),
				binlog("binlog.000003", ""),
			},
			want:        []string{uuidB + ":1-25"},
			wantFetched: []string{"binlog.000003"},
		},
		{
			name: "every uuid of the most recent binlog with transactions",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-60,"+uuidB+":1-25"),
				binlog("binlog.000003", ""),
			},
			want:        []string{uuidA + ":1-50:51-60", uuidB + ":1-25"},
			wantFetched: []string{"binlog.000003"},
		},
		{
			name: "purged uuid is left out",
			binlogs: []pxc.Binlog{
				binlog("binlog.000005", uuidB+":1-25"),
				binlog("binlog.000006", ""),
			},
			purged:      uuidA + ":1-50",
			want:        []string{uuidB + ":1-25"},
			wantFetched: []string{"binlog.000006"},
		},
		{
			name: "invalid last binlog is treated as empty",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			invalid:     map[string]bool{"binlog.000002": true},
			want:        []string{uuidA + ":1-50"},
			wantFetched: []string{"binlog.000002"},
		},
		{
			name: "previous GTIDs can't be read",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", ""),
			},
			prevErr:     errors.New("connection lost"),
			wantFetched: []string{"binlog.000002"},
			wantErr:     "get previous GTIDs of binlog.000002: connection lost",
		},
		{
			name: "GTID set of the last binlog can't be read",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", ""),
			},
			failing:     map[string]error{"binlog.000001": errors.New("connection lost")},
			wantFetched: []string{"binlog.000001"},
			wantErr:     "get GTID set of binlog.000001: connection lost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loc := &fakeLocator{
				binlogs: tt.binlogs,
				purged:  tt.purged,
				invalid: tt.invalid,
				failing: tt.failing,
				prevErr: tt.prevErr,
			}
			empties := &emptyBinlogs{}
			empties.use("pxc-0")

			got, err := latestGTIDSets(context.Background(), loc, tt.binlogs, empties)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantFetched, loc.fetched)
		})
	}
}

func TestFindResumePointOnIdleCluster(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	withTx := pxc.Binlog{Name: "binlog.000001", Size: 100, GTIDSet: pxc.NewGTIDSet(uuidA + ":1-50")}
	empty := func(n string) pxc.Binlog { return pxc.Binlog{Name: "binlog." + n, Size: 50} }

	// n empty binlogs numbered after the first one
	emptyTail := func(n int) []pxc.Binlog {
		var tail []pxc.Binlog
		for i := 2; i < n+2; i++ {
			tail = append(tail, empty(fmt.Sprintf("%06d", i)))
		}
		return tail
	}

	tests := []struct {
		name       string
		binlogs    []pxc.Binlog
		wantFirst  []string
		wantSecond []string
	}{
		{
			name:       "no empty binlogs behind the last upload",
			binlogs:    []pxc.Binlog{withTx},
			wantFirst:  []string{"binlog.000001"},
			wantSecond: []string{"binlog.000001"},
		},
		{
			name:       "empty binlogs are not walked",
			binlogs:    []pxc.Binlog{withTx, empty("000002"), empty("000003"), empty("000004")},
			wantFirst:  []string{"binlog.000004"},
			wantSecond: nil,
		},
		{
			// A day without traffic with a binlog rotation every minute.
			name:       "a long empty tail is not walked",
			binlogs:    append([]pxc.Binlog{withTx}, emptyTail(1440)...),
			wantFirst:  []string{"binlog.001441"},
			wantSecond: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &Collector{storage: &fakeStorage{objects: map[string]string{
				lastSetFilePrefix + uuidA: uuidA + ":1-50",
			}}}
			c.emptyBinlogs.use("pxc-0")

			for i, wantFetched := range [][]string{tt.wantFirst, tt.wantSecond} {
				loc := &fakeLocator{binlogs: tt.binlogs}
				rp, err := c.findResumePoint(context.Background(), loc, uuidA, tt.binlogs)
				require.NoError(t, err)
				assert.Equal(t, resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-50"), start: 0}, rp)
				assert.Equal(t, wantFetched, loc.fetched, "run %d", i+1)
			}
		})
	}
}

func TestHasNewTransactions(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	errBoom := errors.New("boom")

	tests := []struct {
		name         string
		binlogSet    string
		lastUploaded string
		invalid      bool
		setErr       error
		subsetErr    error
		want         bool
		wantErr      bool
		wantSubset   bool // GTIDSubset is expected to be asked
	}{
		{
			name:         "binlog is exactly the uploaded set",
			binlogSet:    uuidA + ":51-100",
			lastUploaded: uuidA + ":51-100",
			want:         false,
			wantSubset:   true,
		},
		{
			// Another node's binlogs split the range differently.
			name:         "binlog is inside the uploaded set",
			binlogSet:    uuidA + ":76-100",
			lastUploaded: uuidA + ":51-100",
			want:         false,
			wantSubset:   true,
		},
		{
			// The last GTID (100) is uploaded but 40-50 are not. Looking only at
			// the last GTID of the binlog would wrongly skip it.
			name:         "binlog starts before the uploaded range",
			binlogSet:    uuidA + ":40-100",
			lastUploaded: uuidA + ":51-100",
			want:         true,
			wantSubset:   true,
		},
		{
			name:         "binlog goes beyond the uploaded set",
			binlogSet:    uuidA + ":51-120",
			lastUploaded: uuidA + ":51-100",
			want:         true,
			wantSubset:   true,
		},
		{
			name:         "binlog from another uuid",
			binlogSet:    uuidB + ":1-10",
			lastUploaded: uuidA + ":1-100",
			want:         true,
			wantSubset:   true,
		},
		{
			name:         "binlog covered by several uuids of the uploaded set",
			binlogSet:    uuidB + ":1-10",
			lastUploaded: uuidA + ":1-100," + uuidB + ":1-25",
			want:         false,
			wantSubset:   true,
		},
		{
			name:         "binlog without transactions has nothing new",
			binlogSet:    "",
			lastUploaded: uuidA + ":1-100",
			want:         false,
			wantSubset:   false,
		},
		{
			name:         "invalid binlog is skipped",
			binlogSet:    uuidA + ":1-5",
			lastUploaded: uuidA + ":1-100",
			invalid:      true,
			want:         false,
			wantSubset:   false,
		},
		{
			name:         "failure getting the binlog set",
			binlogSet:    uuidA + ":1-5",
			lastUploaded: uuidA + ":1-100",
			setErr:       errBoom,
			wantErr:      true,
			wantSubset:   false,
		},
		{
			name:         "failure comparing the sets",
			binlogSet:    uuidA + ":1-5",
			lastUploaded: uuidA + ":1-100",
			subsetErr:    errBoom,
			wantErr:      true,
			wantSubset:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binlog := pxc.Binlog{Name: "binlog.000002", GTIDSet: pxc.NewGTIDSet(tt.binlogSet)}
			loc := &fakeLocator{
				binlogs:   []pxc.Binlog{binlog},
				failing:   map[string]error{binlog.Name: tt.setErr},
				subsetErr: tt.subsetErr,
			}
			if tt.invalid {
				loc.invalid = map[string]bool{binlog.Name: true}
			}

			got, err := hasNewTransactions(context.Background(), loc, binlog, pxc.NewGTIDSet(tt.lastUploaded), &emptyBinlogs{})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantSubset, loc.subsetCalls > 0, "GTIDSubset called")
			assert.Equal(t, []string{binlog.Name}, loc.fetched, "only the binlog headers are read")
		})
	}
}

// fakeHost is a PXC host: the binlog UDFs of fakeLocator plus the calls needed to plan an upload.
type fakeHost struct {
	*fakeLocator
	name        string
	sourceID    string
	sourceIDErr error
	listErr     error

	sourceIDCalls int
	listCalls     int
}

func (h *fakeHost) GetHost() string { return h.name }

func (h *fakeHost) WsrepClusterStateUUID() (string, error) {
	h.sourceIDCalls++
	return h.sourceID, h.sourceIDErr
}

func (h *fakeHost) GetBinLogList(context.Context) ([]pxc.Binlog, error) {
	h.listCalls++
	if h.listErr != nil {
		return nil, h.listErr
	}
	return h.binlogs, nil
}

func TestPlanUpload(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}
	errBoom := errors.New("boom")

	tests := []struct {
		name        string
		binlogs     []pxc.Binlog
		stored      map[string]string
		sourceIDErr error
		listErr     error
		want        resumePoint
		wantSafe    bool
		wantErr     bool
	}{
		{
			name: "caught up on the host",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
				binlog("binlog.000003", ""),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":51-100"},
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":51-100"), start: 1},
			wantSafe: true,
		},
		{
			name:     "nothing uploaded yet",
			binlogs:  []pxc.Binlog{binlog("binlog.000001", uuidA+":1-50")},
			want:     resumePoint{},
			wantSafe: true,
		},
		{
			name: "last uploaded GTID is gone from the host",
			binlogs: []pxc.Binlog{
				binlog("binlog.000005", uuidA+":101-150"),
				binlog("binlog.000006", uuidA+":151-200"),
			},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":1-60"},
			want:     resumePoint{lastUploaded: pxc.NewGTIDSet(uuidA + ":1-60"), gap: true},
			wantSafe: false,
		},
		{
			name:     "host has no binlogs with transactions",
			binlogs:  []pxc.Binlog{binlog("binlog.000001", ""), binlog("binlog.000002", "")},
			stored:   map[string]string{lastSetFilePrefix + uuidA: uuidA + ":1-60"},
			want:     resumePoint{empty: true},
			wantSafe: false,
		},
		{
			name:    "failure listing binlogs",
			binlogs: []pxc.Binlog{binlog("binlog.000001", uuidA+":1-50")},
			listErr: errBoom,
			wantErr: true,
		},
		{
			name:        "failure getting the cluster uuid",
			binlogs:     []pxc.Binlog{binlog("binlog.000001", uuidA+":1-50")},
			sourceIDErr: errBoom,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &Collector{storage: &fakeStorage{objects: tt.stored}}
			host := &fakeHost{
				fakeLocator: &fakeLocator{binlogs: tt.binlogs},
				name:        "pxc-0",
				sourceID:    uuidA,
				sourceIDErr: tt.sourceIDErr,
				listErr:     tt.listErr,
			}

			plan, err := c.planUpload(context.Background(), host)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, plan)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.binlogs, plan.binlogs)
			assert.Equal(t, tt.want, plan.resume)
			assert.Equal(t, tt.wantSafe, plan.safeToKeepHost())

			// the point of planning once: the host is listed, and asked for its
			// cluster uuid, a single time, and that's all the upload needs
			assert.Equal(t, 1, host.listCalls, "binlog list calls")
			assert.Equal(t, 1, host.sourceIDCalls, "cluster uuid calls")
			assert.Equal(t, "pxc-0", c.emptyBinlogs.host)
		})
	}
}

// fakeConn is a connection to a PXC host, tracking whether it was closed.
type fakeConn struct {
	*fakeHost
	closed bool
}

func (c *fakeConn) GetBinLogFirstTimestamp(context.Context, string) (string, error) { return "", nil }
func (c *fakeConn) GetBinLogLastTimestamp(context.Context, string) (string, error)  { return "", nil }

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func TestPickHost(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const prevHost = "pxc-0"
	const otherHost = "pxc-1"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}
	errBoom := errors.New("boom")

	// the last uploaded set in storage is uuidA:51-100
	type hostSpec struct {
		binlogs    []pxc.Binlog
		listErr    error
		connectErr error
	}
	caughtUp := hostSpec{binlogs: []pxc.Binlog{binlog("binlog.000001", uuidA+":1-50"), binlog("binlog.000002", uuidA+":51-100")}}
	purged := hostSpec{binlogs: []pxc.Binlog{binlog("binlog.000005", uuidA+":101-150")}}
	noTransactions := hostSpec{binlogs: []pxc.Binlog{binlog("binlog.000001", ""), binlog("binlog.000002", "")}}
	other := hostSpec{binlogs: []pxc.Binlog{binlog("binlog.000001", uuidA+":1-100")}}

	tests := []struct {
		name       string
		prevHost   string // host of the previous run, empty on the first run
		prev       hostSpec
		other      hostSpec
		unhealthy  bool
		probeErr   error
		oldestHost string
		oldestErr  error

		wantErr      bool
		wantHost     string // host the plan was made on, or the host kept on error
		wantProbes   int
		wantOldest   int
		wantConnects map[string]int
		wantClosed   map[string]int // connections closed during selection
	}{
		{
			name:         "first run picks the oldest host",
			prevHost:     "",
			other:        other,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantProbes:   0,
			wantOldest:   1,
			wantConnects: map[string]int{otherHost: 1},
		},
		{
			name:         "healthy previous host with the marker is kept",
			prevHost:     prevHost,
			prev:         caughtUp,
			other:        other,
			oldestHost:   otherHost,
			wantHost:     prevHost,
			wantProbes:   1,
			wantOldest:   0,
			wantConnects: map[string]int{prevHost: 1},
		},
		{
			name:         "unhealthy previous host is not even connected to",
			prevHost:     prevHost,
			prev:         caughtUp,
			other:        other,
			unhealthy:    true,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{otherHost: 1},
		},
		{
			name:         "failed health probe falls back",
			prevHost:     prevHost,
			prev:         caughtUp,
			other:        other,
			probeErr:     errBoom,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{otherHost: 1},
		},
		{
			name:         "failed connection to the previous host falls back",
			prevHost:     prevHost,
			prev:         hostSpec{binlogs: caughtUp.binlogs, connectErr: errBoom},
			other:        other,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{prevHost: 1, otherHost: 1},
		},
		{
			name:         "previous host that purged the marker falls back and is closed",
			prevHost:     prevHost,
			prev:         purged,
			other:        other,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{prevHost: 1, otherHost: 1},
			wantClosed:   map[string]int{prevHost: 1},
		},
		{
			name:         "previous host without transactions falls back and is closed",
			prevHost:     prevHost,
			prev:         noTransactions,
			other:        other,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{prevHost: 1, otherHost: 1},
			wantClosed:   map[string]int{prevHost: 1},
		},
		{
			name:         "failure listing binlogs on the previous host falls back and is closed",
			prevHost:     prevHost,
			prev:         hostSpec{binlogs: caughtUp.binlogs, listErr: errBoom},
			other:        other,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{prevHost: 1, otherHost: 1},
			wantClosed:   map[string]int{prevHost: 1},
		},
		{
			// The usual selection is not bound by the check that just failed.
			name:         "oldest host can be the previous host again",
			prevHost:     prevHost,
			prev:         purged,
			oldestHost:   prevHost,
			wantHost:     prevHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{prevHost: 2},
			wantClosed:   map[string]int{prevHost: 1},
		},
		{
			name:         "no host found",
			prevHost:     "",
			oldestErr:    errBoom,
			wantErr:      true,
			wantHost:     "",
			wantProbes:   0,
			wantOldest:   1,
			wantConnects: map[string]int{},
		},
		{
			name:         "cannot connect to the oldest host",
			prevHost:     prevHost,
			prev:         caughtUp,
			other:        hostSpec{connectErr: errBoom},
			unhealthy:    true,
			oldestHost:   otherHost,
			wantErr:      true,
			wantHost:     prevHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{otherHost: 1},
		},
		{
			name:         "failure listing binlogs on the oldest host closes it",
			prevHost:     prevHost,
			prev:         caughtUp,
			other:        hostSpec{binlogs: other.binlogs, listErr: errBoom},
			unhealthy:    true,
			oldestHost:   otherHost,
			wantErr:      true,
			wantHost:     prevHost,
			wantProbes:   1,
			wantOldest:   1,
			wantConnects: map[string]int{otherHost: 1},
			wantClosed:   map[string]int{otherHost: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			specs := map[string]hostSpec{prevHost: tt.prev, otherHost: tt.other}
			created := map[string][]*fakeConn{}
			connects := map[string]int{}
			var probes, oldestCalls int
			var oldestPrevHost string

			c := &Collector{storage: &fakeStorage{objects: map[string]string{
				lastSetFilePrefix + uuidA: uuidA + ":51-100",
			}}}
			c.hosts = hostFuncs{
				isHealthy: func(context.Context, string) (bool, error) {
					probes++
					return !tt.unhealthy, tt.probeErr
				},
				connect: func(host string) (hostConn, error) {
					connects[host]++
					spec := specs[host]
					if spec.connectErr != nil {
						return nil, spec.connectErr
					}
					conn := &fakeConn{fakeHost: &fakeHost{
						fakeLocator: &fakeLocator{binlogs: spec.binlogs},
						name:        host,
						sourceID:    uuidA,
						listErr:     spec.listErr,
					}}
					created[host] = append(created[host], conn)
					return conn, nil
				},
				oldest: func(_ context.Context, prev string) (string, error) {
					oldestCalls++
					oldestPrevHost = prev
					return tt.oldestHost, tt.oldestErr
				},
			}

			// the connection of the previous run, already closed by Run
			var previous hostConn
			if tt.prevHost != "" {
				previous = &fakeConn{fakeHost: &fakeHost{fakeLocator: &fakeLocator{}, name: tt.prevHost}}
				c.db = previous
			}

			plan, err := c.pickHost(t.Context())

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, plan)
				assert.Equal(t, previous, c.db, "the connection is left as it was")
			} else {
				require.NoError(t, err)
				require.NotNil(t, plan)
				assert.Equal(t, specs[tt.wantHost].binlogs, plan.binlogs, "plan is made on the picked host")
				assert.Equal(t, tt.wantHost, c.db.GetHost())
			}

			assert.Equal(t, tt.wantProbes, probes, "health probes")
			assert.Equal(t, tt.wantOldest, oldestCalls, "oldest host lookups")
			if oldestCalls > 0 {
				assert.Equal(t, tt.prevHost, oldestPrevHost, "oldest host lookup is told the previous host")
			}
			assert.Equal(t, tt.wantConnects, connects, "connections opened")

			closed := map[string]int{}
			for host, conns := range created {
				for _, conn := range conns {
					if conn.closed {
						closed[host]++
					}
				}
			}
			wantClosed := tt.wantClosed
			if wantClosed == nil {
				wantClosed = map[string]int{}
			}
			assert.Equal(t, wantClosed, closed, "connections closed")
		})
	}
}
