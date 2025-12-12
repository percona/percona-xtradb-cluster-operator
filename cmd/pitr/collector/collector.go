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
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
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
)

func init() {
	prometheus.MustRegister(pxcBinlogCollectorBackupSuccess)
	prometheus.MustRegister(pxcBinlogCollectorBackupFailure)
	prometheus.MustRegister(pxcBinlogCollectorLastProcessingTime)
	prometheus.MustRegister(pxcBinlogCollectorLastUploadTime)
	prometheus.MustRegister(pxcBinlogCollectorGapDetected)
	prometheus.MustRegister(pxcBinlogCollectorUploadedTotal)
}

type Collector struct {
	db              *pxc.PXC
	storage         storage.Storage
	lastUploadedSet pxc.GTIDSet // last uploaded binary logs set
	pxcServiceName  string      // k8s service name for PXC, its for get correct host for connection
	pxcUser         string      // user for connection to PXC
	pxcPass         string      // password for connection to PXC
	gtidCacheKey    string      // filename of gtid cache json
	sourceID        string
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
	GTIDCacheKey       string  `env:"GTID_CACHE_KEY,required"`
}

type BackupS3 struct {
	Endpoint    string `env:"ENDPOINT" envDefault:"s3.amazonaws.com"`
	AccessKeyID string `env:"ACCESS_KEY_ID,required"`
	AccessKey   string `env:"SECRET_ACCESS_KEY,required"`
	BucketURL   string `env:"S3_BUCKET_URL,required"`
	Region      string `env:"DEFAULT_REGION,required"`
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

		s, err = storage.NewS3(ctx, c.BackupStorageS3.Endpoint, c.BackupStorageS3.AccessKeyID, c.BackupStorageS3.AccessKey, bucketArr[0], prefix, c.BackupStorageS3.Region, c.VerifyTLS, caBundle)
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

	return &Collector{
		storage:        s,
		pxcUser:        c.PXCUser,
		pxcPass:        string(pxcPass),
		pxcServiceName: c.PXCServiceName,
		gtidCacheKey:   c.GTIDCacheKey,
	}, nil
}

func (c *Collector) GetStorage() storage.Storage {
	return c.storage
}

func (c *Collector) GetGTIDCacheKey() string {
	return c.gtidCacheKey
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
	err := c.newDB(ctx)
	if err != nil {
		pxcBinlogCollectorBackupFailure.Inc()
		return errors.Wrap(err, "new db connection")
	}
	defer c.close()

	// remove last set because we always
	// read it from aws file
	c.lastUploadedSet = pxc.NewGTIDSet("")
	c.sourceID, err = c.db.WsrepClusterStateUUID()
	if err != nil {
		return errors.Wrap(err, "get cluster state uuid")
	}

	err = c.CollectBinLogs(ctx)
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
	lastSet, err := io.ReadAll(lastSetObject)
	if err != nil {
		return pxc.GTIDSet{}, errors.Wrap(err, "read last gtid set")
	}
	return pxc.NewGTIDSet(string(lastSet)), nil
}

func (c *Collector) newDB(ctx context.Context) error {
	host, err := pxc.GetPXCOldestBinlogHost(ctx, c.pxcServiceName, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrap(err, "get host")
	}

	log.Println("reading binlogs from pxc with hostname=", host)

	c.db, err = pxc.NewPXC(host, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)
	}

	return nil
}

func (c *Collector) close() error {
	return c.db.Close()
}

func (c *Collector) removeEmptyBinlogs(ctx context.Context, logs []pxc.Binlog) ([]pxc.Binlog, error) {
	result := make([]pxc.Binlog, 0)
	for _, v := range logs {
		if !v.GTIDSet.IsEmpty() {
			result = append(result, v)
		}
	}
	return result, nil
}

func (c *Collector) filterBinLogs(ctx context.Context, logs []pxc.Binlog, lastBinlogName string) ([]pxc.Binlog, error) {
	if lastBinlogName == "" {
		return c.removeEmptyBinlogs(ctx, logs)
	}

	logsLen := len(logs)

	startIndex := 0
	for logs[startIndex].Name != lastBinlogName && startIndex < logsLen {
		startIndex++
	}

	if startIndex == logsLen {
		return nil, nil
	}

	set, err := c.db.GetGTIDSet(ctx, logs[startIndex].Name)
	if err != nil {
		return nil, errors.Wrap(err, "get gtid set of last uploaded binlog")
	}

	// we don't need to reupload last file
	// if gtid set is not changed
	if set == c.lastUploadedSet.Raw() {
		startIndex++
	}

	return c.removeEmptyBinlogs(ctx, logs[startIndex:])
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

func (c *Collector) addGTIDSets(ctx context.Context, cache *HostBinlogCache, binlogs []pxc.Binlog) error {
	hostCache, ok := cache.Entries[c.db.GetHost()]
	if ok {
		cacheNeedsUpdate := false

		for i, binlog := range binlogs {
			gtidSet, ok := hostCache.Get(binlog.Name)
			if !ok {
				log.Printf("no cache entry for %s", binlog.Name)

				set, err := c.db.GetGTIDSet(ctx, binlog.Name)
				if errors.Is(err, &mysql.MySQLError{Number: 3200}) {
					log.Printf("ERROR: Binlog file %s is invalid on host %s: %s\n", binlog.Name, c.db.GetHost(), err.Error())
					continue
				}

				gtidSet = set
				hostCache.Set(binlog.Name, gtidSet)
				cacheNeedsUpdate = true
			}

			binlogs[i].GTIDSet = pxc.NewGTIDSet(gtidSet)

			log.Println(binlogs[i])
		}

		if cacheNeedsUpdate {
			if err := saveCache(ctx, c.storage, cache, c.gtidCacheKey); err != nil {
				return errors.Wrap(err, "update binlog cache")
			}
		}

		return nil
	}

	// cache not found
	hostCache = &BinlogCacheEntry{
		Binlogs: make(map[string]string),
	}
	cache.Entries[c.db.GetHost()] = hostCache

	for i, binlog := range binlogs {
		set, err := c.db.GetGTIDSet(ctx, binlog.Name)
		if err != nil {
			if errors.Is(err, &mysql.MySQLError{Number: 3200}) {
				log.Printf("ERROR: Binlog file %s is invalid on host %s: %s\n", binlog.Name, c.db.GetHost(), err.Error())
				continue
			}
			return errors.Wrap(err, "get GTID set")
		}

		binlogs[i].GTIDSet = pxc.NewGTIDSet(set)
		hostCache.Set(binlog.Name, binlogs[i].GTIDSet.Raw())

		log.Println(binlogs[i])
	}

	if err := saveCache(ctx, c.storage, cache, c.gtidCacheKey); err != nil {
		return errors.Wrap(err, "update binlog cache")
	}

	return nil
}

func (c *Collector) CollectBinLogs(ctx context.Context) error {
	cache, err := loadCache(ctx, c.storage, c.gtidCacheKey)
	if err != nil {
		return errors.Wrap(err, "load binlog cache")
	}

	if _, ok := cache.Entries[c.db.GetHost()]; !ok {
		log.Println("WARNING: ignoring timeout to populate the cache, this might take some time...")
		ctx = context.Background()
	}

	binlogList, err := c.db.GetBinLogList(ctx)
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}

	err = c.addGTIDSets(ctx, cache, binlogList)
	if err != nil {
		return errors.Wrap(err, "get GTID sets")
	}

	var lastGTIDSetList []string
	for i := len(binlogList) - 1; i >= 0; i-- {
		gtidSetList := binlogList[i].GTIDSet.List()
		if len(gtidSetList) != 0 {
			lastGTIDSetList = gtidSetList
			break
		}
	}

	if len(lastGTIDSetList) == 0 {
		log.Println("no binlogs to upload")
		return nil
	}

	for _, gtidSet := range lastGTIDSetList {
		sourceID := strings.Split(gtidSet, ":")[0]

		// remove any newline characters from the set name
		sourceID = strings.ReplaceAll(sourceID, "\n", "")
		sourceID = strings.ReplaceAll(sourceID, "\r", "")

		// After a restore, the cluster will generate a new UUID, meaning
		// lastGTIDSetList can contain GTID sets from both the current and an old source UUID.
		// In this situation, we must prioritize the GTID set that matches the
		// current cluster UUID. If no file exists for the current UUID, we must
		// create a new one to avoid triggering a false binlog gap error.
		if c.lastUploadedSet.IsEmpty() || c.sourceID == sourceID {
			c.lastUploadedSet, err = c.lastGTIDSet(ctx, sourceID)
			if err != nil {
				return errors.Wrap(err, "get last uploaded gtid set")
			}
			if c.sourceID == sourceID {
				break
			}
		}
	}

	lastUploadedBinlogName := ""

	if !c.lastUploadedSet.IsEmpty() {
		log.Printf("last uploaded GTID set: %s", c.lastUploadedSet.Raw())

		for i := len(binlogList) - 1; i >= 0 && lastUploadedBinlogName == ""; i-- {
			log.Printf("checking %s (%s) against last uploaded set", binlogList[i].Name, binlogList[i].GTIDSet.Raw())
			for _, gtidSet := range binlogList[i].GTIDSet.List() {
				if lastUploadedBinlogName != "" {
					break
				}
				for _, lastUploaded := range c.lastUploadedSet.List() {
					if lastUploaded == gtidSet {
						log.Printf("last uploaded %s is equal to %s in %s", lastUploaded, gtidSet, binlogList[i].Name)
						lastUploadedBinlogName = binlogList[i].Name
						break
					}
					isSubset, err := c.db.GTIDSubset(ctx, lastUploaded, gtidSet)
					if err != nil {
						return errors.Wrap(err, "check if gtid set is subset")
					}
					if isSubset {
						log.Printf("last uploaded %s is subset of %s in %s", lastUploaded, gtidSet, binlogList[i].Name)
						lastUploadedBinlogName = binlogList[i].Name
						break
					}
					isSubset, err = c.db.GTIDSubset(ctx, gtidSet, lastUploaded)
					if err != nil {
						return errors.Wrap(err, "check if gtid set is subset")
					}
					if isSubset {
						log.Printf("%s in %s is subset of last uploaded %s", gtidSet, binlogList[i].Name, lastUploaded)
						lastUploadedBinlogName = binlogList[i].Name
						break
					}
					log.Printf("last uploaded %s is not subset of %s in %s or vice versa", lastUploaded, gtidSet, binlogList[i].Name)
				}
			}
		}

		if lastUploadedBinlogName == "" {
			log.Println("ERROR: Couldn't find the binlog that contains GTID set:", c.lastUploadedSet.Raw())
			log.Println("ERROR: Gap detected in the binary logs. Binary logs will be uploaded anyway, but full backup needed for consistent recovery.")
			pxcBinlogCollectorGapDetected.Inc()
			if err := createGapFile(c.lastUploadedSet); err != nil {
				return errors.Wrap(err, "create gap file")
			}
		}
	}

	log.Printf("last uploaded binlog: %s", lastUploadedBinlogName)

	binlogList, err = c.filterBinLogs(ctx, binlogList, lastUploadedBinlogName)
	if err != nil {
		return errors.Wrap(err, "filter empty binlogs")
	}

	if len(binlogList) == 0 {
		log.Println("no binlogs to upload after filter")
		pxcBinlogCollectorLastProcessingTime.SetToCurrentTime()
		return nil
	}

	if exists, err := fileExists(naming.TimelinePath); !exists && err == nil {
		firstTs, err := c.db.GetBinLogFirstTimestamp(ctx, binlogList[0].Name)
		if err != nil {
			return errors.Wrap(err, "get first timestamp")
		}

		if err := createTimelineFile(firstTs); err != nil {
			return errors.Wrap(err, "create timeline file")
		}
	}

	for _, binlog := range binlogList {
		err = c.manageBinlog(ctx, binlog)
		if err != nil {
			return errors.Wrap(err, "manage binlog")
		}

		pxcBinlogCollectorUploadedTotal.Inc()
		pxcBinlogCollectorLastUploadTime.SetToCurrentTime()

		lastTs, err := c.db.GetBinLogLastTimestamp(ctx, binlog.Name)
		if err != nil {
			return errors.Wrap(err, "get last timestamp")
		}

		if err := updateTimelineFile(lastTs); err != nil {
			return errors.Wrap(err, "update timeline file")
		}
	}

	pxcBinlogCollectorLastProcessingTime.SetToCurrentTime()

	return nil
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
