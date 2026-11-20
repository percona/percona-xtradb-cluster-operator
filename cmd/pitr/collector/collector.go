package collector

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util/gtid"
)

const collectorPasswordPath = "/etc/mysql/mysql-users-secret/xtrabackup"

var (
	pxcBinlogCollectorBackupSuccess = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pxc_binlog_collector_success_total",
			Help: "Total number of successful binlog collection cycles",
		},
	)
	pxcBinlogCollectorBackupFailure = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pxc_binlog_collector_failure_total",
			Help: "Total number of failed binlog collection cycles",
		},
	)
	pxcBinlogCollectorUploadedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pxc_binlog_collector_uploaded_total",
			Help: "Total number of successfully uploaded binlogs",
		},
	)
	pxcBinlogCollectorLastProcessingTime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pxc_binlog_collector_last_processing_timestamp",
			Help: "Timestamp of the last successful binlog processing",
		},
	)
	pxcBinlogCollectorLastUploadTime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "pxc_binlog_collector_last_upload_timestamp",
			Help: "Timestamp of the last successful binlog upload",
		},
	)
	pxcBinlogCollectorGapDetected = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "pxc_binlog_collector_gap_detected_total",
			Help: "Total number of times the gap was detected in binlog",
		},
	)
	pxcBinlogCollectorSourceHost = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pxc_binlog_collector_source_host",
			Help: "PXC host the collector reads binlogs from. The series of the host in use has the value 1, there is only one at a time",
		},
		[]string{"host"},
	)
)

func init() {
	prometheus.MustRegister(pxcBinlogCollectorBackupSuccess)
	prometheus.MustRegister(pxcBinlogCollectorBackupFailure)
	prometheus.MustRegister(pxcBinlogCollectorLastProcessingTime)
	prometheus.MustRegister(pxcBinlogCollectorLastUploadTime)
	prometheus.MustRegister(pxcBinlogCollectorGapDetected)
	prometheus.MustRegister(pxcBinlogCollectorUploadedTotal)
	prometheus.MustRegister(pxcBinlogCollectorSourceHost)
}

type Collector struct {
	db              hostConn
	hosts           hostFuncs
	storage         storage.Storage
	lastUploadedSet pxc.GTIDSet  // last uploaded binary logs set
	pxcServiceName  string       // k8s service name for PXC, its for get correct host for connection
	pxcUser         string       // user for connection to PXC
	pxcPass         string       // password for connection to PXC
	emptyBinlogs    emptyBinlogs // closed binlogs known to have no transactions
}

type Config struct {
	PXCServiceName     string `env:"PXC_SERVICE,required"`
	PXCUser            string `env:"PXC_USER,required"`
	PXCPass            string `env:"PXC_PASS,required"`
	StorageType        string `env:"STORAGE_TYPE,required"`
	BackupStorageS3    BackupS3
	BackupStorageAzure BackupAzure
	BufferSize         int64   `env:"BUFFER_SIZE"`
	CollectSpanSec     float64 `env:"COLLECT_SPAN_SEC" envDefault:"60"`
	VerifyTLS          bool    `env:"VERIFY_TLS" envDefault:"true"`
	TimeoutSeconds     float64 `env:"TIMEOUT_SECONDS" envDefault:"60"`
	GTIDCacheKey       string  `env:"GTID_CACHE_KEY"` // deprecated, the GTID cache is not used anymore
}

type BackupS3 struct {
	Endpoint              string `env:"ENDPOINT" envDefault:"s3.amazonaws.com"`
	AccessKeyID           string `env:"ACCESS_KEY_ID,required"`
	AccessKey             string `env:"SECRET_ACCESS_KEY,required"`
	SessionToken          string `env:"S3_SESSION_TOKEN"`
	BucketURL             string `env:"S3_BUCKET_URL,required"`
	Region                string `env:"DEFAULT_REGION,required"`
	ForcePath             bool   `env:"S3_FORCE_PATH"`
	ChecksumAlgorithm     string `env:"S3_CHECKSUM_ALGORITHM"`
	SkipBucketExistsCheck bool   `env:"S3_SKIP_BUCKET_EXISTS_CHECK"`
}

type BackupAzure struct {
	Endpoint      string `env:"AZURE_ENDPOINT,required"`
	ContainerPath string `env:"AZURE_CONTAINER_PATH,required"`
	StorageClass  string `env:"AZURE_STORAGE_CLASS"`
	AccountName   string `env:"AZURE_STORAGE_ACCOUNT,required"`
	AccountKey    string `env:"AZURE_ACCESS_KEY,required"`
	BlockSize     int64  `env:"AZURE_BLOCK_SIZE"`
	Concurrency   int    `env:"AZURE_CONCURRENCY"`
}

const (
	lastSetFilePrefix string = "last-binlog-set-" // filename prefix for object where the last binlog set will be stored
	gtidPostfix       string = "-gtid-set"        // filename postfix for files with GTID set
)

func New(ctx context.Context, c Config) (*Collector, error) {
	var s storage.Storage
	var err error
	switch c.StorageType {
	case "s3":
		bucketArr := strings.Split(c.BackupStorageS3.BucketURL, "/")
		prefix := ""
		// if c.S3BucketURL looks like "my-bucket/data/more-data" we need prefix to be "data/more-data/"
		if len(bucketArr) > 1 {
			prefix = strings.TrimPrefix(c.BackupStorageS3.BucketURL, bucketArr[0]+"/") + "/"
		}
		// if c.S3BucketURL ends with "/", we need prefix to be like "data/more-data/", not "data/more-data//"
		prefix = path.Clean(prefix) + "/"

		// try to read the S3 CA bundle
		caBundle, err := os.ReadFile(path.Join(naming.BackupStorageCAFileDirectory, naming.BackupStorageCAFileName))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, errors.Wrap(err, "read CA bundle file")
		}

		opts := storage.S3Options{
			Endpoint:              c.BackupStorageS3.Endpoint,
			AccessKeyID:           c.BackupStorageS3.AccessKeyID,
			SecretAccessKey:       c.BackupStorageS3.AccessKey,
			SessionToken:          c.BackupStorageS3.SessionToken,
			BucketName:            bucketArr[0],
			Prefix:                prefix,
			Region:                c.BackupStorageS3.Region,
			VerifyTLS:             c.VerifyTLS,
			CABundle:              caBundle,
			ForcePathStyle:        c.BackupStorageS3.ForcePath,
			ChecksumAlgorithm:     api.S3ChecksumAlgorithmType(c.BackupStorageS3.ChecksumAlgorithm),
			SkipBucketExistsCheck: c.BackupStorageS3.SkipBucketExistsCheck,
		}
		s, err = storage.NewS3(ctx, opts)
		if err != nil {
			return nil, errors.Wrap(err, "new storage manager")
		}
	case "azure":
		container, prefix, _ := strings.Cut(c.BackupStorageAzure.ContainerPath, "/")
		if prefix != "" {
			prefix += "/"
		}
		prefix = path.Clean(prefix) + "/"
		s, err = storage.NewAzure(c.BackupStorageAzure.AccountName, c.BackupStorageAzure.AccountKey, c.BackupStorageAzure.Endpoint, container, prefix, c.BackupStorageAzure.BlockSize, c.BackupStorageAzure.Concurrency)
		if err != nil {
			return nil, errors.Wrap(err, "new azure storage")
		}
	default:
		return nil, errors.New("unknown STORAGE_TYPE")
	}

	file, err := os.Open(collectorPasswordPath)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}
	pxcPass, err := io.ReadAll(file)
	if err != nil {
		return nil, errors.Wrap(err, "read password")
	}

	collector := &Collector{
		storage:        s,
		pxcUser:        c.PXCUser,
		pxcPass:        string(pxcPass),
		pxcServiceName: c.PXCServiceName,
	}
	collector.hosts = collector.pxcHosts()

	return collector, nil
}

// pxcHosts returns the real implementation of the host calls.
func (c *Collector) pxcHosts() hostFuncs {
	return hostFuncs{
		isHealthy: func(ctx context.Context, host string) (bool, error) {
			return pxc.IsHostSyncedPrimary(ctx, c.pxcServiceName, host)
		},
		connect: func(host string) (hostConn, error) {
			db, err := pxc.NewPXC(host, c.pxcUser, c.pxcPass)
			if err != nil {
				return nil, err
			}
			return db, nil
		},
		oldest: func(ctx context.Context, prevHost string) (string, error) {
			return pxc.GetPXCOldestBinlogHost(ctx, c.pxcServiceName, c.pxcUser, c.pxcPass, prevHost)
		},
	}
}

func (c *Collector) Init(ctx context.Context) error {
	host, err := pxc.GetPXCFirstHost(ctx, c.pxcServiceName)
	if err != nil {
		return errors.Wrap(err, "get first PXC host")
	}

	db, err := pxc.NewPXC(host, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)
	}
	defer db.Close()

	version, err := db.GetVersion(ctx)
	if err != nil {
		return errors.Wrap(err, "get version")
	}

	switch {
	case strings.HasPrefix(version, "8.0"):
		log.Println("creating collector functions")
		if err := c.CreateCollectorFunctions(ctx); err != nil {
			return errors.Wrap(err, "init 8.0: create collector functions")
		}
	case strings.HasPrefix(version, "8.4"):
		log.Println("installing binlog UDF component")
		if err := db.InstallBinlogUDFComponent(ctx); err != nil {
			return errors.Wrap(err, "init 8.4: install component")
		}
	}

	return nil
}

func (c *Collector) CreateCollectorFunctions(ctx context.Context) error {
	nodes, err := pxc.GetNodesByServiceName(ctx, c.pxcServiceName)
	if err != nil {
		return errors.Wrap(err, "get nodes by service name")
	}

	create := func(node string) error {
		nodeArr := strings.Split(node, ":")
		host := nodeArr[0]
		db, err := pxc.NewPXC(host, c.pxcUser, c.pxcPass)
		if err != nil {
			return errors.Errorf("creating connection for host %s: %v", host, err)
		}
		defer db.Close()
		if err := db.CreateCollectorFunctions(ctx); err != nil {
			return errors.Wrap(err, "create collector functions")
		}
		return nil
	}

	for _, node := range nodes {
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			if err := create(node); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Collector) Run(ctx context.Context) error {
	plan, err := c.pickHost(ctx)
	if err != nil {
		pxcBinlogCollectorBackupFailure.Inc()
		return errors.Wrap(err, "pick host")
	}
	defer c.close()

	// remove last set because we always
	// read it from aws file
	c.lastUploadedSet = pxc.NewGTIDSet("")

	err = c.CollectBinLogs(ctx, plan)
	if err != nil {
		pxcBinlogCollectorBackupFailure.Inc()
		return errors.Wrap(err, "collect binlog files")
	}

	pxcBinlogCollectorBackupSuccess.Inc()
	return nil
}

func (c *Collector) lastGTIDSet(ctx context.Context, suffix string) (pxc.GTIDSet, error) {
	// get last binlog set stored on S3
	lastSetObject, err := c.storage.GetObject(ctx, lastSetFilePrefix+suffix)
	if err != nil {
		if err == storage.ErrObjectNotFound {
			return pxc.GTIDSet{}, nil
		}
		return pxc.GTIDSet{}, errors.Wrap(err, "get last set content")
	}
	defer lastSetObject.Close() //nolint:errcheck
	lastSet, err := io.ReadAll(lastSetObject)
	if err != nil {
		return pxc.GTIDSet{}, errors.Wrap(err, "read last gtid set")
	}
	return pxc.NewGTIDSet(string(lastSet)), nil
}

// pickHost selects the PXC host to read binlogs from, connects c.db to it and
// works out what needs to be uploaded from it.
func (c *Collector) pickHost(ctx context.Context) (*uploadPlan, error) {
	prevHost := ""
	if c.db != nil {
		prevHost = c.db.GetHost()
	}

	if prevHost != "" {
		db, plan, ok := c.reusePreviousHost(ctx, prevHost)
		if ok {
			log.Println("reading binlogs from the same pxc host as the previous run, hostname=", prevHost)
			c.db = db
			c.promTrackHost(db.GetHost())
			return plan, nil
		}
	}

	host, err := c.hosts.oldest(ctx, prevHost)
	if err != nil {
		return nil, errors.Wrap(err, "get host")
	}

	log.Println("reading binlogs from pxc with hostname=", host)

	db, err := c.hosts.connect(host)
	if err != nil {
		return nil, errors.Wrapf(err, "new manager with host %s", host)
	}

	plan, err := c.planUpload(ctx, db)
	if err != nil {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARNING: close connection to host %s: %v", host, cerr)
		}
		return nil, errors.Wrapf(err, "plan upload from host %s", host)
	}

	c.db = db
	c.promTrackHost(db.GetHost())
	return plan, nil
}

func (c *Collector) promTrackHost(host string) {
	pxcBinlogCollectorSourceHost.Reset()
	pxcBinlogCollectorSourceHost.WithLabelValues(host).Set(1)
}

// reusePreviousHost connects to the host used by the previous run and returns the
// upload plan for it if it is still safe to keep collecting from it: the host is
// Synced/Primary and its binlogs still contain the end of the last uploaded GTID
// set (no gap). Switching hosts makes the collector re-upload overlapping
// binlogs, so we only do that when we have to.
// The last return value is false when the caller should pick a host the usual way.
func (c *Collector) reusePreviousHost(ctx context.Context, host string) (hostConn, *uploadPlan, bool) {
	healthy, err := c.hosts.isHealthy(ctx, host)
	if err != nil {
		log.Printf("WARNING: can't check state of previous host %s: %v", host, err)
		return nil, nil, false
	}
	if !healthy {
		log.Printf("previous host %s is not Synced/Primary, selecting a new host", host)
		return nil, nil, false
	}

	db, err := c.hosts.connect(host)
	if err != nil {
		log.Printf("WARNING: can't connect to previous host %s: %v", host, err)
		return nil, nil, false
	}

	plan, err := c.planUpload(ctx, db)
	if err != nil {
		log.Printf("WARNING: can't plan upload from previous host %s: %v", host, err)
	}
	if err != nil || !plan.safeToKeepHost() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("WARNING: close connection to host %s: %v", host, cerr)
		}
		return nil, nil, false
	}

	return db, plan, true
}

// hostConn is a connection to a PXC host the collector reads binlogs from.
type hostConn interface {
	binlogHost
	GetBinLogFirstTimestamp(ctx context.Context, binlog string) (string, error)
	GetBinLogLastTimestamp(ctx context.Context, binlog string) (string, error)
	Close() error
}

// hostFuncs are the calls the collector makes to find and reach PXC hosts.
// They are fields so that host selection can be tested without a cluster.
type hostFuncs struct {
	// isHealthy reports whether the host is Synced and Primary.
	isHealthy func(ctx context.Context, host string) (bool, error)
	// connect opens a connection to the host.
	connect func(host string) (hostConn, error)
	// oldest returns the host with the oldest binlog.
	oldest func(ctx context.Context, prevHost string) (string, error)
}

// binlogHost is a PXC host the collector can read binlogs from.
type binlogHost interface {
	binlogLocator
	GetHost() string
	WsrepClusterStateUUID() (string, error)
	GetBinLogList(ctx context.Context) ([]pxc.Binlog, error)
}

// uploadPlan is what the collector found out about a host before uploading from it.
type uploadPlan struct {
	binlogs []pxc.Binlog // closed binlogs on the host, oldest first
	resume  resumePoint
}

// safeToKeepHost reports whether the host has binlogs with transactions and
// they continue from what was already uploaded.
func (p *uploadPlan) safeToKeepHost() bool {
	return !p.resume.empty && !p.resume.gap
}

// planUpload lists the binlogs on the host, rotating the log so that all of them
// are closed, and finds where to resume uploading. It's done once per run, and
// the same plan is used both to decide if the host can be kept and to upload.
func (c *Collector) planUpload(ctx context.Context, host binlogHost) (*uploadPlan, error) {
	c.emptyBinlogs.use(host.GetHost())

	sourceID, err := host.WsrepClusterStateUUID()
	if err != nil {
		return nil, errors.Wrap(err, "get cluster state uuid")
	}

	binlogs, err := host.GetBinLogList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get binlog list")
	}
	c.emptyBinlogs.retain(binlogs)

	rp, err := c.findResumePoint(ctx, host, sourceID, binlogs)
	if err != nil {
		return nil, errors.Wrap(err, "find resume point")
	}

	return &uploadPlan{binlogs: binlogs, resume: rp}, nil
}

// binlogLocator is the subset of *pxc.PXC used to decide where to resume uploading.
type binlogLocator interface {
	GetGTIDSet(ctx context.Context, binlogName string) (string, error)
	GetBinlogByGTID(ctx context.Context, gtid string) (string, error)
	GetPreviousGTIDs(ctx context.Context, binlogName string) (string, error)
	GTIDSubset(ctx context.Context, set1, set2 string) (bool, error)
}

// resumePoint describes which binlogs on a host still need to be uploaded.
type resumePoint struct {
	lastUploaded pxc.GTIDSet
	// start is the index of the first binlog that may contain not yet uploaded
	// transactions. It's 0 when nothing was uploaded yet or when there is a gap.
	start int
	// gap is true when the last uploaded GTID set can't be found on the host.
	gap bool
	// empty is true when the host has no binlogs with transactions.
	empty bool
}

// findResumePoint locates the binlog containing the end of the last uploaded
// GTID set. It only reads binlog headers, so it's cheap on any host.
func (c *Collector) findResumePoint(
	ctx context.Context,
	loc binlogLocator,
	sourceID string,
	binlogs []pxc.Binlog,
) (resumePoint, error) {
	gtidSets, err := latestGTIDSets(ctx, loc, binlogs, &c.emptyBinlogs)
	if err != nil {
		return resumePoint{}, errors.Wrap(err, "get latest GTID sets")
	}
	if len(gtidSets) == 0 {
		return resumePoint{empty: true}, nil
	}

	lastUploaded, err := c.resolveLastUploadedSet(ctx, gtidSets, sourceID)
	if err != nil {
		return resumePoint{}, errors.Wrap(err, "get last uploaded gtid set")
	}
	if lastUploaded.IsEmpty() {
		return resumePoint{}, nil
	}

	log.Printf("last uploaded GTID set: %s", lastUploaded.Raw())

	// Sets can hold more than one UUID (e.g. after a restore). The most recent
	// binlog containing the end of any of them is where we resume.
	start := -1
	for _, entry := range lastUploaded.List() {
		parsed, err := gtid.New(entry)
		if err != nil {
			log.Printf("failed to parse last uploaded gtid %s: %v", entry, err)
			continue
		}
		uuid, endSeq := parsed.End()

		name, err := loc.GetBinlogByGTID(ctx, fmt.Sprintf("%s:%d", uuid, endSeq))
		if err != nil {
			return resumePoint{}, err
		}
		if name == "" {
			continue
		}

		if i := slices.IndexFunc(binlogs, func(b pxc.Binlog) bool { return b.Name == name }); i > start {
			start = i
		}
	}

	if start < 0 {
		return resumePoint{lastUploaded: lastUploaded, gap: true}, nil
	}
	return resumePoint{lastUploaded: lastUploaded, start: start}, nil
}

// hasNewTransactions reports whether the binlog holds transactions that are not
// part of lastUploaded. It only reads the binlog headers: the set of GTIDs in the
// binlog is compared with lastUploaded by the server, so nothing is lost when the
// binlog starts before, or ends after, the uploaded range.
func hasNewTransactions(
	ctx context.Context,
	loc binlogLocator,
	binlog pxc.Binlog,
	lastUploaded pxc.GTIDSet,
	empties *emptyBinlogs,
) (bool, error) {
	gtidSet, err := gtidSetUnlessEmpty(ctx, loc, binlog, empties)
	if err != nil {
		return false, err
	}
	if gtidSet.IsEmpty() {
		return false, nil
	}

	uploaded, err := loc.GTIDSubset(ctx, gtidSet.Raw(), lastUploaded.Raw())
	if err != nil {
		return false, errors.Wrapf(err, "check if %s is already uploaded", binlog.Name)
	}
	return !uploaded, nil
}

// emptyBinlogs remembers closed binlogs that have no transactions, so that the
// collector doesn't ask the server about the same ones on every run. On an idle
// cluster the periodic binlog rotation adds an empty file every minute, and they
// all pile up behind the last binlog that has transactions.
//
// It is kept in memory only. A binlog is identified by name and size, and is
// forgotten when the host changes or the file is purged, so a recreated or
// modified file is always checked again and there is nothing to invalidate.
type emptyBinlogs struct {
	host  string
	known map[string]struct{}
}

func (e *emptyBinlogs) key(b pxc.Binlog) string {
	return fmt.Sprintf("%s:%d", b.Name, b.Size)
}

// use scopes the memory to host, forgetting everything learned about another host.
func (e *emptyBinlogs) use(host string) {
	if e.host != host {
		e.host = host
		e.known = nil
	}
}

func (e *emptyBinlogs) has(b pxc.Binlog) bool {
	_, ok := e.known[e.key(b)]
	return ok
}

func (e *emptyBinlogs) add(b pxc.Binlog) {
	if e.known == nil {
		e.known = make(map[string]struct{})
	}
	e.known[e.key(b)] = struct{}{}
}

// retain forgets binlogs that are no longer in binlogs, e.g. because they were purged.
func (e *emptyBinlogs) retain(binlogs []pxc.Binlog) {
	current := make(map[string]struct{}, len(binlogs))
	for _, b := range binlogs {
		current[e.key(b)] = struct{}{}
	}
	for key := range e.known {
		if _, ok := current[key]; !ok {
			delete(e.known, key)
		}
	}
}

// gtidSetUnlessEmpty returns the GTID set of a binlog. It returns an empty set,
// without asking the server, for binlogs already known to have no transactions.
// Invalid binlogs (MySQL error 3200) are logged and reported as an empty set so
// that callers skip them. They are not remembered, so they are checked again.
func gtidSetUnlessEmpty(ctx context.Context, loc binlogLocator, binlog pxc.Binlog, empties *emptyBinlogs) (pxc.GTIDSet, error) {
	if empties.has(binlog) {
		return pxc.GTIDSet{}, nil
	}

	set, err := loc.GetGTIDSet(ctx, binlog.Name)
	if err != nil {
		if errors.Is(err, &mysql.MySQLError{Number: 3200}) {
			log.Printf("ERROR: Binlog file %s is invalid: %s\n", binlog.Name, err.Error())
			return pxc.GTIDSet{}, nil
		}
		return pxc.GTIDSet{}, errors.Wrapf(err, "get GTID set of %s", binlog.Name)
	}

	gtidSet := pxc.NewGTIDSet(set)
	if gtidSet.IsEmpty() {
		empties.add(binlog)
	}
	return gtidSet, nil
}

// latestGTIDSets returns one GTID set per UUID that has transactions in the
// most recent binlog that has any. Only the UUIDs are meant to be used: when
// that binlog isn't the last one, the sets also cover earlier binlogs.
func latestGTIDSets(ctx context.Context, loc binlogLocator, binlogs []pxc.Binlog, empties *emptyBinlogs) ([]string, error) {
	if len(binlogs) == 0 {
		return nil, nil
	}

	last := binlogs[len(binlogs)-1]
	gtidSet, err := gtidSetUnlessEmpty(ctx, loc, last, empties)
	if err != nil {
		return nil, err
	}
	if list := gtidSet.List(); len(list) != 0 {
		return list, nil
	}

	// There can be lots of empty binlogs at the end, e.g. on an idle cluster,
	// and asking about each of them takes too long. Previous_gtids of the last
	// binlog holds every UUID executed so far. The binlog holding the last
	// transaction of a UUID is the most recent one with that UUID, so the most
	// recent of those binlogs is the one we are looking for, and the UUIDs
	// ending in it are the ones it has transactions for.
	prev, err := loc.GetPreviousGTIDs(ctx, last.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "get previous GTIDs of %s", last.Name)
	}
	prevSet := pxc.NewGTIDSet(prev)

	newest := -1
	var sets []string
	for _, entry := range prevSet.List() {
		entry = strings.TrimSpace(entry)
		parsed, err := gtid.New(entry)
		if err != nil {
			return nil, errors.Wrapf(err, "parse previous GTIDs of %s", last.Name)
		}
		uuid, endSeq := parsed.End()

		name, err := loc.GetBinlogByGTID(ctx, fmt.Sprintf("%s:%d", uuid, endSeq))
		if err != nil {
			return nil, err
		}

		// Not found when the binlog was purged, e.g. GTIDs received with SST.
		i := slices.IndexFunc(binlogs, func(b pxc.Binlog) bool { return b.Name == name })
		switch {
		case i < 0 || i < newest:
		case i > newest:
			newest = i
			sets = []string{entry}
		default:
			sets = append(sets, entry)
		}
	}
	return sets, nil
}

// forEachBinlogWithGTIDs calls fn, in order, for every binlog that has transactions.
// GTID sets are fetched one binlog at a time, right before fn runs, instead of for
// the whole list up front. Fetching them all first can take longer than the run
// timeout when a lot of binlogs are pending, and nothing would ever get uploaded.
// Doing it lazily means every run makes progress: whatever fn finished (and
// recorded as uploaded) is kept even if a later fetch fails or times out.
func forEachBinlogWithGTIDs(
	ctx context.Context,
	loc binlogLocator,
	binlogs []pxc.Binlog,
	empties *emptyBinlogs,
	fn func(pxc.Binlog) error,
) error {
	for _, binlog := range binlogs {
		gtidSet, err := gtidSetUnlessEmpty(ctx, loc, binlog, empties)
		if err != nil {
			return err
		}
		if gtidSet.IsEmpty() {
			continue
		}

		binlog.GTIDSet = gtidSet
		log.Println(binlog)

		if err := fn(binlog); err != nil {
			return err
		}
	}
	return nil
}

// resolveLastUploadedSet reads the last uploaded GTID set from storage for the
// source UUIDs found in gtidSets, preferring the one matching sourceID.
func (c *Collector) resolveLastUploadedSet(ctx context.Context, gtidSets []string, sourceID string) (pxc.GTIDSet, error) {
	lastUploaded := pxc.NewGTIDSet("")
	for _, gtidSet := range gtidSets {
		id := strings.Split(gtidSet, ":")[0]

		// remove any newline characters from the set name
		id = strings.ReplaceAll(id, "\n", "")
		id = strings.ReplaceAll(id, "\r", "")

		// After a restore, the cluster will generate a new UUID, meaning
		// gtidSets can contain GTID sets from both the current and an old source UUID.
		// In this situation, we must prioritize the GTID set that matches the
		// current cluster UUID. If no file exists for the current UUID, we must
		// create a new one to avoid triggering a false binlog gap error.
		if lastUploaded.IsEmpty() || sourceID == id {
			set, err := c.lastGTIDSet(ctx, id)
			if err != nil {
				return pxc.GTIDSet{}, err
			}
			lastUploaded = set
			if sourceID == id {
				break
			}
		}
	}
	return lastUploaded, nil
}

func (c *Collector) close() error {
	return c.db.Close()
}

func createGapFile(gtidSet pxc.GTIDSet) error {
	p := naming.GapDetected
	f, err := os.Create(p)
	if err != nil {
		return errors.Wrapf(err, "create %s", p)
	}
	defer f.Close()

	_, err = f.WriteString(gtidSet.Raw())
	if err != nil {
		return errors.Wrapf(err, "write GTID set to %s", p)
	}

	return nil
}

func fileExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "os stat")
	}
	return true, nil
}

func createTimelineFile(firstTs string) error {
	f, err := os.Create(naming.TimelinePath)
	if err != nil {
		return errors.Wrapf(err, "create %s", naming.TimelinePath)
	}
	defer f.Close()

	_, err = f.WriteString(firstTs)
	if err != nil {
		return errors.Wrap(err, "write first timestamp to timeline file")
	}

	return nil
}

func updateTimelineFile(lastTs string) error {
	f, err := os.OpenFile(naming.TimelinePath, os.O_RDWR, 0o644)
	if err != nil {
		return errors.Wrapf(err, "open %s", naming.TimelinePath)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "scan %s", naming.TimelinePath)
	}

	if len(lines) > 1 {
		lines[len(lines)-1] = lastTs
	} else {
		lines = append(lines, lastTs)
	}

	if _, err := f.Seek(0, 0); err != nil {
		return errors.Wrapf(err, "seek %s", naming.TimelinePath)
	}

	if err := f.Truncate(0); err != nil {
		return errors.Wrapf(err, "truncate %s", naming.TimelinePath)
	}

	_, err = f.WriteString(strings.Join(lines, "\n"))
	if err != nil {
		return errors.Wrap(err, "write last timestamp to timeline file")
	}

	return nil
}

func (c *Collector) CollectBinLogs(ctx context.Context, plan *uploadPlan) error {
	binlogList, rp := plan.binlogs, plan.resume
	if rp.empty {
		log.Println("no binlogs to upload")
		return nil
	}
	c.lastUploadedSet = rp.lastUploaded

	start := rp.start
	switch {
	case rp.gap:
		log.Println("ERROR: Couldn't find the binlog that contains GTID set:", rp.lastUploaded.Raw())
		log.Println("ERROR: Gap detected in the binary logs. Binary logs will be uploaded anyway, but full backup needed for consistent recovery.")
		pxcBinlogCollectorGapDetected.Inc()
		if err := createGapFile(rp.lastUploaded); err != nil {
			return errors.Wrap(err, "create gap file")
		}
	case !rp.lastUploaded.IsEmpty():
		resumeBinlog := binlogList[start].Name
		log.Printf("last uploaded binlog: %s", resumeBinlog)

		// we don't need to reupload the binlog if it has nothing new
		hasNew, err := hasNewTransactions(ctx, c.db, binlogList[start], rp.lastUploaded, &c.emptyBinlogs)
		if err != nil {
			return errors.Wrap(err, "check last uploaded binlog")
		}
		if !hasNew {
			start++
		}
	}

	uploaded := 0
	err := forEachBinlogWithGTIDs(ctx, c.db, binlogList[start:], &c.emptyBinlogs, func(binlog pxc.Binlog) error {
		if err := c.ensureTimelineFile(ctx, binlog.Name); err != nil {
			return err
		}

		if err := c.manageBinlog(ctx, binlog); err != nil {
			return errors.Wrap(err, "manage binlog")
		}
		uploaded++

		pxcBinlogCollectorUploadedTotal.Inc()
		pxcBinlogCollectorLastUploadTime.SetToCurrentTime()

		lastTs, err := c.db.GetBinLogLastTimestamp(ctx, binlog.Name)
		if err != nil {
			return errors.Wrap(err, "get last timestamp")
		}

		return errors.Wrap(updateTimelineFile(lastTs), "update timeline file")
	})
	if err != nil {
		return errors.Wrap(err, "upload binlogs")
	}

	if uploaded == 0 {
		log.Println("no binlogs to upload after filter")
	}
	pxcBinlogCollectorLastProcessingTime.SetToCurrentTime()

	return nil
}

// ensureTimelineFile creates the timeline file, starting at the first timestamp
// of firstBinlog, if it doesn't exist yet.
func (c *Collector) ensureTimelineFile(ctx context.Context, firstBinlog string) error {
	exists, err := fileExists(naming.TimelinePath)
	if err != nil || exists {
		return nil
	}

	firstTs, err := c.db.GetBinLogFirstTimestamp(ctx, firstBinlog)
	if err != nil {
		return errors.Wrap(err, "get first timestamp")
	}

	return errors.Wrap(createTimelineFile(firstTs), "create timeline file")
}

func mergeErrors(a, b error) error {
	if a != nil && b != nil {
		return errors.New(a.Error() + "; " + b.Error())
	}
	if a != nil {
		return a
	}

	return b
}

func extractIncrementalNumber(binlogName string) (string, error) {
	parts := strings.Split(binlogName, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid binlog name format: %s", binlogName)
	}
	return parts[len(parts)-1], nil
}

func (c *Collector) manageBinlog(ctx context.Context, binlog pxc.Binlog) (err error) {
	binlogTmstmp, err := c.db.GetBinLogFirstTimestamp(ctx, binlog.Name)
	if err != nil {
		return errors.Wrapf(err, "get first timestamp for %s", binlog.Name)
	}

	incrementalNum, err := extractIncrementalNumber(binlog.Name) // extracts e.g. "000011"
	if err != nil {
		return errors.Wrapf(err, "extract incremental number from %s", binlog.Name)
	}

	// Construct internal storage filename with timestamp, incremental number, and GTID md5 hash
	binlogName := fmt.Sprintf("binlog_%s_%s_%x", binlogTmstmp, incrementalNum, md5.Sum([]byte(binlog.GTIDSet.Raw())))

	var setBuffer bytes.Buffer
	// no error handling because WriteString() always returns nil error
	// nolint:errcheck
	setBuffer.WriteString(binlog.GTIDSet.Raw())

	tmpDir := os.TempDir() + "/"

	err = os.Remove(tmpDir + binlog.Name)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove temp file")
	}

	// Create named pipe with original binlog.Name
	err = syscall.Mkfifo(tmpDir+binlog.Name, 0o666)
	if err != nil {
		return errors.Wrap(err, "make named pipe file error")
	}

	errBuf := &bytes.Buffer{}
	cmd := exec.CommandContext(ctx, "mysqlbinlog", "-R", "-P", "33062", "--raw", "-h"+c.db.GetHost(), "-u"+c.pxcUser, binlog.Name)
	cmd.Env = append(cmd.Env, "MYSQL_PWD="+c.pxcPass)
	cmd.Dir = os.TempDir()
	cmd.Stderr = errBuf

	err = cmd.Start()
	if err != nil {
		return errors.Wrap(err, "run mysqlbinlog command")
	}

	log.Println("starting to process binlog with name", binlog.Name)

	namedPipeFile := tmpDir + binlog.Name

	fd, err := unix.Open(namedPipeFile, unix.O_RDONLY|unix.O_NONBLOCK, uint32(fs.ModeNamedPipe))
	if err != nil {
		return errors.Wrapf(err, "open named pipe %s", namedPipeFile)
	}

	file := os.NewFile(uintptr(fd), namedPipeFile)

	defer func() {
		errC := file.Close()
		if errC != nil {
			err = mergeErrors(err, errors.Wrapf(errC, "close tmp file for %s", binlog.Name))
			return
		}
		errR := os.Remove(namedPipeFile)
		if errR != nil {
			err = mergeErrors(err, errors.Wrapf(errR, "remove tmp file for %s", binlog.Name))
			return
		}
	}()

	// create a pipe to transfer data from the binlog pipe to s3
	pr, pw := io.Pipe()

	go readBinlog(ctx, file, pw, errBuf, binlog.Name)

	// Use constructed binlogName for storage keys
	err = c.storage.PutObject(ctx, binlogName, pr, -1)
	if err != nil {
		return errors.Wrapf(err, "put %s object", binlog.Name)
	}

	log.Println("successfully wrote binlog file", binlog.Name, "to storage with name", binlogName)

	err = cmd.Wait()
	if err != nil {
		return errors.Wrap(err, "wait mysqlbinlog command error:"+errBuf.String())
	}

	err = c.storage.PutObject(ctx, binlogName+gtidPostfix, &setBuffer, int64(setBuffer.Len()))
	if err != nil {
		return errors.Wrap(err, "put gtid-set object")
	}
	for _, gtidSet := range binlog.GTIDSet.List() {
		// no error handling because WriteString() always returns nil error
		// nolint:errcheck
		setBuffer.WriteString(binlog.GTIDSet.Raw())

		lastSetName := lastSetFilePrefix + strings.Split(gtidSet, ":")[0]

		// remove any newline characters from the last set name
		lastSetName = strings.ReplaceAll(lastSetName, "\n", "")
		lastSetName = strings.ReplaceAll(lastSetName, "\r", "")

		err = c.storage.PutObject(ctx, lastSetName, &setBuffer, int64(setBuffer.Len()))
		if err != nil {
			return errors.Wrap(err, "put last-set object")
		}
	}
	c.lastUploadedSet = binlog.GTIDSet

	return nil
}

func readBinlog(ctx context.Context, file *os.File, pipe *io.PipeWriter, errBuf *bytes.Buffer, binlogName string) {
	b := make([]byte, 10485760) // alloc buffer for 10mb

	// in case of binlog is slow and hasn't written anything to the file yet
	// we have to skip this error and try to read again until some data appears
	read := func() error {
		isEmpty := true
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if errBuf.Len() != 0 {
					return errors.Errorf("mysqlbinlog %s", errBuf.String())
				}

				n, err := file.Read(b)
				if err == io.EOF {
					// If we got EOF immediately after starting to read a file we should skip it since
					// data has not appeared yet. If we receive EOF error after already got some data - then exit.
					if isEmpty {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					return nil
				}

				if err != nil && !strings.Contains(err.Error(), "file already closed") {
					return errors.Wrapf(err, "reading named pipe for %s", binlogName)
				}

				if n == 0 {
					time.Sleep(10 * time.Millisecond)
					continue
				}

				_, err = pipe.Write(b[:n])
				if err != nil {
					return errors.Wrapf(err, "Error: write to pipe for %s", binlogName)
				}

				isEmpty = false
			}
		}
	}

	if err := read(); err != nil {
		// no error handling because CloseWithError() always return nil error
		// nolint:errcheck
		pipe.CloseWithError(err)
		return
	}

	// in case of any errors from mysqlbinlog it sends EOF to pipe
	// to prevent this, need to check error buffer before closing pipe without error
	if errBuf.Len() != 0 {
		// no error handling because CloseWithError() always return nil error
		// nolint:errcheck
		pipe.CloseWithError(errors.New("mysqlbinlog error:" + errBuf.String()))
		return
	}

	// no error handling because Close() always return nil error
	// nolint:errcheck
	pipe.Close()
}
