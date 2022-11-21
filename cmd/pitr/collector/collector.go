package collector

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

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
	verifyTLS      bool
}

type Config struct {
	PXCServiceName string  `env:"PXC_SERVICE,required"`
	PXCUser        string  `env:"PXC_USER,required"`
	PXCPass        string  `env:"PXC_PASS,required"`
	S3Endpoint     string  `env:"ENDPOINT" envDefault:"s3.amazonaws.com"`
	S3AccessKeyID  string  `env:"ACCESS_KEY_ID,required"`
	S3AccessKey    string  `env:"SECRET_ACCESS_KEY,required"`
	S3BucketURL    string  `env:"S3_BUCKET_URL,required"`
	S3Region       string  `env:"DEFAULT_REGION,required"`
	BufferSize     int64   `env:"BUFFER_SIZE"`
	CollectSpanSec float64 `env:"COLLECT_SPAN_SEC" envDefault:"60"`
	VerifyTLS      bool    `env:"VERIFY_TLS" envDefault:"true"`
}

const (
	lastSetFilePrefix string = "last-binlog-set-" // filename prefix for object where the last binlog set will stored
	gtidPostfix       string = "-gtid-set"        // filename postfix for files with GTID set
)

func New(c Config) (*Collector, error) {
	bucketArr := strings.Split(c.S3BucketURL, "/")
	prefix := ""
	// if c.S3BucketURL looks like "my-bucket/data/more-data" we need prefix to be "data/more-data/"
	if len(bucketArr) > 1 {
		prefix = strings.TrimPrefix(c.S3BucketURL, bucketArr[0]+"/") + "/"
	}
	s3, err := storage.NewS3(strings.TrimPrefix(strings.TrimPrefix(c.S3Endpoint, "https://"), "http://"), c.S3AccessKeyID, c.S3AccessKey, bucketArr[0], prefix, c.S3Region, strings.HasPrefix(c.S3Endpoint, "https"), c.VerifyTLS)
	if err != nil {
		return nil, errors.Wrap(err, "new storage manager")
	}

	return &Collector{
		storage:        s3,
		pxcUser:        c.PXCUser,
		pxcServiceName: c.PXCServiceName,
		verifyTLS:      c.VerifyTLS,
	}, nil
}

func (c *Collector) Run() error {
	err := c.newDB()
	if err != nil {
		return errors.Wrap(err, "new db connection")
	}
	defer c.close()

	// remove last set because we always
	// read it from aws file
	c.lastSet = ""

	err = c.CollectBinLogs()
	if err != nil {
		return errors.Wrap(err, "collect binlog files")
	}

	return nil
}

func (c *Collector) lastGTIDSet(sourceID string) (string, error) {
	// get last binlog set stored on S3
	lastSetObject, err := c.storage.GetObject(lastSetFilePrefix + sourceID)
	if err != nil {
		return "", errors.Wrap(err, "get last set content")
	}
	lastSet, err := ioutil.ReadAll(lastSetObject)
	if err != nil && minio.ToErrorResponse(errors.Cause(err)).Code != "NoSuchKey" {
		return "", errors.Wrap(err, "read last gtid set")
	}
	return string(lastSet), nil
}

func (c *Collector) newDB() error {
	file, err := os.Open("/etc/mysql/mysql-users-secret/xtrabackup")
	if err != nil {
		return errors.Wrap(err, "open file")
	}
	pxcPass, err := ioutil.ReadAll(file)
	if err != nil {
		return errors.Wrap(err, "read password")
	}
	c.pxcPass = string(pxcPass)

	host, err := pxc.GetPXCOldestBinlogHost(c.pxcServiceName, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrap(err, "get host")
	}

	log.Println("Reading binlogs from pxc with hostname=", host)

	c.db, err = pxc.NewPXC(host, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)
	}

	return nil
}

func (c *Collector) close() error {
	return c.db.Close()
}

func (c *Collector) CurrentSourceID(logs []pxc.Binlog) (string, error) {
	var (
		gtidSet string
		err     error
	)
	for i := len(logs) - 1; i >= 0 && gtidSet == ""; i-- {
		gtidSet, err = c.db.GetGTIDSet(logs[i].Name)
		if err != nil {
			return gtidSet, err
		}
	}
	return strings.Split(gtidSet, ":")[0], nil
}

func (c *Collector) removeEmptyBinlogs(logs []pxc.Binlog) ([]pxc.Binlog, error) {
	result := make([]pxc.Binlog, 0)
	for _, v := range logs {
		set, err := c.db.GetGTIDSet(v.Name)
		if err != nil {
			return nil, errors.Wrap(err, "get GTID set")
		}
		// we don't upload binlog without gtid
		// because it is empty and doesn't have any information
		if set != "" {
			v.GTIDSet = set
			result = append(result, v)
		}
	}
	return result, nil
}

func (c *Collector) filterBinLogs(logs []pxc.Binlog, lastBinlogName string) ([]pxc.Binlog, error) {
	if lastBinlogName == "" {
		return c.removeEmptyBinlogs(logs)
	}

	logsLen := len(logs)

	startIndex := 0
	for logs[startIndex].Name != lastBinlogName && startIndex < logsLen {
		startIndex++
	}

	if startIndex == logsLen {
		return nil, nil
	}

	set, err := c.db.GetGTIDSet(logs[startIndex].Name)
	if err != nil {
		return nil, errors.Wrap(err, "get gtid set of last uploaded binlog")
	}
	// we don't need to reupload last file
	// if gtid set is not changed
	if set == c.lastSet {
		startIndex++
	}

	return c.removeEmptyBinlogs(logs[startIndex:])
}

func createGapFile(gtidSet string) error {
	p := "/tmp/gap-detected"
	f, err := os.Create(p)
	if err != nil {
		return errors.Wrapf(err, "create %s", p)
	}

	_, err = f.WriteString(gtidSet)
	if err != nil {
		return errors.Wrapf(err, "write GTID set to %s", p)
	}

	return nil
}

func (c *Collector) CollectBinLogs() error {
	list, err := c.db.GetBinLogList()
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}

	sourceID, err := c.CurrentSourceID(list)
	if err != nil {
		return errors.Wrap(err, "get current source id")
	}

	if sourceID == "" {
		log.Println("No binlogs to upload")
		return nil
	}

	c.lastSet, err = c.lastGTIDSet(sourceID)
	if err != nil {
		return errors.Wrap(err, "get last uploaded gtid set")
	}

	lastUploadedBinlogName := ""

	if c.lastSet != "" {
		// get last uploaded binlog file name
		lastUploadedBinlogName, err = c.db.GetBinLogName(c.lastSet)
		if err != nil {
			return errors.Wrap(err, "get last uploaded binlog name by gtid set")
		}

		if lastUploadedBinlogName == "" {
			log.Println("ERROR: Couldn't find the binlog that contains GTID set:", c.lastSet)
			log.Println("ERROR: Gap detected in the binary logs. Binary logs will be uploaded anyway, but full backup needed for consistent recovery.")
			if err := createGapFile(c.lastSet); err != nil {
				return errors.Wrap(err, "create gap file")
			}
		}
	}

	list, err = c.filterBinLogs(list, lastUploadedBinlogName)
	if err != nil {
		return errors.Wrap(err, "filter empty binlogs")
	}

	if len(list) == 0 {
		log.Println("No binlogs to upload")
		return nil
	}

	for _, binlog := range list {
		err = c.manageBinlog(binlog)
		if err != nil {
			return errors.Wrap(err, "manage binlog")
		}
	}
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

func (c *Collector) manageBinlog(binlog pxc.Binlog) (err error) {

	binlogTmstmp, err := c.db.GetBinLogFirstTimestamp(binlog.Name)
	if err != nil {
		return errors.Wrapf(err, "get first timestamp for %s", binlog.Name)
	}

	binlogName := fmt.Sprintf("binlog_%s_%x", binlogTmstmp, md5.Sum([]byte(binlog.GTIDSet)))

	var setBuffer bytes.Buffer
	// no error handling because WriteString() always return nil error
	// nolint:errcheck
	setBuffer.WriteString(binlog.GTIDSet)

	tmpDir := os.TempDir() + "/"

	err = os.Remove(tmpDir + binlog.Name)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove temp file")
	}

	err = syscall.Mkfifo(tmpDir+binlog.Name, 0666)
	if err != nil {
		return errors.Wrap(err, "make named pipe file error")
	}

	errBuf := &bytes.Buffer{}
	cmd := exec.Command("mysqlbinlog", "-R", "--raw", "-h"+c.db.GetHost(), "-u"+c.pxcUser, binlog.Name)
	cmd.Env = append(cmd.Env, "MYSQL_PWD="+c.pxcPass)
	cmd.Dir = os.TempDir()
	cmd.Stderr = errBuf

	err = cmd.Start()
	if err != nil {
		return errors.Wrap(err, "run mysqlbinlog command")
	}

	log.Println("Starting to process binlog with name", binlog.Name)

	file, err := os.OpenFile(tmpDir+binlog.Name, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		return errors.Wrap(err, "open named pipe file error")
	}

	defer func() {
		errC := file.Close()
		if errC != nil {
			err = mergeErrors(err, errors.Wrapf(errC, "close tmp file for %s", binlog.Name))
			return
		}
		errR := os.Remove(tmpDir + binlog.Name)
		if errR != nil {
			err = mergeErrors(err, errors.Wrapf(errR, "remove tmp file for %s", binlog.Name))
			return
		}
	}()

	// create a pipe to transfer data from the binlog pipe to s3
	pr, pw := io.Pipe()

	go readBinlog(file, pw, errBuf, binlog.Name)

	err = c.storage.PutObject(binlogName, pr, -1)
	if err != nil {
		return errors.Wrapf(err, "put %s object", binlog.Name)
	}

	log.Println("Successfully written binlog file", binlog.Name, "to s3 with name", binlogName)

	err = cmd.Wait()
	if err != nil {
		return errors.Wrap(err, "wait mysqlbinlog command error:"+errBuf.String())
	}

	err = c.storage.PutObject(binlogName+gtidPostfix, &setBuffer, int64(setBuffer.Len()))
	if err != nil {
		return errors.Wrap(err, "put gtid-set object")
	}
	// no error handling because WriteString() always return nil error
	// nolint:errcheck
	setBuffer.WriteString(binlog.GTIDSet)

	err = c.storage.PutObject(lastSetFilePrefix+strings.Split(binlog.GTIDSet, ":")[0], &setBuffer, int64(setBuffer.Len()))
	if err != nil {
		return errors.Wrap(err, "put last-set object")
	}
	c.lastSet = binlog.GTIDSet

	return nil
}

func readBinlog(file *os.File, pipe *io.PipeWriter, errBuf *bytes.Buffer, binlogName string) {
	b := make([]byte, 10485760) //alloc buffer for 10mb

	// in case of binlog is slow and hasn't written anything to the file yet
	// we have to skip this error and try to read again until some data appears
	isEmpty := true
	for {
		if errBuf.Len() != 0 {
			// stop reading since we receive error from binlog command in stderr
			// no error handling because CloseWithError() always return nil error
			// nolint:errcheck
			pipe.CloseWithError(errors.Errorf("Error: mysqlbinlog %s", errBuf.String()))
			return
		}
		n, err := file.Read(b)
		if err == io.EOF {
			// If we got EOF immediately after starting to read a file we should skip it since
			// data has not appeared yet. If we receive EOF error after already got some data - then exit.
			if isEmpty {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			break
		}
		if err != nil && !strings.Contains(err.Error(), "file already closed") {
			// no error handling because CloseWithError() always return nil error
			// nolint:errcheck
			pipe.CloseWithError(errors.Wrapf(err, "Error: reading named pipe for %s", binlogName))
			return
		}
		if n == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		_, err = pipe.Write(b[:n])
		if err != nil {
			// no error handling because CloseWithError() always return nil error
			// nolint:errcheck
			pipe.CloseWithError(errors.Wrapf(err, "Error: write to pipe for %s", binlogName))
			return
		}
		isEmpty = false
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
