package controller

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/pkg/errors"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/db"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/storage"
)

type Controller struct {
	dbm            *db.Manager
	sm             *storage.Manager
	lastSet        string
	pxcServiceName string
	pxcUser        string
	pxcPass        string
}

type Config struct {
	PXCServiceName string
	PXCUser        string
	PXCPass        string
	S3Endpoint     string
	S3accessKeyID  string
	S3accessKey    string
	S3bucketName   string
}

func New(c Config) (*Controller, error) {
	sm, err := storage.NewManager(c.S3Endpoint, c.S3accessKeyID, c.S3accessKey, c.S3bucketName, "last-set.txt", true)
	if err != nil {
		return nil, errors.Wrap(err, "new storage manager")
	}

	// get last binlog set stored on S3
	lastSet, err := sm.GetObjectContent(sm.LastSetObjectName)
	if err != nil && !strings.Contains(err.Error(), "does not exist") {
		return nil, errors.Wrap(err, "get lastset content")
	}

	return &Controller{
		sm:             sm,
		lastSet:        string(lastSet),
		pxcUser:        c.PXCUser,
		pxcPass:        c.PXCPass,
		pxcServiceName: c.PXCServiceName,
	}, nil
}

func (c *Controller) Run() error {
	err := c.newDB()
	if err != nil {
		return errors.Wrap(err, "new db connection")
	}
	defer c.closeDB()

	err = c.CollectBinLogFiles()
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
	m, err := db.NewManager(host, c.pxcUser, c.pxcPass)
	if err != nil {
		return errors.Wrapf(err, "new manager with host %s", host)

	}
	c.dbm = m

	return nil
}

func (c *Controller) closeDB() error {
	return c.dbm.Close()
}

func (c *Controller) getHost() (string, error) {
	cmd := exec.Command("peer-list", "-on-start=/usr/bin/get-pxc-state", "-service="+c.pxcServiceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, "get output")
	}
	nodes := strings.Split(string(out), "node:")
	for _, node := range nodes {
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			nodeArr := strings.Split(node, ".")
			return nodeArr[0] + "." + c.pxcServiceName, nil
		}
	}

	return "", nil
}

func (c *Controller) CollectBinLogFiles() error {
	// get last uploaded binlog file name
	binlogName, err := c.getLastBinlogName()
	if err != nil {
		return errors.Wrap(err, "get last binlog name")
	}
	list, err := c.dbm.GetBinLogFilesList()
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

func (c *Controller) getLastBinlogName() (string, error) {
	// get name of binlog file that contains given GTID set
	binlogName, err := c.dbm.GetBinLogNameByGTIDSet(c.lastSet)
	if err != nil {
		return "", errors.Wrap(err, "get binlog by set")
	}

	return binlogName, nil
}

func (c *Controller) manageBinlog(binlog string) error {
	cmd := exec.Command("mysqlbinlog", "-R", "-h"+c.dbm.Config.Addr, "-u"+c.dbm.Config.User, "-p"+c.dbm.Config.Passwd, binlog)
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

	var stdErr bytes.Buffer
	go func(stdErr *bytes.Buffer) {
		if errOut != nil {
			stdErr.ReadFrom(errOut)

		}
	}(&stdErr)

	err = c.sm.PutObject(binlog, out)
	if err != nil {
		return errors.Wrap(err, "put binlog object")
	}
	if stdErr.Bytes() != nil && stdErr.String() != db.UsingPassErrorMessage {
		return errors.New("mysqlbinlog: " + stdErr.String())
	}
	set, err := c.dbm.GetGTIDSetByBinLog(binlog)
	if err != nil {
		return errors.Wrap(err, "get GTID set")
	}
	var setBuffer bytes.Buffer
	setBuffer.WriteString(set)
	err = c.sm.PutObject(binlog+"-gtid-set", &setBuffer)
	if err != nil {
		return errors.Wrap(err, "put gtid-set object")
	}
	setBuffer.WriteString(set)
	err = c.sm.PutObject(c.sm.LastSetObjectName, &setBuffer)
	if err != nil {
		return errors.Wrap(err, "put last-set object")
	}
	c.lastSet = set

	return nil
}
