package collector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestFindResumePoint(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}

	tests := []struct {
		name      string
		binlogs   []pxc.Binlog
		invalid   map[string]bool
		byGTIDErr error
		stored    map[string]string // object name -> content
		sourceID  string
		want      resumePoint
		wantErr   bool
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
		{
			// Reporting a gap here would mark backups as not PITR-ready for a
			// failure that has nothing to do with the binlogs.
			name: "failure finding the binlog of the last uploaded GTID is an error, not a gap",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			byGTIDErr: errors.New("connection lost"),
			stored:    map[string]string{lastSetFilePrefix + uuidA: uuidA + ":51-100"},
			sourceID:  uuidA,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &Collector{storage: &fakeStorage{objects: tt.stored}}
			loc := &fakeLocator{binlogs: tt.binlogs, invalid: tt.invalid, byGTIDErr: tt.byGTIDErr}

			got, err := c.findResumePoint(context.Background(), loc, tt.sourceID, tt.binlogs)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
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

func TestLatestGTIDSets(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	binlog := func(name, gtidSet string) pxc.Binlog {
		return pxc.Binlog{Name: name, GTIDSet: pxc.NewGTIDSet(gtidSet)}
	}

	// a day without traffic with a binlog rotation every minute
	longEmptyTail := []pxc.Binlog{binlog("binlog.000001", uuidA+":1-50")}
	for i := 2; i <= 1441; i++ {
		longEmptyTail = append(longEmptyTail, binlog(fmt.Sprintf("binlog.%06d", i), ""))
	}

	// Only the UUIDs of the result are checked: they pick the markers to read.
	// The format of the ranges is up to the server.
	tests := []struct {
		name        string
		binlogs     []pxc.Binlog
		purged      string
		invalid     map[string]bool
		failing     map[string]error
		prevErr     error
		wantUUIDs   []string
		wantFetched []string
		wantErr     bool
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
			wantUUIDs:   []string{uuidA},
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
			wantUUIDs:   []string{uuidA},
			wantFetched: []string{"binlog.000004"},
		},
		{
			name:        "a long empty tail is not walked",
			binlogs:     longEmptyTail,
			wantUUIDs:   []string{uuidA},
			wantFetched: []string{"binlog.001441"},
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
			// The UUID of the older binlog sorts after the newer one, so it is
			// looked at last and has to be dropped, not just never picked.
			name: "uuid of an older binlog is left out",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidB+":1-25"),
				binlog("binlog.000002", uuidA+":1-50"),
				binlog("binlog.000003", ""),
			},
			wantUUIDs:   []string{uuidA},
			wantFetched: []string{"binlog.000003"},
		},
		{
			name: "every uuid of the most recent binlog with transactions",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-60,"+uuidB+":1-25"),
				binlog("binlog.000003", ""),
			},
			wantUUIDs:   []string{uuidA, uuidB},
			wantFetched: []string{"binlog.000003"},
		},
		{
			name: "purged uuid is left out",
			binlogs: []pxc.Binlog{
				binlog("binlog.000005", uuidB+":1-25"),
				binlog("binlog.000006", ""),
			},
			purged:      uuidA + ":1-50",
			wantUUIDs:   []string{uuidB},
			wantFetched: []string{"binlog.000006"},
		},
		{
			name: "invalid last binlog is treated as empty",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", uuidA+":1-50"),
				binlog("binlog.000002", uuidA+":51-100"),
			},
			invalid:     map[string]bool{"binlog.000002": true},
			wantUUIDs:   []string{uuidA},
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
			wantErr:     true,
		},
		{
			name: "GTID set of the last binlog can't be read",
			binlogs: []pxc.Binlog{
				binlog("binlog.000001", ""),
			},
			failing:     map[string]error{"binlog.000001": errors.New("connection lost")},
			wantFetched: []string{"binlog.000001"},
			wantErr:     true,
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
			got, err := latestGTIDSets(context.Background(), loc, tt.binlogs)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			var gotUUIDs []string
			for _, entry := range got {
				gotUUIDs = append(gotUUIDs, strings.TrimSpace(strings.Split(entry, ":")[0]))
			}
			assert.Equal(t, tt.wantUUIDs, gotUUIDs)
			assert.Equal(t, tt.wantFetched, loc.fetched)
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
	}{
		{
			name:         "binlog is exactly the uploaded set",
			binlogSet:    uuidA + ":51-100",
			lastUploaded: uuidA + ":51-100",
			want:         false,
		},
		{
			// Another node's binlogs split the range differently.
			name:         "binlog is inside the uploaded set",
			binlogSet:    uuidA + ":76-100",
			lastUploaded: uuidA + ":51-100",
			want:         false,
		},
		{
			// The last GTID (100) is uploaded but 40-50 are not, e.g. when the
			// binlog comes from another node that split the range differently.
			name:         "binlog starts before the uploaded range",
			binlogSet:    uuidA + ":40-100",
			lastUploaded: uuidA + ":51-100",
			want:         true,
		},
		{
			name:         "binlog goes beyond the uploaded set",
			binlogSet:    uuidA + ":51-120",
			lastUploaded: uuidA + ":51-100",
			want:         true,
		},
		{
			name:         "binlog from another uuid",
			binlogSet:    uuidB + ":1-10",
			lastUploaded: uuidA + ":1-100",
			want:         true,
		},
		{
			name:         "binlog covered by several uuids of the uploaded set",
			binlogSet:    uuidB + ":1-10",
			lastUploaded: uuidA + ":1-100," + uuidB + ":1-25",
			want:         false,
		},
		{
			name:         "binlog without transactions has nothing new",
			binlogSet:    "",
			lastUploaded: uuidA + ":1-100",
			want:         false,
		},
		{
			name:         "invalid binlog is skipped",
			binlogSet:    uuidA + ":1-5",
			lastUploaded: uuidA + ":1-100",
			invalid:      true,
			want:         false,
		},
		{
			name:         "failure getting the binlog set",
			binlogSet:    uuidA + ":1-5",
			lastUploaded: uuidA + ":1-100",
			setErr:       errBoom,
			wantErr:      true,
		},
		{
			name:         "failure comparing the sets",
			binlogSet:    uuidA + ":1-5",
			lastUploaded: uuidA + ":1-100",
			subsetErr:    errBoom,
			wantErr:      true,
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

			got, err := hasNewTransactions(context.Background(), loc, binlog, pxc.NewGTIDSet(tt.lastUploaded))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlanUpload(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	// Listing rotates the binlogs. gtid_executed must be read before that:
	// transactions committed after the read either end up in a binlog this
	// run uploads, or aren't in the remembered set, so the next run can't
	// wrongly skip them.
	tests := []struct {
		name      string
		binlogs   []pxc.Binlog
		executed  string
		wantCalls []string
	}{
		{
			name: "gtid_executed is read before the binlogs are rotated, which happens once",
			binlogs: []pxc.Binlog{
				{Name: "binlog.000001", GTIDSet: pxc.NewGTIDSet(uuidA + ":1-50")},
				{Name: "binlog.000002", GTIDSet: pxc.NewGTIDSet(uuidA + ":51-100")},
			},
			executed:  uuidA + ":1-100",
			wantCalls: []string{"gtid_executed", "list"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &Collector{storage: &fakeStorage{}}
			host := &fakeHost{
				fakeLocator:  &fakeLocator{binlogs: tt.binlogs},
				name:         "pxc-0",
				sourceID:     uuidA,
				gtidExecuted: tt.executed,
			}

			plan, err := c.planUpload(context.Background(), host)
			require.NoError(t, err)
			assert.Equal(t, tt.executed, plan.gtidExecuted)
			assert.Equal(t, tt.wantCalls, host.calls)
		})
	}
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
		binlogs      []pxc.Binlog
		listErr      error
		connectErr   error
		gtidExecuted string
		executedErr  error
		subsetErr    error
	}
	caughtUp := hostSpec{
		binlogs:      []pxc.Binlog{binlog("binlog.000001", uuidA+":1-50"), binlog("binlog.000002", uuidA+":51-100")},
		gtidExecuted: uuidA + ":1-100",
	}
	caughtUpWithNewWrites := hostSpec{
		binlogs:      append(slices.Clone(caughtUp.binlogs), binlog("binlog.000003", uuidA+":101")),
		gtidExecuted: uuidA + ":1-101",
	}
	idleRun := lastRun{host: prevHost, gtidExecuted: uuidA + ":1-100"}
	purged := hostSpec{binlogs: []pxc.Binlog{binlog("binlog.000005", uuidA+":101-150")}}
	noTransactions := hostSpec{binlogs: []pxc.Binlog{binlog("binlog.000001", ""), binlog("binlog.000002", "")}}
	other := hostSpec{binlogs: []pxc.Binlog{binlog("binlog.000001", uuidA+":1-100")}}

	tests := []struct {
		name       string
		prevHost   string  // host of the previous run, empty on the first run
		lastRun    lastRun // what the last successful run read
		prev       hostSpec
		other      hostSpec
		unhealthy  bool // the previous host is not Synced/Primary
		probeErr   error
		oldestHost string
		oldestErr  error

		wantErr        bool
		wantNothingNew bool           // the run is skipped, without listing (and rotating) binlogs
		wantHost       string         // host the plan was made on, or the host kept on error
		wantConnects   map[string]int // connections opened, checked only when set
		wantClosed     map[string]int // connections closed during selection
	}{
		{
			name:       "first run picks the oldest host",
			prevHost:   "",
			other:      other,
			oldestHost: otherHost,
			wantHost:   otherHost,
		},
		{
			name:       "healthy previous host with the marker is kept",
			prevHost:   prevHost,
			prev:       caughtUp,
			other:      other,
			oldestHost: otherHost,
			wantHost:   prevHost,
		},
		{
			name:         "unhealthy previous host is not even connected to",
			prevHost:     prevHost,
			prev:         caughtUp,
			other:        other,
			unhealthy:    true,
			oldestHost:   otherHost,
			wantHost:     otherHost,
			wantConnects: map[string]int{otherHost: 1},
		},
		{
			name:       "failed health probe falls back",
			prevHost:   prevHost,
			prev:       caughtUp,
			other:      other,
			probeErr:   errBoom,
			oldestHost: otherHost,
			wantHost:   otherHost,
		},
		{
			name:       "failed connection to the previous host falls back",
			prevHost:   prevHost,
			prev:       hostSpec{binlogs: caughtUp.binlogs, connectErr: errBoom},
			other:      other,
			oldestHost: otherHost,
			wantHost:   otherHost,
		},
		{
			name:       "previous host that purged the marker falls back and is closed",
			prevHost:   prevHost,
			prev:       purged,
			other:      other,
			oldestHost: otherHost,
			wantHost:   otherHost,
			wantClosed: map[string]int{prevHost: 1},
		},
		{
			name:       "previous host without transactions falls back and is closed",
			prevHost:   prevHost,
			prev:       noTransactions,
			other:      other,
			oldestHost: otherHost,
			wantHost:   otherHost,
			wantClosed: map[string]int{prevHost: 1},
		},
		{
			name:       "failure listing binlogs on the previous host falls back and is closed",
			prevHost:   prevHost,
			prev:       hostSpec{binlogs: caughtUp.binlogs, listErr: errBoom},
			other:      other,
			oldestHost: otherHost,
			wantHost:   otherHost,
			wantClosed: map[string]int{prevHost: 1},
		},
		{
			// The usual selection is not bound by the check that just failed.
			name:       "oldest host can be the previous host again",
			prevHost:   prevHost,
			prev:       purged,
			oldestHost: prevHost,
			wantHost:   prevHost,
			wantClosed: map[string]int{prevHost: 1},
		},
		{
			name:           "nothing new on the previous host skips the run",
			prevHost:       prevHost,
			lastRun:        idleRun,
			prev:           caughtUp,
			other:          other,
			oldestHost:     otherHost,
			wantNothingNew: true,
			wantHost:       prevHost,
		},
		{
			name:       "new transactions on the previous host are planned as usual",
			prevHost:   prevHost,
			lastRun:    idleRun,
			prev:       caughtUpWithNewWrites,
			other:      other,
			oldestHost: otherHost,
			wantHost:   prevHost,
		},
		{
			name:       "last run on another host is not compared",
			prevHost:   prevHost,
			lastRun:    lastRun{host: otherHost, gtidExecuted: uuidA + ":1-100"},
			prev:       caughtUp,
			other:      other,
			oldestHost: otherHost,
			wantHost:   prevHost,
		},
		{
			// its gtid_executed may not move while the rest of the cluster takes writes
			name:       "unhealthy previous host with nothing new is not skipped",
			prevHost:   prevHost,
			lastRun:    idleRun,
			prev:       caughtUp,
			other:      other,
			unhealthy:  true,
			oldestHost: otherHost,
			wantHost:   otherHost,
		},
		{
			name:       "failure getting gtid_executed on the previous host falls back and is closed",
			prevHost:   prevHost,
			lastRun:    idleRun,
			prev:       hostSpec{binlogs: caughtUp.binlogs, executedErr: errBoom},
			other:      other,
			oldestHost: otherHost,
			wantHost:   otherHost,
			wantClosed: map[string]int{prevHost: 1},
		},
		{
			// a failed check must not stop uploads: the run goes on as usual
			name:       "failure comparing gtid_executed with the last run does not skip",
			prevHost:   prevHost,
			lastRun:    idleRun,
			prev:       hostSpec{binlogs: caughtUp.binlogs, gtidExecuted: caughtUp.gtidExecuted, subsetErr: errBoom},
			other:      other,
			oldestHost: otherHost,
			wantHost:   prevHost,
		},
		{
			name:      "no host found",
			prevHost:  "",
			oldestErr: errBoom,
			wantErr:   true,
			wantHost:  "",
		},
		{
			name:       "cannot connect to the oldest host",
			prevHost:   prevHost,
			prev:       caughtUp,
			other:      hostSpec{connectErr: errBoom},
			unhealthy:  true,
			oldestHost: otherHost,
			wantErr:    true,
			wantHost:   prevHost,
		},
		{
			name:       "failure listing binlogs on the oldest host closes it",
			prevHost:   prevHost,
			prev:       caughtUp,
			other:      hostSpec{binlogs: other.binlogs, listErr: errBoom},
			unhealthy:  true,
			oldestHost: otherHost,
			wantErr:    true,
			wantHost:   prevHost,
			wantClosed: map[string]int{otherHost: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			specs := map[string]hostSpec{prevHost: tt.prev, otherHost: tt.other}
			created := map[string][]*fakeConn{}
			connects := map[string]int{}

			c := &Collector{
				storage: &fakeStorage{objects: map[string]string{
					lastSetFilePrefix + uuidA: uuidA + ":51-100",
				}},
				lastRun: tt.lastRun,
			}
			c.hosts = hostFuncs{
				isHealthy: func(_ context.Context, host string) (bool, error) {
					return !(tt.unhealthy && host == prevHost), tt.probeErr
				},
				connect: func(host string) (hostConn, error) {
					connects[host]++
					spec := specs[host]
					if spec.connectErr != nil {
						return nil, spec.connectErr
					}
					conn := &fakeConn{fakeHost: &fakeHost{
						fakeLocator: &fakeLocator{binlogs: spec.binlogs, subsetErr: spec.subsetErr},
						name:        host,
						sourceID:    uuidA,
						listErr:     spec.listErr,

						gtidExecuted: spec.gtidExecuted,
						executedErr:  spec.executedErr,
					}}
					created[host] = append(created[host], conn)
					return conn, nil
				},
				oldest: func(context.Context, string) (string, error) {
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
				assert.Equal(t, tt.wantHost, c.db.GetHost())
				assert.Equal(t, tt.wantNothingNew, plan.nothingNew)
				if tt.wantNothingNew {
					assert.Empty(t, plan.binlogs)
					assert.Zero(t, created[tt.wantHost][0].listCalls, "binlogs are not listed, so not rotated")
				} else {
					assert.Equal(t, specs[tt.wantHost].binlogs, plan.binlogs, "plan is made on the picked host")
					assert.Equal(t, specs[tt.wantHost].gtidExecuted, plan.gtidExecuted)
				}
			}

			if tt.wantConnects != nil {
				assert.Equal(t, tt.wantConnects, connects, "connections opened")
			}

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

func TestRunSkipsWhenNothingNew(t *testing.T) {
	t.Parallel()

	const uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const host = "pxc-0"

	// Everything on the host was already uploaded, so a run uploads nothing.
	// Runs that do upload need mysqlbinlog, so they are not covered here; that
	// a run with new transactions isn't skipped is covered by TestPickHost.
	binlogs := []pxc.Binlog{
		{Name: "binlog.000001", GTIDSet: pxc.NewGTIDSet(uuidA + ":1-50")},
		{Name: "binlog.000002", GTIDSet: pxc.NewGTIDSet(uuidA + ":51-100")},
	}

	tests := []struct {
		name     string
		executed string // gtid_executed of the host, the same in both runs
	}{
		{
			name:     "idle cluster",
			executed: uuidA + ":1-100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var conns []*fakeConn
			c := &Collector{storage: &fakeStorage{objects: map[string]string{
				lastSetFilePrefix + uuidA: uuidA + ":51-100",
			}}}
			c.hosts = hostFuncs{
				isHealthy: func(context.Context, string) (bool, error) { return true, nil },
				connect: func(name string) (hostConn, error) {
					conn := &fakeConn{fakeHost: &fakeHost{
						fakeLocator:  &fakeLocator{binlogs: binlogs},
						name:         name,
						sourceID:     uuidA,
						gtidExecuted: tt.executed,
					}}
					conns = append(conns, conn)
					return conn, nil
				},
				oldest: func(context.Context, string) (string, error) { return host, nil },
			}

			require.NoError(t, c.Run(t.Context()))
			require.Len(t, conns, 1)
			assert.Equal(t, 1, conns[0].listCalls, "the first run is never skipped")
			assert.Equal(t, lastRun{host: host, gtidExecuted: tt.executed}, c.lastRun)

			require.NoError(t, c.Run(t.Context()))
			require.Len(t, conns, 2)
			assert.Zero(t, conns[1].listCalls, "the second run is skipped without rotating the binlogs")
			assert.True(t, conns[1].closed, "the connection is closed after the skipped run")
		})
	}
}
