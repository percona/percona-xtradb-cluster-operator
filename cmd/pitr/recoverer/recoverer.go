package recoverer

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
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
}

type Config struct {
	PXCServiceName string `env:"PXC_SERVICE,required"`
	PXCUser        string `env:"PXC_USER,required"`
	PXCPass        string `env:"PXC_PASS,required"`
	BackupStorage  BackupS3
	RecoverTime    string `env:"PITR_DATE"`
	RecoverType    string `env:"PITR_RECOVERY_TYPE,required"`
	GTIDSet        string `env:"PITR_GTID_SET"`
	BinlogStorage  BinlogS3
}

type BackupS3 struct {
	Endpoint    string `env:"ENDPOINT,required"`
	AccessKeyID string `env:"ACCESS_KEY_ID,required"`
	AccessKey   string `env:"SECRET_ACCESS_KEY,required"`
	Region      string `env:"DEFAULT_REGION,required"`
	BackupDest  string `env:"S3_BUCKET_URL,required"`
}

type BinlogS3 struct {
	Endpoint    string `env:"BINLOG_S3_ENDPOINT,required"`
	AccessKeyID string `env:"BINLOG_ACCESS_KEY_ID,required"`
	AccessKey   string `env:"BINLOG_SECRET_ACCESS_KEY,required"`
	Region      string `env:"BINLOG_S3_REGION,required"`
	BucketURL   string `env:"BINLOG_S3_BUCKET_URL,required"`
}

type RecoverType string

func New(c Config) (*Recoverer, error) {
	bucket, prefix, err := getBucketAndPrefix(c.BinlogStorage.BucketURL)
	if err != nil {
		return nil, errors.Wrap(err, "get bucket and prefix")
	}

	s3, err := storage.NewS3(strings.TrimPrefix(strings.TrimPrefix(c.BinlogStorage.Endpoint, "https://"), "http://"), c.BinlogStorage.AccessKeyID, c.BinlogStorage.AccessKey, bucket, prefix, c.BinlogStorage.Region, strings.HasPrefix(c.BinlogStorage.Endpoint, "https"))
	if err != nil {
		return nil, errors.Wrap(err, "new storage manager")
	}

	startGTID, err := getStartGTIDSet(c.BackupStorage)
	if err != nil {
		return nil, errors.Wrap(err, "get start GTID")
	}

	return &Recoverer{
		storage:        s3,
		recoverTime:    c.RecoverTime,
		pxcUser:        c.PXCUser,
		pxcPass:        c.PXCPass,
		pxcServiceName: c.PXCServiceName,
		recoverType:    RecoverType(c.RecoverType),
		startGTID:      startGTID,
		gtidSet:        c.GTIDSet,
	}, nil
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

func getStartGTIDSet(c BackupS3) (string, error) {
	bucketArr := strings.Split(c.BackupDest, "/")
	if len(bucketArr) < 2 {
		return "", errors.New("parsing bucket")
	}

	prefix := strings.TrimPrefix(c.BackupDest, bucketArr[0]+"/") + "/"

	s3, err := storage.NewS3(strings.TrimPrefix(strings.TrimPrefix(c.Endpoint, "https://"), "http://"), c.AccessKeyID, c.AccessKey, bucketArr[0], prefix, c.Region, strings.HasPrefix(c.Endpoint, "https"))
	if err != nil {
		return "", errors.Wrap(err, "new storage manager")
	}

	infoObj, err := s3.GetObject("xtrabackup_info.00000000000000000000") //TODO: work with compressed file
	if err != nil {
		return "", errors.Wrapf(err, "get %s info", prefix)
	}

	lastGTID, err := getLastBackupGTID(infoObj)
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

func (r *Recoverer) Run() error {
	host, err := pxc.GetPXCLastHost(r.pxcServiceName)
	if err != nil {
		return errors.Wrap(err, "get host")
	}
	r.db, err = pxc.NewPXC(host, r.pxcUser, r.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)

	}

	err = r.setBinlogs()
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}

	err = r.recover()
	if err != nil {
		return errors.Wrap(err, "recover")
	}

	return nil
}

func (r *Recoverer) recover() (err error) {
	err = r.db.DropCollectorFunctions()
	if err != nil {
		return errors.Wrap(err, "drop collector funcs")
	}
	flags := ""
	endTime := time.Time{}
	// TODO: add logic for all types
	switch r.recoverType {
	case Skip:
		flags = " --exclude-gtids=" + r.gtidSet
	case Transaction:
	case Date:
		flags = ` --stop-datetime="` + r.recoverTime + `"`

		const format = "2006-01-02 15:04:05"
		endTime, err = time.Parse(format, r.recoverTime)
		if err != nil {
			return errors.Wrap(err, "parse date")
		}
	case Latest:
	default:
		return errors.New("wrong recover type")
	}

	for _, binlog := range r.binlogs {
		log.Println("working with", binlog)
		if r.recoverType == Date {
			binlogArr := strings.Split(binlog, "_")
			if len(binlogArr) < 2 {
				return errors.New("get timestamp from binlog name")
			}
			binlogTime, err := strconv.ParseInt(binlogArr[1], 10, 64)
			if err != nil {
				return errors.Wrap(err, "get binlog time")
			}
			if binlogTime > endTime.Unix() {
				return nil
			}
		}

		binlogObj, err := r.storage.GetObject(binlog)
		if err != nil {
			return errors.Wrap(err, "get obj")
		}

		err = os.Setenv("MYSQL_PWD", os.Getenv("PXC_PASS"))
		if err != nil {
			return errors.Wrap(err, "set mysql pwd env var")
		}

		cmdString := "mysqlbinlog" + flags + " - | mysql -h" + r.db.GetHost() + " -u" + r.pxcUser
		cmd := exec.Command("sh", "-c", cmdString)

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

func getLastBackupGTID(infoObj io.Reader) (string, error) {
	sep := []byte("GTID of the last")

	content, err := ioutil.ReadAll(infoObj)
	if err != nil {
		return "", errors.Wrap(err, "read object")
	}

	startIndex := bytes.Index(content, sep)
	if startIndex == -1 {
		return "", errors.New("no gtid data in backup")
	}
	newOut := content[startIndex+len(sep):]
	e := bytes.Index(newOut, []byte("'\n"))
	if e == -1 {
		return "", errors.New("can't find gtid data in backup")
	}
	content = content[:e]

	se := bytes.Index(newOut, []byte("'"))
	set := newOut[se+1 : e]

	return string(set), nil
}

func (r *Recoverer) setBinlogs() error {
	list, err := r.storage.ListObjects("binlog_")
	if err != nil {
		return errors.Wrap(err, "list objects with prefix 'binlog_'")
	}
	reverse(list)
	binlogs := []string{}
	for _, binlog := range list {
		if strings.Contains(binlog, "-gtid-set") {
			continue
		}
		binlogs = append(binlogs, binlog)

		infoObj, err := r.storage.GetObject(binlog + "-gtid-set")
		if err != nil {
			return errors.Wrapf(err, "get %s object with gtid set", binlog)
		}
		content, err := ioutil.ReadAll(infoObj)
		if err != nil {
			return errors.Wrapf(err, "read %s gtid-set object", binlog)
		}

		isSubset, err := r.db.IsGTIDSubset(r.startGTID, string(content))
		if err != nil {
			return errors.Wrapf(err, "check if '%s' is a subset of '%s", r.startGTID, string(content))
		}

		if isSubset {
			break
		}
	}
	if len(binlogs) == 0 {
		return errors.Errorf("no objects for prefix %s", "binlog_")
	}
	reverse(binlogs)
	r.binlogs = binlogs

	return nil
}

func reverse(list []string) {
	for i := len(list)/2 - 1; i >= 0; i-- {
		opp := len(list) - 1 - i
		list[i], list[opp] = list[opp], list[i]
	}
}
