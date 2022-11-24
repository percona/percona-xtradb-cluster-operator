package recoverer

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/storage"

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
	startGTID      string
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

func (c Config) storages() (storage.Storage, storage.Storage, error) {
	var binlogStorage, defaultStorage storage.Storage
	switch c.StorageType {
	case "s3":
		bucket, prefix, err := getBucketAndPrefix(c.BinlogStorageS3.BucketURL)
		if err != nil {
			return nil, nil, errors.Wrap(err, "get bucket and prefix")
		}
		binlogStorage, err = storage.NewS3(c.BinlogStorageS3.Endpoint, c.BinlogStorageS3.AccessKeyID, c.BinlogStorageS3.AccessKey, bucket, prefix, c.BinlogStorageS3.Region, c.VerifyTLS)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new s3 storage")
		}

		bucket, prefix, err = getBucketAndPrefix(c.BackupStorageS3.BackupDest)
		if err != nil {
			return nil, nil, errors.Wrap(err, "get bucket and prefix")
		}
		prefix = prefix[:len(prefix)-1]
		defaultStorage, err = storage.NewS3(c.BackupStorageS3.Endpoint, c.BackupStorageS3.AccessKeyID, c.BackupStorageS3.AccessKey, bucket, prefix+".sst_info/", c.BackupStorageS3.Region, c.VerifyTLS)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new storage manager")
		}
	case "azure":
		var err error
		container, prefix := getContainerAndPrefix(c.BinlogStorageAzure.ContainerPath)
		binlogStorage, err = storage.NewAzure(c.BinlogStorageAzure.AccountName, c.BinlogStorageAzure.AccountKey, c.BinlogStorageAzure.Endpoint, container, prefix)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new azure storage")
		}
		defaultStorage, err = storage.NewAzure(c.BackupStorageAzure.AccountName, c.BackupStorageAzure.AccountKey, c.BackupStorageAzure.Endpoint, c.BackupStorageAzure.ContainerName, c.BackupStorageAzure.BackupDest+".sst_info/")
		if err != nil {
			return nil, nil, errors.Wrap(err, "new azure storage")
		}
	default:
		return nil, nil, errors.New("unknown STORAGE_TYPE")
	}
	return binlogStorage, defaultStorage, nil
}

type BackupS3 struct {
	Endpoint    string `env:"ENDPOINT" envDefault:"s3.amazonaws.com"`
	AccessKeyID string `env:"ACCESS_KEY_ID,required"`
	AccessKey   string `env:"SECRET_ACCESS_KEY,required"`
	Region      string `env:"DEFAULT_REGION,required"`
	BackupDest  string `env:"S3_BUCKET_URL,required"`
}

type BackupAzure struct {
	Endpoint      string `env:"AZURE_ENDPOINT,required"`
	ContainerName string `env:"AZURE_CONTAINER_NAME,required"`
	StorageClass  string `env:"AZURE_STORAGE_CLASS"`
	AccountName   string `env:"AZURE_STORAGE_ACCOUNT,required"`
	AccountKey    string `env:"AZURE_ACCESS_KEY,required"`
	BackupDest    string `env:"BACKUP_PATH,required"`
}

type BinlogS3 struct {
	Endpoint    string `env:"BINLOG_S3_ENDPOINT" envDefault:"s3.amazonaws.com"`
	AccessKeyID string `env:"BINLOG_ACCESS_KEY_ID,required"`
	AccessKey   string `env:"BINLOG_SECRET_ACCESS_KEY,required"`
	Region      string `env:"BINLOG_S3_REGION,required"`
	BucketURL   string `env:"BINLOG_S3_BUCKET_URL,required"`
}

type BinlogAzure struct {
	Endpoint      string `env:"BINLOG_AZURE_ENDPOINT,required"`
	ContainerPath string `env:"BINLOG_AZURE_CONTAINER_PATH,required"`
	StorageClass  string `env:"BINLOG_AZURE_STORAGE_CLASS"`
	AccountName   string `env:"BINLOG_AZURE_STORAGE_ACCOUNT,required"`
	AccountKey    string `env:"BINLOG_AZURE_ACCESS_KEY,required"`
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

	binlogStorage, storage, err := c.storages()
	if err != nil {
		return nil, errors.Wrap(err, "new binlog storage manager")
	}

	startGTID, err := getStartGTIDSet(ctx, storage)
	if err != nil {
		return nil, errors.Wrap(err, "get start GTID")
	}

	if c.RecoverType == string(Transaction) {
		gtidSplitted := strings.Split(startGTID, ":")
		if len(gtidSplitted) != 2 {
			return nil, errors.New("Invalid start gtidset provided")
		}
		lastSetIdx := 1
		setSplitted := strings.Split(gtidSplitted[1], "-")
		if len(setSplitted) == 1 {
			lastSetIdx = 0
		}
		lastSet := setSplitted[lastSetIdx]
		lastSetInt, err := strconv.ParseInt(lastSet, 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "failed to cast last set value to in")
		}
		transactionNum, err := strconv.ParseInt(strings.Split(c.GTID, ":")[1], 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse transaction num to restore")
		}
		if transactionNum < lastSetInt {
			return nil, errors.New("Can't restore to transaction before backup")
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
		gtid:           c.GTID,
		verifyTLS:      c.VerifyTLS,
	}, nil
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

func getStartGTIDSet(ctx context.Context, s storage.Storage) (string, error) {
	sstInfo, err := s.ListObjects(ctx, "sst_info")
	if err != nil {
		return "", errors.Wrapf(err, "list objects")
	}
	if len(sstInfo) == 0 {
		return "", errors.New("no info files in sst dir")
	}
	sort.Strings(sstInfo)

	sstInfoObj, err := s.GetObject(ctx, sstInfo[0])
	if err != nil {
		return "", errors.Wrapf(err, "get object")
	}
	defer sstInfoObj.Close()

	s.SetPrefix(strings.TrimSuffix(s.GetPrefix(), ".sst_info/") + "/")
	xtrabackupInfo, err := s.ListObjects(ctx, "xtrabackup_info")
	if err != nil {
		return "", errors.Wrapf(err, "list objects")
	}
	if len(xtrabackupInfo) == 0 {
		return "", errors.New("no info files in backup")
	}
	sort.Strings(xtrabackupInfo)

	xtrabackupInfoObj, err := s.GetObject(ctx, xtrabackupInfo[0])
	if err != nil {
		return "", errors.Wrapf(err, "get object")
	}

	lastGTID, err := getLastBackupGTID(ctx, sstInfoObj, xtrabackupInfoObj)
	if err != nil {
		return "", errors.Wrap(err, "get last backup gtid")
	}

	return lastGTID, nil
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
		r.recoverFlag = " --exclude-gtids=" + r.gtid
	case Transaction:
		r.recoverFlag = " --exclude-gtids=" + r.gtidSet
	case Date:
		r.recoverFlag = ` --stop-datetime="` + r.recoverTime + `"`

		const format = "2006-01-02 15:04:05"
		endTime, err := time.Parse(format, r.recoverTime)
		if err != nil {
			return errors.Wrap(err, "parse date")
		}
		r.recoverEndTime = endTime
	case Latest:
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
	err = r.db.DropCollectorFunctions(ctx)
	if err != nil {
		return errors.Wrap(err, "drop collector funcs")
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
				return nil
			}
		}

		binlogObj, err := r.storage.GetObject(ctx, binlog)
		if err != nil {
			return errors.Wrap(err, "get obj")
		}

		err = os.Setenv("MYSQL_PWD", os.Getenv("PXC_PASS"))
		if err != nil {
			return errors.Wrap(err, "set mysql pwd env var")
		}

		cmdString := "mysqlbinlog --disable-log-bin" + r.recoverFlag + " - | mysql -h" + r.db.GetHost() + " -u" + r.pxcUser
		cmd := exec.CommandContext(ctx, "sh", "-c", cmdString)

		cmd.Stdin = binlogObj
		var outb, errb bytes.Buffer
		cmd.Stdout = &outb
		cmd.Stderr = &errb
		err = cmd.Run()
		if err != nil {
			return errors.Wrapf(err, "cmd run. stderr: %s, stdout: %s", errb.String(), outb.String())
		}
	}

	return nil
}

func getLastBackupGTID(ctx context.Context, sstInfo, xtrabackupInfo io.Reader) (string, error) {
	sstContent, err := getDecompressedContent(ctx, sstInfo, "sst_info")
	if err != nil {
		return "", errors.Wrap(err, "get sst_info content")
	}

	xtrabackupContent, err := getDecompressedContent(ctx, xtrabackupInfo, "xtrabackup_info")
	if err != nil {
		return "", errors.Wrap(err, "get xtrabackup info content")
	}

	sstGTIDset, err := getGTIDFromSSTInfo(sstContent)
	if err != nil {
		return "", err
	}
	currGTID := strings.Split(sstGTIDset, ":")[0]

	set, err := getSetFromXtrabackupInfo(currGTID, xtrabackupContent)
	if err != nil {
		return "", err
	}

	return currGTID + ":" + set, nil
}

func getSetFromXtrabackupInfo(gtid string, xtrabackupInfo []byte) (string, error) {
	gtids, err := getGTIDFromXtrabackup(xtrabackupInfo)
	if err != nil {
		return "", errors.Wrap(err, "get gtid from xtrabackup info")
	}
	for _, v := range strings.Split(gtids, ",") {
		valueSplitted := strings.Split(v, ":")
		if valueSplitted[0] == gtid {
			return valueSplitted[1], nil
		}
	}
	return "", errors.New("can't find current gtid in xtrabackup file")
}

func getGTIDFromXtrabackup(content []byte) (string, error) {
	sep := []byte("GTID of the last")
	startIndex := bytes.Index(content, sep)
	if startIndex == -1 {
		return "", errors.New("no gtid data in backup")
	}
	newOut := content[startIndex+len(sep):]
	e := bytes.Index(newOut, []byte("'\n"))
	if e == -1 {
		return "", errors.New("can't find gtid data in backup")
	}

	se := bytes.Index(newOut, []byte("'"))
	set := newOut[se+1 : e]

	return string(set), nil
}

func getGTIDFromSSTInfo(content []byte) (string, error) {
	sep := []byte("galera-gtid=")
	startIndex := bytes.Index(content, sep)
	if startIndex == -1 {
		return "", errors.New("no gtid data in backup")
	}
	newOut := content[startIndex+len(sep):]
	e := bytes.Index(newOut, []byte("\n"))
	if e == -1 {
		return "", errors.New("can't find gtid data in backup")
	}
	return string(newOut[:e]), nil
}

func getDecompressedContent(ctx context.Context, infoObj io.Reader, filename string) ([]byte, error) {
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

	decContent, err := ioutil.ReadFile(tmpDir + "/" + filename)
	if err != nil {
		return nil, errors.Wrap(err, "read xtrabackup_info file")
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
		content, err := ioutil.ReadAll(infoObj)
		if err != nil {
			return errors.Wrapf(err, "read %s gtid-set object", binlog)
		}
		binlogGTIDSet := string(content)
		log.Println("checking current file", " name ", binlog, " gtid ", binlogGTIDSet)

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
