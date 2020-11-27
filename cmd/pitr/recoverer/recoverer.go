package recoverer

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/storage"
	"github.com/pkg/errors"
)

type Recoverer struct {
	recoverTime    string
	storage        Storage
	pxcUser        string
	pxcPass        string
	recoverType    RecoverType
	pxcServiceName string
	binlogs        []string
	backupName     string
	gtidSet        string
	s3BucketName   string
}

type Config struct {
	PXCServiceName string
	PXCUser        string
	S3Endpoint     string
	S3AccessKeyID  string
	S3AccessKey    string
	S3BucketName   string
	S3Region       string
	RecoverTime    string
	RecoverType    string
	BackupName     string
	GTIDSet        string
}

type Storage interface {
	GetObject(objectName string) (io.Reader, error)
	PutObject(name string, data io.Reader) error
	ListObjects(prefix string) []string
}

type RecoverType string

func New(c Config) (*Recoverer, error) {
	s3, err := storage.NewS3(c.S3Endpoint, c.S3AccessKeyID, c.S3AccessKey, c.S3BucketName, c.S3Region, true)
	if err != nil {
		return nil, errors.Wrap(err, "new storage manager")
	}
	return &Recoverer{
		storage:        s3,
		recoverTime:    c.RecoverTime,
		pxcUser:        c.PXCUser,
		pxcServiceName: c.PXCServiceName,
		recoverType:    RecoverType(c.RecoverType),
		backupName:     c.BackupName,
	}, nil
}

const (
	Latest      RecoverType = "latest"      // recover to the latest existing binlog
	Date        RecoverType = "date"        // recover to exact date
	Transaction RecoverType = "transaction" // recover to needed trunsaction
	Skip        RecoverType = "skip"        // skip transactions
)

func (r *Recoverer) getHost() (string, error) {
	cmd := exec.Command("peer-list", "-on-start=/usr/bin/get-pxc-state", "-service="+r.pxcServiceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, "get output")
	}
	nodes := strings.Split(string(out), "node:")
	sort.Strings(nodes)
	lastHost := ""
	for _, node := range nodes {
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			nodeArr := strings.Split(node, ".")
			lastHost = nodeArr[0]
		}
	}
	if len(lastHost) == 0 {
		return "", errors.New("cant find host")
	}

	return lastHost + "." + r.pxcServiceName, nil
}

func (r *Recoverer) Run() error {
	startGTID, err := r.getLastBackupGTID()
	if err != nil {
		return errors.Wrap(err, "get last gtid")
	}
	err = r.setBinlogs(startGTID)
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
	host, err := r.getHost()
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
			binlogArr := strings.Split(binlog, ":")
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

		cmdString := "cat | mysqlbinlog" + flags + " - | mysql -h" + host + " -u" + r.pxcUser + " -p$PXC_PASS"
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

func (r *Recoverer) getLastBackupGTID() (int64, error) {
	sep := []byte("GTID of the last")

	infoObj, err := r.storage.GetObject(r.backupName + "/xtrabackup_info.lz4.00000000000000000000")
	if err != nil {
		return 0, errors.Wrap(err, "get object")
	}
	content, err := ioutil.ReadAll(infoObj)
	if err != nil {
		return 0, errors.Wrap(err, "read object")
	}
	startIndex := bytes.Index(content, sep)
	if startIndex == -1 {
		return 0, errors.New("no gtid data in backup")
	}
	newOut := content[startIndex+len(sep):]
	e := bytes.Index(newOut, []byte("'\n"))
	if e == -1 {
		return 0, errors.New("cant find gtid data in backup")
	}

	set := newOut[:e]
	setArr := bytes.Split(set, []byte("-"))
	if len(setArr) < 2 {
		return 0, errors.New("cant find lastgtid in backup")
	}
	idBytes := setArr[len(setArr)-1]

	id, err := strconv.ParseInt(string(idBytes), 10, 64)
	if err != nil {
		return 0, errors.New("cant convert last gtid to int64")
	}

	return id, nil
}

func (r *Recoverer) setBinlogs(startID int64) error {
	saveBinlog := false
	for _, binlog := range r.storage.ListObjects("") {
		if strings.Contains(binlog, "binlog") && !strings.Contains(binlog, "gtid-set") {
			if saveBinlog {
				r.binlogs = append(r.binlogs, binlog)
				continue
			}
			infoObj, err := r.storage.GetObject(binlog + "-gtid-set")
			if err != nil {
				return errors.Wrap(err, "get object with gtid set")
			}
			content, err := ioutil.ReadAll(infoObj)
			if err != nil {
				return errors.Wrap(err, "get object")
			}
			oArr := bytes.Split(content, []byte(":"))
			if len(oArr) < 2 {
				return errors.New("cant read gtid set")
			}

			gtidArr := bytes.Split(oArr[1], []byte("-"))
			lastGTIDinSet := gtidArr[0]
			if len(gtidArr) > 1 {
				lastGTIDinSet = gtidArr[1]
			}

			lastID, err := strconv.ParseInt(string(lastGTIDinSet), 10, 64)
			if err != nil {
				return errors.New("cant convert last gtid to int64")
			}
			if startID < lastID {
				saveBinlog = true
				r.binlogs = append(r.binlogs, binlog)
			}
		}
	}

	return nil
}
