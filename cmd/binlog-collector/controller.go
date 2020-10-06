package main

import (
	"log"
	"strings"

	"github.com/pkg/errors"

	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/db"
	"github.com/percona/percona-xtradb-cluster-operator/cmd/binlog-collector/storage"
)

func manageBinlogs(c config) error {
	var dbm *db.Manager
	for _, host := range c.pxcHosts {
		m, err := db.NewManager(host, c.pxcUser, c.pxcPass)
		if err != nil {
			log.Println(errors.Wrapf(err, "new manager with host %s", host))
			continue
		}
		dbm = m
		break
	}
	if dbm == nil {
		return errors.New("can't connect to any host")
	}
	defer dbm.Close()

	sm, err := storage.NewManager(c.s3Endpoint, c.s3accessKeyID, c.s3accessKey, c.s3bucketName, "last-set.txt", true)
	if err != nil {
		return errors.Wrap(err, "new storage manager")
	}

	err = collectBinLogFiles(dbm, &sm)
	if err != nil {
		return errors.Wrap(err, "collect binlog files")
	}

	return nil
}

func collectBinLogFiles(dbm *db.Manager, sm *storage.Manager) error {
	// get last uploaded binlog file name
	binlogName, err := getLastBinlogName(dbm, sm)
	if err != nil {
		return errors.Wrap(err, "get last binlog name")
	}
	list, err := dbm.GetBinLogFilesList()
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
			binlogFileContent, err := dbm.GetBinLogFileContent(binlog)
			if err != nil {
				return errors.Wrap(err, "get binlog content")
			}
			err = sm.PutObject(binlog, string(binlogFileContent))
			if err != nil {
				return errors.Wrap(err, "put binlog object")
			}

			set, err := dbm.GetGTIDSetByBinLog(binlog)
			if err != nil {
				return errors.Wrap(err, "get GTID set")
			}
			err = sm.PutObject(sm.LastSetObjectName, set)
			if err != nil {
				return errors.Wrap(err, "put last-set object")
			}
		}
	}

	return nil
}

func getLastBinlogName(dbm *db.Manager, sm *storage.Manager) (string, error) {
	// get last binlog set stored on S3
	lastSet, err := sm.GetObjectContent(sm.LastSetObjectName)
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		return "", nil
	} else if err != nil {
		return "", errors.Wrap(err, "get object content")
	}

	// get name of binlog file that contains given GTID set
	binlogName, err := dbm.GetBinLogNameByGTIDSet(string(lastSet))
	if err != nil {
		return "", errors.Wrap(err, "get binlog by set")
	}

	return binlogName, nil
}
