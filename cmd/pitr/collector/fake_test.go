package collector

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util/gtid"
)

// fakeLocator mimics the binlog UDFs on a host: binlogs hold per-file GTID sets
// and a GTID is found in the first binlog containing it.
type fakeLocator struct {
	binlogs []pxc.Binlog
	invalid map[string]bool  // binlogs failing with MySQL error 3200
	failing map[string]error // binlogs failing with the given error
	fetched []string         // binlogs GetGTIDSet was called for, in order

	purged  string // GTIDs executed before the first binlog, like gtid_purged
	prevErr error  // returned by GetPreviousGTIDs

	subsetErr error // returned by GTIDSubset

	byGTIDErr error // returned by GetBinlogByGTID
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
	if f.byGTIDErr != nil {
		return "", f.byGTIDErr
	}
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

// fakeHost is a PXC host: the binlog UDFs of fakeLocator plus the calls needed to plan an upload.
type fakeHost struct {
	*fakeLocator
	name        string
	sourceID    string
	sourceIDErr error
	listErr     error

	gtidExecuted string
	executedErr  error

	listCalls int
	calls     []string // gtid_executed reads and binlog lists, in order
}

func (h *fakeHost) GetHost() string { return h.name }

func (h *fakeHost) WsrepClusterStateUUID() (string, error) {
	return h.sourceID, h.sourceIDErr
}

func (h *fakeHost) GetGTIDExecuted(context.Context) (string, error) {
	h.calls = append(h.calls, "gtid_executed")
	return h.gtidExecuted, h.executedErr
}

func (h *fakeHost) GetBinLogList(context.Context) ([]pxc.Binlog, error) {
	h.listCalls++
	h.calls = append(h.calls, "list")
	if h.listErr != nil {
		return nil, h.listErr
	}
	return h.binlogs, nil
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
