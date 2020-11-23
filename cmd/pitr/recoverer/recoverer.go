package recoverer

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/storage"
	"github.com/pkg/errors"
)

type Recoverer struct {
	recoverTime      int64
	storage          Storage
	localDest        string
	pxcUser          string
	pxcHost          string
	pxcPass          string
	recoverType      RecoverType
	pxcServiceName   string
	binlogs          []string
	backupNAme       string
	excludingGTIDSet string
	s3BucketName     string
}

type Config struct {
	PXCServiceName string
	PXCUser        string
	PXCPass        string
	S3Endpoint     string
	S3AccessKeyID  string
	S3AccessKey    string
	S3BucketName   string
	S3Region       string
	RecoverTime    int64
	RecoverType    string
	BackupName     string
}

type Storage interface {
	GetObject(objectName string) ([]byte, error)
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
		pxcPass:        c.PXCPass,
		pxcServiceName: c.PXCServiceName,
		recoverType:    RecoverType(c.RecoverType),
		backupNAme:     c.BackupName,
		localDest:      "/tmp/",
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
	startGTID, err := r.GetLastBackupGTID()
	if err != nil {
		return errors.Wrap(err, "get last gtid")
	}
	err = r.SetBinlogs(startGTID)
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}
	err = r.Recover()
	if err != nil {
		return errors.Wrap(err, "recover")
	}
	return nil
}

func (r *Recoverer) Recover() error {
	host, err := r.getHost()
	if err != nil {
		return errors.Wrap(err, "get host")
	}
	cfgCmd := exec.Command("sh", "-c", `mc -C `+r.localDest+`mc config host add dest "${ENDPOINT:-https://s3.amazonaws.com}" "$ACCESS_KEY_ID" "$SECRET_ACCESS_KEY"`)
	err = cfgCmd.Run()
	if err != nil {
		return errors.Wrap(err, "update mc config")
	}
	flags := ""

	// TODO: add logic for all types
	switch r.recoverType {
	case Skip:
		flags = "--exclude-gtids=" + r.excludingGTIDSet
	case Transaction:
	case Date:
	case Latest:
	}

	for _, binlog := range r.binlogs {
		err = r.DownloadBinlog(binlog)
		if err != nil {
			return errors.Wrap(err, "download binlog")
		}

		cmdString := "mc -C " + r.localDest + "mc cat " + r.s3BucketName + "/" + binlog + " | mysqlbinlog " + flags + " - | mysql -h" + host + " -u" + r.pxcUser + " -p$PXC_PASS"
		cmd := exec.Command("sh", "-c", cmdString)
		var outb, errb bytes.Buffer
		cmd.Stdout = &outb
		cmd.Stderr = &errb
		err := cmd.Run()
		if err != nil {
			return errors.Wrap(err, "run cmd")
		}
		if errb.Bytes() != nil {
			errors.Errorf("cmd error: %s", errb.String())
		}
		err = os.Remove(r.localDest + binlog)
		if err != nil {
			return errors.Wrap(err, "remove binlog")
		}
	}

	return nil
}

func (r *Recoverer) DownloadBinlog(binlog string) error {
	content, err := r.storage.GetObject(binlog)
	if err != nil {
		return errors.Wrap(err, "get object")
	}
	f, err := os.Create(r.localDest + binlog)
	if err != nil {
		return errors.Wrap(err, "create file")
	}
	defer f.Close()
	_, err = f.Write(content)
	if err != nil {
		return errors.Wrap(err, "write content to file")
	}
	return nil
}

func (r *Recoverer) GetLastBackupGTID() (int64, error) {
	sep := []byte("GTID of the last")

	o, err := r.storage.GetObject(r.backupNAme + "/xtrabackup_info.lz4.00000000000000000000")
	if err != nil {
		return 0, errors.Wrap(err, "get object")
	}

	startIndex := bytes.Index(o, sep)
	if startIndex == -1 {
		return 0, errors.New("no gtid  data in backup")
	}
	newOut := o[startIndex+len(sep):]
	e := bytes.Index(newOut, []byte("'\n"))
	if e == -1 {
		return 0, errors.New("cant find gtid  data in backup")
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

func (r *Recoverer) SetBinlogs(startID int64) error {
	saveBinlog := false
	for _, binlog := range r.storage.ListObjects("") {
		if strings.Contains(binlog, "binlog.") && !strings.Contains(binlog, "gtid-set") {
			if saveBinlog {
				r.binlogs = append(r.binlogs, binlog)
				continue
			}
			o, err := r.storage.GetObject(binlog + "-gtid-set")
			if err != nil {
				return errors.Wrap(err, "get object with gtid set")
			}

			oArr := bytes.Split(o, []byte(":"))
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
