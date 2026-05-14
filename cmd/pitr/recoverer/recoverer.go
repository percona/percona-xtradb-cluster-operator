package recoverer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	xbserver "github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/server"

	"github.com/pkg/errors"
)

type Recoverer struct {
	db             *pxc.PXC
	recoverTime    string
	storage        storage.Storage
	pxcUser        string
	pxcPass        string
	recoverType    RecoverType
	pxcServiceName string
	binlogs        []string
	gtidSet        string
	startGTID      string // contains the gtid_executed of the restored full backup
	timelineUUID   string // contains the galera UUID of cluster at the time of backup
	recoverFlag    string
	recoverEndTime time.Time
	gtid           string
	verifyTLS      bool
}

type Config struct {
	PXCServiceName     string `env:"PXC_SERVICE,required"`
	PXCUser            string `env:"PXC_USER,required"`
	PXCPass            string `env:"PXC_PASS,required"`
	BackupStorageS3    BackupS3
	BackupStorageAzure BackupAzure
	RecoverTime        string `env:"PITR_DATE"`
	RecoverType        string `env:"PITR_RECOVERY_TYPE,required"`
	GTID               string `env:"PITR_GTID"`
	VerifyTLS          bool   `env:"VERIFY_TLS" envDefault:"true"`
	StorageType        string `env:"STORAGE_TYPE,required"`
	BinlogStorageS3    BinlogS3
	BinlogStorageAzure BinlogAzure
}

func (c Config) storages(ctx context.Context) (storage.Storage, storage.Storage, error) {
	var binlogStorage, defaultStorage storage.Storage
	switch c.StorageType {
	case "s3":
		bucket, prefix, err := getBucketAndPrefix(c.BinlogStorageS3.BucketURL)
		if err != nil {
			return nil, nil, errors.Wrap(err, "get bucket and prefix")
		}

		// try to read the S3 CA bundle
		caBundle, err := os.ReadFile(path.Join(naming.BackupStorageCAFileDirectory, naming.BackupStorageCAFileName))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, errors.Wrap(err, "read CA bundle file")
		}

		binlogStorage, err = storage.NewS3(ctx, c.BinlogStorageS3.Endpoint, c.BinlogStorageS3.AccessKeyID, c.BinlogStorageS3.AccessKey, c.BinlogStorageS3.SessionToken, bucket, prefix, c.BinlogStorageS3.Region, c.VerifyTLS, caBundle, c.BinlogStorageS3.ForcePath)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new s3 storage")
		}

		bucket, prefix, err = getBucketAndPrefix(c.BackupStorageS3.BackupDest)
		if err != nil {
			return nil, nil, errors.Wrap(err, "get bucket and prefix")
		}
		defaultStorage, err = storage.NewS3(ctx, c.BackupStorageS3.Endpoint, c.BackupStorageS3.AccessKeyID, c.BackupStorageS3.AccessKey, c.BackupStorageS3.SessionToken, bucket, prefix, c.BackupStorageS3.Region, c.VerifyTLS, caBundle, c.BackupStorageS3.ForcePath)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new storage manager")
		}
	case "azure":
		var err error
		container, prefix := getContainerAndPrefix(c.BinlogStorageAzure.ContainerPath)
		binlogStorage, err = storage.NewAzure(c.BinlogStorageAzure.AccountName, c.BinlogStorageAzure.AccountKey, c.BinlogStorageAzure.Endpoint, container, prefix, c.BinlogStorageAzure.BlockSize, c.BinlogStorageAzure.Concurrency)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new azure storage")
		}
		defaultStorage, err = storage.NewAzure(c.BackupStorageAzure.AccountName, c.BackupStorageAzure.AccountKey, c.BackupStorageAzure.Endpoint, c.BackupStorageAzure.ContainerName, c.BackupStorageAzure.BackupDest, c.BackupStorageAzure.BlockSize, c.BackupStorageAzure.Concurrency)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new azure storage")
		}
	default:
		return nil, nil, errors.New("unknown STORAGE_TYPE")
	}
	return binlogStorage, defaultStorage, nil
}

type BackupS3 struct {
	Endpoint     string `env:"ENDPOINT" envDefault:"s3.amazonaws.com"`
	AccessKeyID  string `env:"ACCESS_KEY_ID,required"`
	AccessKey    string `env:"SECRET_ACCESS_KEY,required"`
	SessionToken string `env:"S3_SESSION_TOKEN"`
	Region       string `env:"DEFAULT_REGION,required"`
	BackupDest   string `env:"S3_BUCKET_URL,required"`
	ForcePath    bool   `env:"S3_FORCE_PATH"`
}

type BackupAzure struct {
	Endpoint      string `env:"AZURE_ENDPOINT,required"`
	ContainerName string `env:"AZURE_CONTAINER_NAME,required"`
	StorageClass  string `env:"AZURE_STORAGE_CLASS"`
	AccountName   string `env:"AZURE_STORAGE_ACCOUNT,required"`
	AccountKey    string `env:"AZURE_ACCESS_KEY,required"`
	BackupDest    string `env:"BACKUP_PATH,required"`
	BlockSize     int64  `env:"AZURE_BLOCK_SIZE"`
	Concurrency   int    `env:"AZURE_CONCURRENCY"`
}

type BinlogS3 struct {
	Endpoint     string `env:"BINLOG_S3_ENDPOINT" envDefault:"s3.amazonaws.com"`
	AccessKeyID  string `env:"BINLOG_ACCESS_KEY_ID,required"`
	AccessKey    string `env:"BINLOG_SECRET_ACCESS_KEY,required"`
	SessionToken string `env:"BINLOG_SESSION_TOKEN"`
	Region       string `env:"BINLOG_S3_REGION,required"`
	BucketURL    string `env:"BINLOG_S3_BUCKET_URL,required"`
	ForcePath    bool   `env:"BINLOG_S3_FORCE_PATH"`
}

type BinlogAzure struct {
	Endpoint      string `env:"BINLOG_AZURE_ENDPOINT,required"`
	ContainerPath string `env:"BINLOG_AZURE_CONTAINER_PATH,required"`
	StorageClass  string `env:"BINLOG_AZURE_STORAGE_CLASS"`
	AccountName   string `env:"BINLOG_AZURE_STORAGE_ACCOUNT,required"`
	AccountKey    string `env:"BINLOG_AZURE_ACCESS_KEY,required"`
	BlockSize     int64  `env:"BINLOG_AZURE_BLOCK_SIZE"`
	Concurrency   int    `env:"BINLOG_AZURE_CONCURRENCY"`
}

func (c *Config) Verify() {
	if len(c.BackupStorageS3.Endpoint) == 0 {
		c.BackupStorageS3.Endpoint = "s3.amazonaws.com"
	}
	if len(c.BinlogStorageS3.Endpoint) == 0 {
		c.BinlogStorageS3.Endpoint = "s3.amazonaws.com"
	}
}

type RecoverType string

func New(ctx context.Context, c Config) (*Recoverer, error) {
	c.Verify()

	log.Printf("starting point-in-time-recovery, type: %s", c.RecoverType)

	binlogStorage, storage, err := c.storages(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "new binlog storage manager")
	}

	startGTID, err := getStartGTIDSet(ctx, storage)
	if err != nil {
		return nil, errors.Wrap(err, "get start GTID")
	}
	log.Printf("backup GTID executed: %s", startGTID)

	timelineUUID, err := getBackupTimelineUUID(ctx, storage)
	if err != nil {
		return nil, errors.Wrap(err, "get backup timeline UUID")
	}
	if timelineUUID == "" {
		return nil, errors.New("backup timeline UUID is empty")
	}

	log.Printf("backup timeline UUID: %s", timelineUUID)

	if c.RecoverType == string(Transaction) {
		if err := validateTransactionGTID(c.GTID, startGTID); err != nil {
			return nil, errors.Wrap(err, "validate transaction GTID")
		}
	}

	return &Recoverer{
		storage:        binlogStorage,
		recoverTime:    c.RecoverTime,
		pxcUser:        c.PXCUser,
		pxcPass:        c.PXCPass,
		pxcServiceName: c.PXCServiceName,
		recoverType:    RecoverType(c.RecoverType),
		startGTID:      startGTID,
		timelineUUID:   timelineUUID,
		gtid:           c.GTID,
		verifyTLS:      c.VerifyTLS,
	}, nil
}

func validateTransactionGTID(targetGTID, startGTID string) error {
	targetParts := strings.SplitN(targetGTID, ":", 2)
	if len(targetParts) != 2 {
		return errors.Errorf("invalid target GTID %q", targetGTID)
	}
	targetUUID := targetParts[0]
	targetSeq, err := strconv.ParseInt(targetParts[1], 10, 64)
	if err != nil {
		return errors.Wrap(err, "parse target GTID seqno")
	}

	for seg := range strings.SplitSeq(startGTID, ",") {
		seg = strings.TrimSpace(seg)
		segParts := strings.SplitN(seg, ":", 2)
		if len(segParts) != 2 || segParts[0] != targetUUID {
			continue
		}

		rangeStr := segParts[1]
		rangeStr = rangeStr[strings.LastIndex(rangeStr, ":")+1:]
		hi := rangeStr
		if _, after, ok := strings.Cut(rangeStr, "-"); ok {
			hi = after
		}
		hiInt, err := strconv.ParseInt(hi, 10, 64)
		if err != nil {
			return errors.Wrap(err, "parse high end of backup range")
		}
		if targetSeq < hiInt {
			return errors.Errorf(
				"target GTID %s is already inside the backup (segment %s); can't recover to a transaction before backup",
				targetGTID, seg)
		}
	}

	return nil
}

func getContainerAndPrefix(s string) (string, string) {
	container, prefix, _ := strings.Cut(s, "/")
	if prefix != "" {
		prefix += "/"
	}
	return container, prefix
}

func getBucketAndPrefix(bucketURL string) (bucket string, prefix string, err error) {
	u, err := url.Parse(bucketURL)
	if err != nil {
		err = errors.Wrap(err, "parse url")
		return bucket, prefix, err
	}
	path := strings.TrimPrefix(strings.TrimSuffix(u.Path, "/"), "/")

	if u.IsAbs() && u.Scheme == "s3" {
		bucket = u.Host
		prefix = path + "/"
		return bucket, prefix, err
	}
	bucketArr := strings.Split(path, "/")
	if len(bucketArr) > 1 {
		prefix = strings.TrimPrefix(path, bucketArr[0]+"/") + "/"
	}
	bucket = bucketArr[0]
	if len(bucket) == 0 {
		err = errors.Errorf("can't get bucket name from %s", bucketURL)
		return bucket, prefix, err
	}

	return bucket, prefix, err
}

const (
	Latest      RecoverType = "latest"      // recover to the latest existing binlog
	Date        RecoverType = "date"        // recover to exact date
	Transaction RecoverType = "transaction" // recover to needed trunsaction
	Skip        RecoverType = "skip"        // skip transactions
)

func (r *Recoverer) Run(ctx context.Context) error {
	host, err := pxc.GetPXCFirstHost(ctx, r.pxcServiceName)
	if err != nil {
		return errors.Wrap(err, "get host")
	}
	r.db, err = pxc.NewPXC(host, r.pxcUser, r.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)
	}

	err = r.setBinlogs(ctx)
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}

	switch r.recoverType {
	case Skip:
		r.recoverFlag = "--exclude-gtids=" + r.gtid
		log.Printf("recovery type: %s, gtid: %s", Skip, r.gtid)
	case Transaction:
		r.recoverFlag = "--exclude-gtids=" + r.gtidSet
		log.Printf("recovery type: %s, gtid set: %s", Transaction, r.gtidSet)
	case Date:
		r.recoverFlag = `--stop-datetime="` + r.recoverTime + `"`

		const format = "2006-01-02 15:04:05"
		endTime, err := time.Parse(format, r.recoverTime)
		if err != nil {
			return errors.Wrap(err, "parse date")
		}
		r.recoverEndTime = endTime

		log.Printf("recovery type: %s, target time: %s", Date, r.recoverEndTime)
	case Latest:
		log.Printf("recovery type: %s", Latest)
	default:
		return errors.New("wrong recover type")
	}

	err = r.recover(ctx)
	if err != nil {
		return errors.Wrap(err, "recover")
	}

	return nil
}

func (r *Recoverer) recover(ctx context.Context) (err error) {
	version, err := r.db.GetVersion(ctx)
	if err != nil {
		return errors.Wrap(err, "get version")
	}

	switch {
	case strings.HasPrefix(version, "8.0"):
		err = r.db.DropCollectorFunctions(ctx)
		if err != nil {
			return errors.Wrap(err, "drop collector funcs")
		}
	case strings.HasPrefix(version, "8.4"):
		if err := r.db.UninstallBinlogUDFComponent(ctx); err != nil {
			return errors.Wrap(err, "uninstall component")
		}
	}

	err = os.Setenv("MYSQL_PWD", os.Getenv("PXC_PASS"))
	if err != nil {
		return errors.Wrap(err, "set mysql pwd env var")
	}

	mysqlStdin, binlogStdout := io.Pipe()
	defer mysqlStdin.Close()

	mysqlCmd := exec.CommandContext(ctx, "mysql", "-h", r.db.GetHost(), "-P", "33062", "-u", r.pxcUser)
	log.Printf("Running %s", mysqlCmd.String())
	mysqlCmd.Stdin = mysqlStdin
	mysqlCmd.Stderr = os.Stderr
	mysqlCmd.Stdout = os.Stdout
	if err := mysqlCmd.Start(); err != nil {
		return errors.Wrap(err, "start mysql")
	}

	for i, binlog := range r.binlogs {
		remaining := len(r.binlogs) - i
		log.Printf("working with %s, %d out of %d remaining\n", binlog, remaining, len(r.binlogs))
		if r.recoverType == Date {
			binlogArr := strings.Split(binlog, "_")
			if len(binlogArr) < 2 {
				return errors.New("get timestamp from binlog name")
			}
			binlogTime, err := strconv.ParseInt(binlogArr[1], 10, 64)
			if err != nil {
				return errors.Wrap(err, "get binlog time")
			}
			if binlogTime > r.recoverEndTime.Unix() {
				log.Printf("Stopping at %s because it's after the recovery time (%d > %d)", binlog, binlogTime, r.recoverEndTime.Unix())
				break
			}
		}

		binlogObj, err := r.storage.GetObject(ctx, binlog)
		if err != nil {
			return errors.Wrap(err, "get obj")
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", "mysqlbinlog --disable-log-bin "+r.recoverFlag+" -")
		log.Printf("Running %s", cmd.String())
		cmd.Stdin = binlogObj
		cmd.Stdout = binlogStdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			return errors.Wrapf(err, "run mysqlbinlog")
		}
	}

	if err := binlogStdout.Close(); err != nil {
		return errors.Wrap(err, "close binlog stdout")
	}

	log.Printf("Waiting for mysql to finish")

	if err := mysqlCmd.Wait(); err != nil {
		return errors.Wrap(err, "wait mysql")
	}

	log.Printf("Finished")

	return nil
}

type testContextKey struct{}

func getDecompressedContent(ctx context.Context, infoObj io.Reader, filename string) ([]byte, error) {
	// this is done to support unit tests
	if val, ok := ctx.Value(testContextKey{}).(bool); ok && val {
		return io.ReadAll(infoObj)
	}

	tmpDir := os.TempDir()

	cmd := exec.CommandContext(ctx, "xbstream", "-x", "--decompress")
	cmd.Dir = tmpDir
	cmd.Stdin = infoObj
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "xbstream cmd run. stderr: %s, stdout: %s", &errb, &outb)
	}
	if errb.Len() > 0 {
		return nil, errors.Errorf("run xbstream error: %s", &errb)
	}

	decContent, err := os.ReadFile(tmpDir + "/" + filename)
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", filename)
	}

	return decContent, nil
}

func (r *Recoverer) setBinlogs(ctx context.Context) error {
	list, err := r.storage.ListObjects(ctx, "binlog_")
	if err != nil {
		return errors.Wrap(err, "list objects with prefix 'binlog_'")
	}
	reverse(list)
	binlogs := []string{}
	sourceID := strings.Split(r.startGTID, ":")[0]
	log.Println("current gtid set is", r.startGTID)
	for _, binlog := range list {
		if strings.Contains(binlog, "-gtid-set") {
			continue
		}
		infoObj, err := r.storage.GetObject(ctx, binlog+"-gtid-set")
		if err != nil {
			log.Println("Can't get binlog object with gtid set. Name:", binlog, "error", err)
			continue
		}

		content, err := io.ReadAll(infoObj)
		infoObj.Close() //nolint:errcheck
		if err != nil {
			return errors.Wrapf(err, "read %s gtid-set object", binlog)
		}

		binlogGTIDSet := string(content)
		log.Println("checking current file", " name ", binlog, " gtid ", binlogGTIDSet)

		if !gtidSetContainsUUID(binlogGTIDSet, r.timelineUUID) {
			log.Println("skipping binlog", binlog, "because it's not from the same timeline as the backup")
			continue
		}

		if len(r.gtid) > 0 && r.recoverType == Transaction {
			subResult, err := r.db.SubtractGTIDSet(ctx, binlogGTIDSet, r.gtid)
			if err != nil {
				return errors.Wrapf(err, "check if '%s' is a subset of '%s", binlogGTIDSet, r.gtid)
			}
			if subResult != binlogGTIDSet {
				set, err := getExtendGTIDSet(binlogGTIDSet, r.gtid)
				if err != nil {
					return errors.Wrap(err, "get gtid set for extend")
				}
				r.gtidSet = set
			}
			if len(r.gtidSet) == 0 {
				continue
			}
		}

		binlogs = append(binlogs, binlog)
		subResult, err := r.db.SubtractGTIDSet(ctx, r.startGTID, binlogGTIDSet)
		log.Println("Checking sub result", " binlog gtid ", binlogGTIDSet, " sub result ", subResult)
		if err != nil {
			return errors.Wrapf(err, "check if '%s' is a subset of '%s", r.startGTID, binlogGTIDSet)
		}
		if subResult != r.startGTID {
			break
		}
	}
	if len(binlogs) == 0 {
		return errors.Errorf("no objects for prefix binlog_ or with source_id=%s", sourceID)
	}
	reverse(binlogs)
	r.binlogs = binlogs

	return nil
}

func gtidSetContainsUUID(gtidSet, uuid string) bool {
	for segment := range strings.SplitSeq(gtidSet, ",") {
		segment = strings.TrimSpace(segment)
		if strings.HasPrefix(segment, uuid+":") {
			return true
		}
	}
	return false
}

func getExtendGTIDSet(gtidSet, gtid string) (string, error) {
	if gtidSet == gtid {
		return gtid, nil
	}

	s := strings.Split(gtidSet, ":")
	if len(s) < 2 {
		return "", errors.Errorf("incorrect source in gtid set %s", gtidSet)
	}

	eidx := 1
	e := strings.Split(s[1], "-")
	if len(e) == 1 {
		eidx = 0
	}

	gs := strings.Split(gtid, ":")
	if len(gs) < 2 {
		return "", errors.Errorf("incorrect source in gtid set %s", gtid)
	}

	es := strings.Split(gs[1], "-")

	return gs[0] + ":" + es[0] + "-" + e[eidx], nil
}

func reverse(list []string) {
	for i := len(list)/2 - 1; i >= 0; i-- {
		opp := len(list) - 1 - i
		list[i], list[opp] = list[opp], list[i]
	}
}

func getStartGTIDSet(ctx context.Context, s storage.Storage) (string, error) {
	// Prefer reading from xtrabackup_binlog_info if it exists
	list, err := s.ListObjects(ctx, "xtrabackup_binlog_info")
	if err != nil {
		return "", errors.Wrapf(err, "list xtrabackup_binlog_info objects")
	}
	if len(list) > 0 {
		sort.Strings(list)
		obj, err := s.GetObject(ctx, list[0])
		if err != nil {
			return "", errors.Wrapf(err, "get xtrabackup_binlog_info object")
		}
		defer obj.Close() //nolint:errcheck

		content, err := getDecompressedContent(ctx, obj, "xtrabackup_binlog_info")
		if err != nil {
			return "", errors.Wrapf(err, "get decompressed content for xtrabackup_binlog_info")
		}
		tokens := strings.Split(strings.TrimSpace(string(content)), "\t")
		if len(tokens) != 3 {
			return "", errors.Errorf("malformed xtrabackup_binlog_info: %d tokens", len(tokens))
		}
		return tokens[2], nil
	}

	log.Println("no xtrabackup_binlog_info objects found, falling back to xtrabackup_info")

	// Fallback to xtrabackup_info if xtrabackup_binlog_info is not found
	list, err = s.ListObjects(ctx, "xtrabackup_info")
	if err != nil {
		return "", errors.Wrapf(err, "list xtrabackup_info objects")
	}
	if len(list) == 0 {
		return "", errors.New("neither xtrabackup_binlog_info nor xtrabackup_info objects found")
	}

	sort.Strings(list)
	obj, err := s.GetObject(ctx, list[0])
	if err != nil {
		return "", errors.Wrapf(err, "get xtrabackup_info object")
	}
	defer obj.Close() //nolint:errcheck

	content, err := getDecompressedContent(ctx, obj, "xtrabackup_info")
	if err != nil {
		return "", errors.Wrapf(err, "get decompressed content for xtrabackup_info")
	}

	gtid, err := getGTIDFromXtrabackup(content)
	if err != nil {
		return "", errors.Wrapf(err, "get gtid from xtrabackup_info")
	}
	return gtid, nil
}

func getGTIDFromXtrabackup(content []byte) (string, error) {
	sep := []byte("GTID of the last")
	_, after, ok := bytes.Cut(content, sep)
	if !ok {
		return "", errors.New("no gtid data in backup")
	}
	newOut := after
	e := bytes.Index(newOut, []byte("'\n"))
	if e == -1 {
		return "", errors.New("can't find gtid data in backup")
	}

	se := bytes.Index(newOut, []byte("'"))
	set := newOut[se+1 : e]

	return string(set), nil
}

func getBackupTimelineUUID(ctx context.Context, s storage.Storage) (string, error) {
	// Try reading galera-gtid from sst_info
	if uuid, err := readUUIDFromSSTInfo(ctx, s); err == nil {
		return uuid, nil
	} else if !errors.Is(err, storage.ErrObjectNotFound) {
		return "", errors.Wrap(err, "read sst_info")
	}

	// sst_info not found, try reading from .meta.json created
	// by xtrabackup sidecar
	if meta, err := readBackupMeta(ctx, s); err == nil {
		return meta.ClusterUUID, nil
	} else if !errors.Is(err, storage.ErrObjectNotFound) {
		return "", errors.Wrap(err, "read backup meta")
	}

	return "", errors.New(
		"no Galera state info in backup (none of sst_info, .meta.json); " +
			"PITR cannot determine timeline identity — backup may have been produced by an " +
			"older sidecar that did not capture wsrep_cluster_state_uuid")
}

func readUUIDFromSSTInfo(ctx context.Context, s storage.Storage) (string, error) {
	prev := s.GetPrefix()
	defer s.SetPrefix(prev)

	s.SetPrefix(strings.TrimSuffix(prev, "/") + ".sst_info/")

	list, err := s.ListObjects(ctx, "sst_info")
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", storage.ErrObjectNotFound
	}
	sort.Strings(list)
	obj, err := s.GetObject(ctx, list[0])
	if err != nil {
		return "", err
	}
	defer obj.Close() //nolint:errcheck

	content, err := getDecompressedContent(ctx, obj, "sst_info")
	if err != nil {
		return "", err
	}

	galeraGtid, err := parseGTIDFromSSTInfoContent(content)
	if err != nil {
		return "", errors.Wrapf(err, "parse gtid from sst_info content")
	}
	return strings.Split(galeraGtid, ":")[0], nil
}

// readBackupMeta reads the backup .meta.json file created by the operator.
func readBackupMeta(ctx context.Context, s storage.Storage) (*xbserver.BackupMeta, error) {
	prev := s.GetPrefix()
	defer s.SetPrefix(prev)

	metaObject := strings.TrimSuffix(prev, "/") + ".meta.json"
	s.SetPrefix("")

	obj, err := s.GetObject(ctx, metaObject)
	if err != nil {
		return nil, err
	}
	defer obj.Close() //nolint:errcheck

	content, err := io.ReadAll(obj)
	if err != nil {
		return nil, err
	}

	meta := &xbserver.BackupMeta{}
	if err := json.Unmarshal(content, meta); err != nil {
		return nil, errors.Wrapf(err, "unmarshal meta.json")
	}
	return meta, nil
}

func parseGTIDFromSSTInfoContent(content []byte) (string, error) {
	sep := []byte("galera-gtid=")
	_, after, ok := bytes.Cut(content, sep)
	if !ok {
		return "", errors.New("no gtid data in backup")
	}
	newOut := after
	before, _, ok := bytes.Cut(newOut, []byte("\n"))
	if !ok {
		return "", errors.New("can't find gtid data in backup")
	}
	return string(before), nil
}
