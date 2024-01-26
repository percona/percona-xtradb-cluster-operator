package collector

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/pitr/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

type Collector struct {
	db              *pxc.PXC
	storage         storage.Storage
	lastUploadedSet pxc.GTIDSet // last uploaded binary logs set
	pxcServiceName  string      // k8s service name for PXC, its for get correct host for connection
	pxcUser         string      // user for connection to PXC
	pxcPass         string      // password for connection to PXC
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
}

const (
	lastSetFilePrefix string = "last-binlog-set-"   // filename prefix for object where the last binlog set will stored
	gtidPostfix       string = "-gtid-set"          // filename postfix for files with GTID set
	timelinePath      string = "/tmp/pitr-timeline" // path to file with timeline
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
		s, err = storage.NewS3(ctx, c.BackupStorageS3.Endpoint, c.BackupStorageS3.AccessKeyID, c.BackupStorageS3.AccessKey, bucketArr[0], prefix, c.BackupStorageS3.Region, c.VerifyTLS)
		if err != nil {
			return nil, errors.Wrap(err, "new storage manager")
		}
	case "azure":
		container, prefix, _ := strings.Cut(c.BackupStorageAzure.ContainerPath, "/")
		if prefix != "" {
			prefix += "/"
		}
		s, err = storage.NewAzure(c.BackupStorageAzure.AccountName, c.BackupStorageAzure.AccountKey, c.BackupStorageAzure.Endpoint, container, prefix)
		if err != nil {
			return nil, errors.Wrap(err, "new azure storage")
		}
	default:
		return nil, errors.New("unknown STORAGE_TYPE")
	}

	return &Collector{
		storage:        s,
		pxcUser:        c.PXCUser,
		pxcServiceName: c.PXCServiceName,
	}, nil
}

func (c *Collector) Run(ctx context.Context) error {
	err := c.newDB(ctx)
	if err != nil {
		return errors.Wrap(err, "new db connection")
	}
	defer c.close()

	// remove last set because we always
	// read it from aws file
	c.lastUploadedSet = pxc.NewGTIDSet("")

	err = c.CollectBinLogs(ctx)
	if err != nil {
		return errors.Wrap(err, "collect binlog files")
	}

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
	file, err := os.Open("/etc/mysql/mysql-users-secret/xtrabackup")
	if err != nil {
		return errors.Wrap(err, "open file")
	}
	pxcPass, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrap(err, "read password")
	}
	c.pxcPass = string(pxcPass)

	host, err := pxc.GetPXCOldestBinlogHost(ctx, c.pxcServiceName, c.pxcUser, c.pxcPass)
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
	p := "/tmp/gap-detected"
	f, err := os.Create(p)
	if err != nil {
		return errors.Wrapf(err, "create %s", p)
	}

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
	f, err := os.Create(timelinePath)
	if err != nil {
		return errors.Wrapf(err, "create %s", timelinePath)
	}

	_, err = f.WriteString(firstTs)
	if err != nil {
		return errors.Wrap(err, "write first timestamp to timeline file")
	}

	return nil
}

func updateTimelineFile(lastTs string) error {
	f, err := os.OpenFile(timelinePath, os.O_RDWR, 0o644)
	if err != nil {
		return errors.Wrapf(err, "open %s", timelinePath)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "scan %s", timelinePath)
	}

	if len(lines) > 1 {
		lines[len(lines)-1] = lastTs
	} else {
		lines = append(lines, lastTs)
	}

	if _, err := f.Seek(0, 0); err != nil {
		return errors.Wrapf(err, "seek %s", timelinePath)
	}

	if err := f.Truncate(0); err != nil {
		return errors.Wrapf(err, "truncate %s", timelinePath)
	}

	_, err = f.WriteString(strings.Join(lines, "\n"))
	if err != nil {
		return errors.Wrap(err, "write last timestamp to timeline file")
	}

	return nil
}

func (c *Collector) addGTIDSets(ctx context.Context, logs []pxc.Binlog) error {
	for i, v := range logs {
		set, err := c.db.GetGTIDSet(ctx, v.Name)
		if err != nil {
			if errors.Is(err, &mysql.MySQLError{Number: 3200}) {
				log.Printf("ERROR: Binlog file %s is invalid on host %s: %s\n", v.Name, c.db.GetHost(), err.Error())
				continue
			}
			return errors.Wrap(err, "get GTID set")
		}
		logs[i].GTIDSet = pxc.NewGTIDSet(set)
	}
	return nil
}

func (c *Collector) CollectBinLogs(ctx context.Context) error {
	list, err := c.db.GetBinLogList(ctx)
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}
	err = c.addGTIDSets(ctx, list)
	if err != nil {
		return errors.Wrap(err, "get GTID sets")
	}
	var lastGTIDSetList []string
	for i := len(list) - 1; i >= 0 && len(lastGTIDSetList) == 0; i-- {
		gtidSetList := list[i].GTIDSet.List()
		if gtidSetList == nil {
			continue
		}
		lastGTIDSetList = gtidSetList
	}

	if len(lastGTIDSetList) == 0 {
		log.Println("No binlogs to upload")
		return nil
	}

	for _, gtidSet := range lastGTIDSetList {
		sourceID := strings.Split(gtidSet, ":")[0]
		c.lastUploadedSet, err = c.lastGTIDSet(ctx, sourceID)
		if err != nil {
			return errors.Wrap(err, "get last uploaded gtid set")
		}
		if !c.lastUploadedSet.IsEmpty() {
			break
		}
	}

	lastUploadedBinlogName := ""

	if !c.lastUploadedSet.IsEmpty() {
		for i := len(list) - 1; i >= 0 && lastUploadedBinlogName == ""; i-- {
			for _, gtidSet := range list[i].GTIDSet.List() {
				if lastUploadedBinlogName != "" {
					break
				}
				for _, lastUploaded := range c.lastUploadedSet.List() {
					isSubset, err := c.db.GTIDSubset(ctx, lastUploaded, gtidSet)
					if err != nil {
						return errors.Wrap(err, "check if gtid set is subset")
					}
					if isSubset {
						lastUploadedBinlogName = list[i].Name
						break
					}
					isSubset, err = c.db.GTIDSubset(ctx, gtidSet, lastUploaded)
					if err != nil {
						return errors.Wrap(err, "check if gtid set is subset")
					}
					if isSubset {
						lastUploadedBinlogName = list[i].Name
						break
					}
				}
			}
		}

		if lastUploadedBinlogName == "" {
			log.Println("ERROR: Couldn't find the binlog that contains GTID set:", c.lastUploadedSet.Raw())
			log.Println("ERROR: Gap detected in the binary logs. Binary logs will be uploaded anyway, but full backup needed for consistent recovery.")
			if err := createGapFile(c.lastUploadedSet); err != nil {
				return errors.Wrap(err, "create gap file")
			}
		}
	}

	list, err = c.filterBinLogs(ctx, list, lastUploadedBinlogName)
	if err != nil {
		return errors.Wrap(err, "filter empty binlogs")
	}

	if len(list) == 0 {
		log.Println("No binlogs to upload")
		return nil
	}

	if exists, err := fileExists(timelinePath); !exists && err == nil {
		firstTs, err := c.db.GetBinLogFirstTimestamp(ctx, list[0].Name)
		if err != nil {
			return errors.Wrap(err, "get first timestamp")
		}

		if err := createTimelineFile(firstTs); err != nil {
			return errors.Wrap(err, "create timeline file")
		}
	}

	for _, binlog := range list {
		err = c.manageBinlog(ctx, binlog)
		if err != nil {
			return errors.Wrap(err, "manage binlog")
		}

		lastTs, err := c.db.GetBinLogLastTimestamp(ctx, binlog.Name)
		if err != nil {
			return errors.Wrap(err, "get last timestamp")
		}

		if err := updateTimelineFile(lastTs); err != nil {
			return errors.Wrap(err, "update timeline file")
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

func (c *Collector) manageBinlog(ctx context.Context, binlog pxc.Binlog) (err error) {
	binlogTmstmp, err := c.db.GetBinLogFirstTimestamp(ctx, binlog.Name)
	if err != nil {
		return errors.Wrapf(err, "get first timestamp for %s", binlog.Name)
	}

	binlogName := fmt.Sprintf("binlog_%s_%x", binlogTmstmp, md5.Sum([]byte(binlog.GTIDSet.Raw())))

	var setBuffer bytes.Buffer
	// no error handling because WriteString() always return nil error
	// nolint:errcheck
	setBuffer.WriteString(binlog.GTIDSet.Raw())

	tmpDir := os.TempDir() + "/"

	err = os.Remove(tmpDir + binlog.Name)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove temp file")
	}

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

	err = c.storage.PutObject(ctx, binlogName, pr, -1)
	if err != nil {
		return errors.Wrapf(err, "put %s object", binlog.Name)
	}

	log.Println("Successfully written binlog file", binlog.Name, "to s3 with name", binlogName)

	err = cmd.Wait()
	if err != nil {
		return errors.Wrap(err, "wait mysqlbinlog command error:"+errBuf.String())
	}

	err = c.storage.PutObject(ctx, binlogName+gtidPostfix, &setBuffer, int64(setBuffer.Len()))
	if err != nil {
		return errors.Wrap(err, "put gtid-set object")
	}
	for _, gtidSet := range binlog.GTIDSet.List() {
		// no error handling because WriteString() always return nil error
		// nolint:errcheck
		setBuffer.WriteString(binlog.GTIDSet.Raw())

		err = c.storage.PutObject(ctx, lastSetFilePrefix+strings.Split(gtidSet, ":")[0], &setBuffer, int64(setBuffer.Len()))
		if err != nil {
			return errors.Wrap(err, "put last-set object")
		}
	}
	c.lastUploadedSet = binlog.GTIDSet

	return nil
}

func readBinlog(file *os.File, pipe *io.PipeWriter, errBuf *bytes.Buffer, binlogName string) {
	b := make([]byte, 10485760) // alloc buffer for 10mb

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
