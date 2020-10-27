package collector

import (
	"bytes"
	"log"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/db"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/storage"
)

type Controller struct {
	db              *db.PXC
	storage         storage.Client
	lastSet         string // last uploaded binary logs set
	pxcServiceName  string // k8s service name for PXC, its for get correct host for connection
	pxcUser         string // user for connection to PXC
	pxcPass         string // password for connection to PXC
	lastSetFileName string // name for object where the last binlog set will stored
}

type Config struct {
	PXCServiceName string
	PXCUser        string
	PXCPass        string
	S3Endpoint     string
	S3AccessKeyID  string
	S3AccessKey    string
	S3BucketName   string
}

func New(c Config) (*Controller, error) {
	s3, err := storage.NewS3(c.S3Endpoint, c.S3AccessKeyID, c.S3AccessKey, c.S3BucketName, true)
	if err != nil {
		return nil, errors.Wrap(err, "new storage manager")
	}
	lastSetFileName := "last-set"
	// get last binlog set stored on S3
	lastSet, err := s3.GetObject(lastSetFileName)
	if err != nil {
		return nil, errors.Wrap(err, "get lastset content")
	}

	return &Controller{
		storage:         s3,
		lastSet:         string(lastSet),
		pxcUser:         c.PXCUser,
		pxcPass:         c.PXCPass,
		pxcServiceName:  c.PXCServiceName,
		lastSetFileName: lastSetFileName,
	}, nil
}

func (c *Controller) Run() error {
	err := c.newDB()
	if err != nil {
		return errors.Wrap(err, "new db connection")
	}
	defer c.closeDB()

	err = c.CollectBinLogs()
	if err != nil {
		return errors.Wrap(err, "collect binlog files:")
	}

	return nil
}

func (c *Controller) newDB() error {
	host, err := c.getHost()
	if err != nil {
		return errors.Wrap(err, "get host")
	}
	pxc, err := db.NewPXC(host, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)

	}
	c.db = pxc

	return nil
}

func (c *Controller) closeDB() error {
	return c.db.Close()
}

func (c *Controller) getHost() (string, error) {
	cmd := exec.Command("peer-list", "-on-start=/usr/bin/get-pxc-state", "-service="+c.pxcServiceName)
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

	return lastHost + "." + c.pxcServiceName, nil
}

func (c *Controller) CollectBinLogs() error {
	// get last uploaded binlog file name
	binlogName, err := c.db.GetBinLogName(c.lastSet)
	if err != nil {
		return errors.Wrap(err, "get binlog name by set")
	}
	list, err := c.db.GetBinLogList()
	if err != nil {
		return errors.Wrap(err, "get binlog list")
	}

	upload := false
	// if there are no uploaded files we going to upload every binlog file
	if len(binlogName) == 0 {
		upload = true
	}

	for _, binlog := range list {
		if binlog == binlogName { // this check is for uploading starting from needed file
			upload = true
		}
		if upload {
			err = c.manageBinlog(binlog)
			if err != nil {
				return errors.Wrap(err, "manage binlog")
			}
		}
	}

	return nil
}

func (c *Controller) manageBinlog(binlog string) error {
	cmd := exec.Command("mysqlbinlog", "-R", "-h"+c.db.GetHost(), "-u"+c.pxcUser, "-p"+c.pxcPass, binlog)

	out, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "get stdout pipe")
	}

	errOut, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrap(err, "get stderr pipe")
	}
	err = cmd.Start()
	if err != nil {
		return errors.Wrap(err, "run mysqlbinlog command")
	}
	defer cmd.Wait()
	var outBuf bytes.Buffer
	wg := sync.WaitGroup{}
	go func() {
		wg.Add(1)
		_, err = outBuf.ReadFrom(out)
		if err != nil {
			log.Println("ERROR: read to buffer:", err)
		}
		wg.Done()
	}()

	err = c.storage.PutObject(binlog, &outBuf)
	if err != nil {
		return errors.Wrap(err, "put binlog object")
	}
	wg.Wait()

	var stdErr bytes.Buffer
	stdErr.ReadFrom(errOut)
	if stdErr.Bytes() != nil && stdErr.String() != db.UsingPassErrorMessage {
		return errors.Errorf("mysqlbinlog: %s", stdErr.String())
	}

	set, err := c.db.GetGTIDSet(binlog)
	if err != nil {
		return errors.Wrap(err, "get GTID set")
	}
	var setBuffer bytes.Buffer
	setBuffer.WriteString(set)
	err = c.storage.PutObject(binlog+"-gtid-set", &setBuffer)
	if err != nil {
		return errors.Wrap(err, "put gtid-set object")
	}
	setBuffer.WriteString(set)
	err = c.storage.PutObject(c.lastSetFileName, &setBuffer)
	if err != nil {
		return errors.Wrap(err, "put last-set object")
	}
	c.lastSet = set

	return nil
}
