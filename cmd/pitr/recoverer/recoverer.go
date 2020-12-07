package recoverer

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/db"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/network"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/storage"

	"github.com/pkg/errors"
)

type Recoverer struct {
	db             *db.PXC
	recoverTime    string
	storage        Storage
	pxcUser        string
	pxcPass        string
	recoverType    RecoverType
	pxcServiceName string
	binlogs        []string
	gtidSet        string
	s3Prefix       string
	startGTID      string
}

type Config struct {
	PXCServiceName string
	PXCUser        string
	PXCPass        string
	BackupStorage  S3
	RecoverTime    string
	RecoverType    string
	GTIDSet        string
	BinlogStorage  S3
}

type S3 struct {
	Endpoint    string
	AccessKeyID string
	AccessKey   string
	Region      string
	BackupDest  string
	BucketURL   string
}

type Storage interface {
	GetObject(objectName string) (io.Reader, error)
	PutObject(name string, data io.Reader) error
	ListObjects(prefix string) []string
}

type RecoverType string

func New(c Config) (*Recoverer, error) {
	bucketArr := strings.Split(c.BinlogStorage.BucketURL, "/")
	s3Prefix := ""
	if len(bucketArr) > 1 {
		s3Prefix = strings.TrimPrefix(c.BinlogStorage.BucketURL, bucketArr[0]+"/")
	}
	s3, err := storage.NewS3(strings.TrimPrefix(strings.TrimPrefix(c.BinlogStorage.Endpoint, "https://"), "http://"), c.BinlogStorage.AccessKeyID, c.BinlogStorage.AccessKey, bucketArr[0], c.BinlogStorage.Region, strings.HasPrefix(c.BinlogStorage.Endpoint, "https"))
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
		s3Prefix:       s3Prefix,
		gtidSet:        c.GTIDSet,
	}, nil
}

func getStartGTIDSet(c S3) (string, error) {
	bucketArr := strings.Split(c.BackupDest, "/")
	if len(bucketArr) < 2 {
		return "", errors.New("parsing bucket")
	}
	prefix := strings.TrimLeft(c.BackupDest, bucketArr[0]+"/")
	bucket := bucketArr[0]
	s3, err := storage.NewS3(strings.TrimPrefix(strings.TrimPrefix(c.Endpoint, "https://"), "http://"), c.AccessKeyID, c.AccessKey, bucket, c.Region, strings.HasPrefix(c.Endpoint, "https"))
	if err != nil {
		return "", errors.Wrap(err, "new storage manager")
	}

	infoObj, err := s3.GetObject(prefix + "/xtrabackup_info.00000000000000000000") //TODO: work with compressed file
	if err != nil {
		return "", errors.Wrap(err, "get object")
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
	host, err := network.GetPXCLastHost(r.pxcServiceName)
	if err != nil {
		return errors.Wrap(err, "get host")
	}
	pxc, err := db.NewPXC(host, r.pxcUser, r.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)

	}
	r.db = pxc

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

func (r *Recoverer) recover() error {
	host, err := network.GetPXCLastHost(r.pxcServiceName)
	if err != nil {
		return errors.Wrap(err, "get host")
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

		format := "2006-01-02 15:04:05"
		t, err := time.Parse(format, r.recoverTime)
		if err != nil {
			return errors.Wrap(err, "parse date")
		}
		endTime = t
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

		cmdString := "mysqlbinlog" + flags + " - | mysql -h" + host + " -u" + r.pxcUser
		cmd := exec.Command("sh", "-c", cmdString)

		cmd.Stdin = binlogObj
		var outb, errb bytes.Buffer
		cmd.Stdout = &outb
		cmd.Stderr = &errb
		err = cmd.Run()
		if err != nil {
			return errors.Wrap(err, "cmd run")
		}

		if errb.Bytes() != nil {
			log.Println(errors.Errorf("cmd error: %s, stdout: %s", errb.String(), outb.String()))
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
	list := r.storage.ListObjects(r.s3Prefix + "/binlog_")
	binlogs := []string{}
	for _, binlog := range reverseArr(list) {
		binlogs = append(binlogs, binlog)

		infoObj, err := r.storage.GetObject(r.s3Prefix + binlog + "-gtid-set")
		if err != nil {
			return errors.Wrap(err, "get object with gtid set")
		}
		content, err := ioutil.ReadAll(infoObj)
		if err != nil {
			return errors.Wrap(err, "get object")
		}

		isSubset, err := r.db.IsGTIDSubset(r.startGTID, string(content))
		if err != nil {
			return errors.Wrapf(err, "is '%s' subset of '%s", r.startGTID, string(content))
		}

		if isSubset {
			break
		}
	}

	r.binlogs = reverseArr(binlogs)

	return nil
}

func reverseArr(arr []string) []string {
	newArr := make([]string, len(arr))
	for k, v := range arr {
		newKey := len(newArr) - k - 1
		newArr[newKey] = v
	}
	return newArr
}
