package pxc

import (
	"container/heap"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/deployment"
)

type BackupScheduleJob struct {
	api.PXCScheduledBackupSchedule
	JobID cron.EntryID
}

func (r *ReconcilePerconaXtraDBCluster) reconcileBackups(cr *api.PerconaXtraDBCluster) error {
	logger := r.logger("backup", cr.Namespace)
	backups := make(map[string]api.PXCScheduledBackupSchedule)
	backupNamePrefix := backupJobClusterPrefix(cr.Name)

	if cr.Spec.Backup != nil {

		if cr.Status.Status == api.AppStateReady && cr.Spec.Backup.PITR.Enabled && !cr.Spec.Pause {
			binlogCollector, err := deployment.GetBinlogCollectorDeployment(cr)
			if err != nil {
				return errors.Errorf("get binlog collector deployment for cluster '%s': %v", cr.Name, err)
			}
			binlogCollectorName := deployment.GetBinlogCollectorDeploymentName(cr)
			currentCollector := appsv1.Deployment{}
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: binlogCollectorName, Namespace: cr.Namespace}, &currentCollector)
			if err != nil && k8serrors.IsNotFound(err) {
				err = r.client.Create(context.TODO(), &binlogCollector)
				if err != nil && !k8serrors.IsAlreadyExists(err) {
					return fmt.Errorf("create binlog collector deployment for cluster '%s': %v", cr.Name, err)
				}
			} else if err != nil {
				return fmt.Errorf("get binlogCollector '%s': %v", binlogCollectorName, err)
			} else {
				currentCollector.Spec = binlogCollector.Spec
				err = r.client.Update(context.TODO(), &currentCollector)
				if err != nil {
					return fmt.Errorf("update binlogCollector '%s': %v", binlogCollectorName, err)
				}
			}
		}
		if !cr.Spec.Backup.PITR.Enabled || cr.Spec.Pause {
			err := r.deletePITR(cr)
			if err != nil {
				return errors.Wrap(err, "delete pitr")
			}
		}

		for i, bcp := range cr.Spec.Backup.Schedule {
			bcp.Name = backupNamePrefix + "-" + bcp.Name
			backups[bcp.Name] = bcp
			strg, ok := cr.Spec.Backup.Storages[bcp.StorageName]
			if !ok {
				logger.Info("invalid storage name for backup", "backup name", cr.Spec.Backup.Schedule[i].Name, "storage name", bcp.StorageName)
				continue
			}

			sch := BackupScheduleJob{}
			schRaw, ok := r.crons.backupJobs.Load(bcp.Name)
			if ok {
				sch = schRaw.(BackupScheduleJob)
			}

			if !ok || sch.PXCScheduledBackupSchedule.Schedule != bcp.Schedule ||
				sch.PXCScheduledBackupSchedule.StorageName != bcp.StorageName {
				r.log.Info("Creating or updating backup job", "name", bcp.Name, "schedule", bcp.Schedule)
				r.deleteBackupJob(bcp.Name)
				jobID, err := r.crons.AddFuncWithSeconds(bcp.Schedule, r.createBackupJob(cr, bcp, strg.Type))
				if err != nil {
					logger.Error(err, "can't parse cronjob schedule", "backup name", cr.Spec.Backup.Schedule[i].Name, "schedule", bcp.Schedule)
					continue
				}

				r.crons.backupJobs.Store(bcp.Name, BackupScheduleJob{
					PXCScheduledBackupSchedule: bcp,
					JobID:                      jobID,
				})
			}
		}
	}

	r.crons.backupJobs.Range(func(k, v interface{}) bool {
		item := v.(BackupScheduleJob)
		if !strings.HasPrefix(item.Name, backupNamePrefix) {
			return true
		}
		if spec, ok := backups[item.Name]; ok {
			if spec.Keep > 0 {
				oldjobs, err := r.oldScheduledBackups(cr, item.Name, spec.Keep)
				if err != nil {
					logger.Error(err, "failed to list old backups", "job name", item.Name)
					return true
				}

				for _, todel := range oldjobs {
					err = r.client.Delete(context.TODO(), &todel)
					if err != nil {
						logger.Error(err, "failed to delete old backup", "backup name", todel.Name)
					}
				}

			}
		} else {
			r.log.Info("deleting outdated backup job", "name", item.Name)
			r.deleteBackupJob(item.Name)
		}

		return true
	})

	return nil
}

func backupJobClusterPrefix(clusterName string) string {
	h := sha1.New()
	h.Write([]byte(clusterName))
	return hex.EncodeToString(h.Sum(nil))[:5]
}

// oldScheduledBackups returns list of the most old pxc-bakups that execeed `keep` limit
func (r *ReconcilePerconaXtraDBCluster) oldScheduledBackups(cr *api.PerconaXtraDBCluster, ancestor string, keep int) ([]api.PerconaXtraDBClusterBackup, error) {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	err := r.client.List(context.TODO(),
		&bcpList,
		&client.ListOptions{
			Namespace: cr.Namespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"cluster":  cr.Name,
				"ancestor": ancestor,
			}),
		},
	)
	if err != nil {
		return []api.PerconaXtraDBClusterBackup{}, err
	}

	// fast path
	if len(bcpList.Items) <= keep {
		return []api.PerconaXtraDBClusterBackup{}, nil
	}

	// just build an ordered by creationTimestamp min-heap from items and return top "len(items) - keep" items
	h := &minHeap{}
	heap.Init(h)
	for _, bcp := range bcpList.Items {
		if bcp.Status.State == api.BackupSucceeded {
			heap.Push(h, bcp)
		}
	}

	if h.Len() <= keep {
		return []api.PerconaXtraDBClusterBackup{}, nil
	}

	ret := make([]api.PerconaXtraDBClusterBackup, 0, h.Len()-keep)
	for i := h.Len() - keep; i > 0; i-- {
		o := heap.Pop(h).(api.PerconaXtraDBClusterBackup)
		ret = append(ret, o)
	}

	return ret, nil
}

func (r *ReconcilePerconaXtraDBCluster) createBackupJob(cr *api.PerconaXtraDBCluster, backupJob api.PXCScheduledBackupSchedule, storageType api.BackupStorageType) func() {
	fins := []string{}
	if storageType == api.BackupStorageS3 {
		fins = append(fins, api.FinalizerDeleteS3Backup)
	}

	return func() {
		localCr := &api.PerconaXtraDBCluster{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCr)
		if k8serrors.IsNotFound(err) {
			r.log.Info("cluster is not found, deleting the job",
				"job name", jobName, "cluster", cr.Name, "namespace", cr.Namespace)
			r.deleteBackupJob(backupJob.Name)
			return
		}

		bcp := &api.PerconaXtraDBClusterBackup{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: fins,
				Namespace:  cr.Namespace,
				Name:       generateBackupName(cr, backupJob.StorageName) + "-" + strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(backupJob.Schedule))), 32)[:5],
				Labels: map[string]string{
					"ancestor": backupJob.Name,
					"cluster":  cr.Name,
					"type":     "cron",
				},
			},
			Spec: api.PXCBackupSpec{
				PXCCluster:  cr.Name,
				StorageName: backupJob.StorageName,
			},
		}
		err = r.client.Create(context.TODO(), bcp)
		if err != nil {
			r.log.Error(err, "failed to create backup")
		}
	}
}

func (r *ReconcilePerconaXtraDBCluster) deleteBackupJob(name string) {
	job, ok := r.crons.backupJobs.LoadAndDelete(name)
	if !ok {
		return
	}
	r.crons.crons.Remove(job.(BackupScheduleJob).JobID)
}

func generateBackupName(cr *api.PerconaXtraDBCluster, storageName string) string {
	result := "cron-"
	if len(cr.Name) > 16 {
		result += cr.Name[:16]
	} else {
		result += cr.Name
	}
	result += "-" + trimNameRight(storageName, 16) + "-"
	tnow := time.Now()
	result += fmt.Sprintf("%d%d%d%d%d%d", tnow.Year(), tnow.Month(), tnow.Day(), tnow.Hour(), tnow.Minute(), tnow.Second())
	return result
}

func trimNameRight(name string, ln int) string {
	if len(name) <= ln {
		ln = len(name)
	}

	for ; ln > 0; ln-- {
		if name[ln-1] >= 'a' && name[ln-1] <= 'z' ||
			name[ln-1] >= '0' && name[ln-1] <= '9' {
			break
		}
	}

	return name[:ln]
}

// A minHeap is a min-heap of backup jobs.
type minHeap []api.PerconaXtraDBClusterBackup

func (h minHeap) Len() int { return len(h) }
func (h minHeap) Less(i, j int) bool {
	return h[i].CreationTimestamp.Before(&h[j].CreationTimestamp)
}
func (h minHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *minHeap) Push(x interface{}) {
	*h = append(*h, x.(api.PerconaXtraDBClusterBackup))
}

func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (r *ReconcilePerconaXtraDBCluster) deletePITR(cr *api.PerconaXtraDBCluster) error {
	collectorDeployment := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployment.GetBinlogCollectorDeploymentName(cr),
			Namespace: cr.Namespace,
		},
	}
	err := r.client.Delete(context.TODO(), &collectorDeployment)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "delete pitr deployment")
	}

	return nil
}
