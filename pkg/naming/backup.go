package naming

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func BackupLeaseName(clusterName string) string {
	return "pxc-" + clusterName + "-backup-lock"
}

func BackupHolderId(cr *pxcv1.PerconaXtraDBClusterBackup) string {
	return fmt.Sprintf("%s-%s", cr.Name, cr.UID)
}

// BackupJobName generates legit name for backup resources.
// k8s sets the `job-name` label for the created by job pod.
// So we have to be sure that job name won't be longer than 63 symbols.
// Yet the job name has to have some meaningful name which won't be conflicting with other jobs' names.
func BackupJobName(crName string) string {
	return trimJobName("xb-" + crName)
}

// trimJobName trims the provided string to ensure it stays within the 63-character limit.
// The job name will be included in the "batch.kubernetes.io/job-name" label in the ".spec.template" section of the job.
// Labels have a maximum length of 63 characters, so this function ensures the job name fits within that limit.
func trimJobName(name string) string {
	trimLeft := func(name string) string {
		for i := 0; i < len(name); i++ {
			if (name[i] < 'a' || name[i] > 'z') && (name[i] < '0' || name[i] > '9') {
				continue
			}
			return name[i:]
		}
		return ""
	}

	trimRight := func(name string) string {
		for i := len(name) - 1; i >= 0; i-- {
			if (name[i] < 'a' || name[i] > 'z') && (name[i] < '0' || name[i] > '9') {
				continue
			}
			return name[:i+1]
		}
		return ""
	}

	name = trimLeft(name)
	name = trimRight(name)
	if len(name) > validation.DNS1035LabelMaxLength {
		name = name[:validation.DNS1035LabelMaxLength]
		name = trimRight(name)
	}

	return name
}

func ScheduledBackupName(crName, storageName, schedule string) string {
	result := "cron"

	if len(crName) > 16 {
		result += "-" + crName[:16]
	} else {
		result += "-" + crName
	}

	if len(storageName) > 16 {
		result += "-" + storageName[:16]
	} else {
		result += "-" + storageName
	}

	tnow := time.Now()
	result += "-" + fmt.Sprintf("%d%d%d%d%d%d", tnow.Year(), tnow.Month(), tnow.Day(), tnow.Hour(), tnow.Minute(), tnow.Second())
	result += "-" + strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(schedule))), 32)[:5]
	return result
}
