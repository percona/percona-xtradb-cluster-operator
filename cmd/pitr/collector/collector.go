package collector

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/storage"
)

type Collector struct {
	db             *pxc.PXC
	storage        storage.Storage
	lastSet        string // last uploaded binary logs set
	pxcServiceName string // k8s service name for PXC, its for get correct host for connection
	pxcUser        string // user for connection to PXC
	pxcPass        string // password for connection to PXC
	bufferSize     int64  // size of uploading buffer
	bucketPrefix   string // prefix for files in bucket. For example bucket-name/{prefix}/binlog-file
}

type Config struct {
	PXCServiceName string
	PXCUser        string
	PXCPass        string
	S3Endpoint     string
	S3AccessKeyID  string
	S3AccessKey    string
	S3BucketURL    string
	S3Region       string
	BufferSize     int64
}

const (
	lastSetFileName string = "last-binlog-set" // name for object where the last binlog set will stored
	gtidPostfix     string = "-gtid-set"       // filename postfix for files with GTID set
)

func New(c Config) (*Collector, error) {
	bucketArr := strings.Split(c.S3BucketURL, "/")

	s3, err := storage.NewS3(strings.TrimPrefix(strings.TrimPrefix(c.S3Endpoint, "https://"), "http://"), c.S3AccessKeyID, c.S3AccessKey, bucketArr[0], c.S3Region, strings.HasPrefix(c.S3Endpoint, "https"))
	if err != nil {
		return nil, errors.Wrap(err, "new storage manager")
	}
	prefix := ""
	// if c.S3BucketURL looks like "my-bucket/data/more-data" we need prefix to be "data/more-data/"
	if len(bucketArr) > 0 {
		prefix = strings.TrimPrefix(c.S3BucketURL, bucketArr[0]+"/") + "/"
	}
	// get last binlog set stored on S3
	lastSetObject, err := s3.GetObject(prefix + lastSetFileName)
	if err != nil {
		return nil, errors.Wrap(err, "get last set content")
	}
	lastSet, err := ioutil.ReadAll(lastSetObject)
	if err != nil && minio.ToErrorResponse(errors.Cause(err)).Code != "NoSuchKey" {
		return nil, errors.Wrap(err, "read last gtid set")
	}

	return &Collector{
		storage:        s3,
		lastSet:        string(lastSet),
		pxcUser:        c.PXCUser,
		pxcPass:        c.PXCPass,
		pxcServiceName: c.PXCServiceName,
		bucketPrefix:   prefix,
	}, nil
}

func (c *Collector) Run() error {
	err := c.newDB()
	if err != nil {
		return errors.Wrap(err, "new db connection")
	}
	defer c.closeDB()

	err = c.CollectBinLogs()
	if err != nil {
		return errors.Wrap(err, "collect binlog files")
	}

	return nil
}

func (c *Collector) newDB() error {
	host, err := pxc.GetPXCLastHost(c.pxcServiceName)
	if err != nil {
		return errors.Wrap(err, "get host")
	}
	c.db, err = pxc.NewPXC(host, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)

	}

	return nil
}

func (c *Collector) closeDB() error {
	return c.db.Close()
}

func (c *Collector) CollectBinLogs() error {
	list, err := c.db.GetBinLogList()
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}
	// get last uploaded binlog file name
	binlogName, err := c.db.GetBinLogName(c.lastSet)
	if err != nil {
		return errors.Wrap(err, "get binlog name by set")
	}

	upload := false
	// if there are no uploaded files we going to upload every binlog file
	if len(binlogName) == 0 {
		upload = true
	}

	for _, binlog := range list {
		binlogSet := ""
		// this check is for uploading starting from needed file
		if binlog == binlogName {
			binlogSet, err = c.db.GetGTIDSet(binlog)
			if err != nil {
				return errors.Wrap(err, "get binlog gtid set")
			}
			if c.lastSet != binlogSet {
				upload = true
			}
		}
		if upload {
			err = c.manageBinlog(binlog)
			if err != nil {
				return errors.Wrap(err, "manage binlog")
			}
		}
		// need this for start uploading files that goes after current
		if c.lastSet == binlogSet {
			upload = true
		}
	}

	return nil
}

type reader struct {
	r io.Reader
}

func (r *reader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func mergeErrors(firstErr, secErr error) error {
	if firstErr != nil && secErr != nil {
		return errors.New(firstErr.Error() + "; " + secErr.Error())
	}
	if firstErr != nil && secErr == nil {
		return firstErr
	}
	if firstErr == nil && secErr != nil {
		return secErr
	}
	return nil
}

func (c *Collector) manageBinlog(binlog string) (err error) {
	set, err := c.db.GetGTIDSet(binlog)
	if err != nil {
		return errors.Wrap(err, "get GTID set")
	}
	if len(set) == 0 {
		return nil
	}

	binlogTmstmp, err := c.db.GetBinLogFirstTimestamp(binlog)
	if err != nil {
		return errors.Wrapf(err, "get first timestamp for %s", binlog)
	}

	binlogName := "binlog_" + binlogTmstmp + "_" + fmt.Sprintf("%x", md5.Sum([]byte(set)))
	if len(c.bucketPrefix) > 0 {
		binlogName = c.bucketPrefix + binlogName
	}

	var setBuffer bytes.Buffer
	setBuffer.WriteString(set)

	tmpDir := os.TempDir() + "/"

	err = os.Remove(tmpDir + binlog)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove temp file")
	}

	err = syscall.Mkfifo(tmpDir+binlog, 0666)
	if err != nil {
		return errors.Wrap(err, "make named pipe file error")
	}

	file, err := os.OpenFile(tmpDir+binlog, syscall.O_NONBLOCK, os.ModeNamedPipe)
	if err != nil {
		return errors.Wrap(err, "open named pipe file error")
	}
	defer func() {
		if err != nil {
			return
		}
		errC := file.Close()
		if errC != nil {
			err = mergeErrors(err, errors.Wrapf(errC, "close tmp file for %s", binlog))
			return
		}
		errR := os.Remove(tmpDir + binlog)
		if errR != nil {
			err = mergeErrors(err, errors.Wrapf(errR, "remove tmp file for %s", binlog))
			return
		}
	}()
	err = os.Setenv("MYSQL_PWD", os.Getenv("PXC_PASS"))
	if err != nil {
		return errors.Wrap(err, "set mysql pwd env var")
	}

	cmd := exec.Command("mysqlbinlog", "-R", "--raw", "-h"+c.db.GetHost(), "-u"+c.pxcUser, "--result-file="+tmpDir, binlog)

	errOut, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrap(err, "get mysqlbinlog stderr pipe")
	}

	err = cmd.Start()
	if err != nil {
		return errors.Wrap(err, "run mysqlbinlog command")
	}

	data := reader{file}
	err = c.storage.PutObject(binlogName, &data)
	if err != nil {
		return errors.Wrapf(err, "put %s object", binlog)
	}
	stdErr, err := ioutil.ReadAll(errOut)
	if err != nil {
		return errors.Wrap(err, "read mysqlbinlog error output")
	}

	cmd.Wait()

	if stdErr != nil && string(bytes.TrimRight(stdErr, "\n")) != pxc.UsingPassErrorMessage {
		return errors.Errorf("mysqlbinlog: %s", stdErr)
	}

	err = c.storage.PutObject(binlogName+gtidPostfix, &setBuffer)
	if err != nil {
		return errors.Wrap(err, "put gtid-set object")
	}

	setBuffer.WriteString(set)
	err = c.storage.PutObject(c.bucketPrefix+lastSetFileName, &setBuffer)
	if err != nil {
		return errors.Wrap(err, "put last-set object")
	}
	c.lastSet = set

	return nil
}
