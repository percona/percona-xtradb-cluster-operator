package pxc

import (
	"container/heap"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/binlogcollector"
)

type BackupScheduleJob struct {
	api.PXCScheduledBackupSchedule
	JobID cron.EntryID
}

func (r *ReconcilePerconaXtraDBCluster) reconcileBackups(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	backups := make(map[string]api.PXCScheduledBackupSchedule)
	backupNamePrefix := backupJobClusterPrefix(cr.Namespace + "-" + cr.Name)

	if cr.Spec.Backup != nil {
		restoreRunning, err := r.isRestoreRunning(cr.Name, cr.Namespace)
		if err != nil {
			return errors.Wrap(err, "failed to check if restore is running")
		}

		if cr.Status.Status == api.AppStateReady && cr.Spec.Backup.PITR.Enabled && !cr.Spec.Pause && !restoreRunning {
			if err := r.reconcileBinlogCollector(ctx, cr); err != nil {
				return errors.Wrap(err, "reconcile binlog collector")
			}
		}

		if !cr.Spec.Backup.PITR.Enabled || cr.Spec.Pause || restoreRunning {
			err := r.deletePITR(ctx, cr)
			if err != nil {
				return errors.Wrap(err, "delete pitr")
			}
		}

		for i, bcp := range cr.Spec.Backup.Schedule {
			bcp.Name = backupNamePrefix + "-" + bcp.Name
			backups[bcp.Name] = bcp
			strg, ok := cr.Spec.Backup.Storages[bcp.StorageName]
			if !ok {
				log.Info("invalid storage name for backup", "backup name", cr.Spec.Backup.Schedule[i].Name, "storage name", bcp.StorageName)
				continue
			}

			sch := BackupScheduleJob{}
			schRaw, ok := r.crons.backupJobs.Load(bcp.Name)
			if ok {
				sch = schRaw.(BackupScheduleJob)
			}

			if !ok || shouldRecreateBackupJob(bcp, sch) {
				log.Info("Creating or updating backup job", "name", bcp.Name, "schedule", bcp.Schedule)
				r.deleteBackupJob(bcp.Name)
				jobID, err := r.crons.AddFuncWithSeconds(bcp.Schedule, r.createBackupJob(ctx, cr, bcp, strg.Type))
				if err != nil {
					log.Error(err, "can't parse cronjob schedule", "backup name", cr.Spec.Backup.Schedule[i].Name, "schedule", bcp.Schedule)
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
			if spec.GetRetention().IsValidCountRetention() {
				oldjobs, err := r.oldScheduledBackups(ctx, cr, item.Name, spec.GetRetention().Count)
				if err != nil {
					log.Error(err, "failed to list old backups", "name", item.Name)
					return true
				}

				for _, todel := range oldjobs {
					log.Info("deleting outdated backup", "backup", todel.Name)
					err = r.client.Delete(ctx, &todel)
					if err != nil {
						log.Error(err, "failed to delete old backup", "name", todel.Name)
					}
				}

			}
		} else {
			log.Info("deleting outdated backup job", "name", item.Name)
			r.deleteBackupJob(item.Name)
		}

		return true
	})

	return nil
}

// shouldRecreateBackupJob determines whether the existing backup job needs to be recreated.
func shouldRecreateBackupJob(expected api.PXCScheduledBackupSchedule, existing BackupScheduleJob) bool {
	recreate := existing.PXCScheduledBackupSchedule.Schedule != expected.Schedule ||
		existing.PXCScheduledBackupSchedule.StorageName != expected.StorageName

	if recreate {
		return true
	}

	if existing.PXCScheduledBackupSchedule.Retention != nil && expected.Retention != nil {
		if existing.PXCScheduledBackupSchedule.Retention.DeleteFromStorage !=
			expected.Retention.DeleteFromStorage {
			return true
		}
	}

	return false
}

func backupJobClusterPrefix(clusterName string) string {
	h := sha1.New()
	h.Write([]byte(clusterName))
	return hex.EncodeToString(h.Sum(nil))[:5]
}

// oldScheduledBackups returns list of the most old pxc-bakups that execeed `keep` limit
func (r *ReconcilePerconaXtraDBCluster) oldScheduledBackups(ctx context.Context, cr *api.PerconaXtraDBCluster, ancestor string, keep int) ([]api.PerconaXtraDBClusterBackup, error) {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	err := r.client.List(ctx,
		&bcpList,
		&client.ListOptions{
			Namespace: cr.Namespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				naming.LabelPerconaClusterName:        cr.Name,
				naming.LabelPerconaBackupAncestorName: ancestor,
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

func (r *ReconcilePerconaXtraDBCluster) createBackupJob(ctx context.Context, cr *api.PerconaXtraDBCluster, backupJob api.PXCScheduledBackupSchedule, storageType api.BackupStorageType) func() {
	log := logf.FromContext(ctx)

	finalizers := backupFinalizers(cr, backupJob, storageType)

	return func() {
		localCr := &api.PerconaXtraDBCluster{}
		err := r.client.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCr)
		if k8serrors.IsNotFound(err) {
			log.Info("cluster is not found, deleting the job",
				"name", backupJob.Name, "cluster", cr.Name, "namespace", cr.Namespace)
			r.deleteBackupJob(backupJob.Name)
			return
		}

		if err := localCr.CanBackup(); err != nil {
			log.Info("Cluster is not ready for backup. Scheduled backup is not created", "error", err.Error(), "name", backupJob.Name, "cluster", cr.Name, "namespace", cr.Namespace)
			return
		}

		bcp := &api.PerconaXtraDBClusterBackup{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: finalizers,
				Namespace:  cr.Namespace,
				Name:       naming.ScheduledBackupName(cr.Name, backupJob.StorageName, backupJob.Schedule),
				Labels:     naming.LabelsScheduledBackup(cr, backupJob.Name),
			},
			Spec: api.PXCBackupSpec{
				PXCCluster:              cr.Name,
				StorageName:             backupJob.StorageName,
				StartingDeadlineSeconds: cr.Spec.Backup.StartingDeadlineSeconds,
			},
		}
		err = r.client.Create(ctx, bcp)
		if err != nil {
			log.Error(err, "failed to create backup")
		}
	}
}

func backupFinalizers(cr *api.PerconaXtraDBCluster, backupJob api.PXCScheduledBackupSchedule, storageType api.BackupStorageType) []string {
	switch storageType {
	case api.BackupStorageS3, api.BackupStorageAzure, api.BackupStorageFilesystem:
		if cr.CompareVersionWith("1.18.0") >= 0 && !backupJob.GetRetention().DeleteFromStorage {
			return []string{}
		}
		return []string{naming.FinalizerDeleteBackup}
	default:
		return []string{}
	}
}

func (r *ReconcilePerconaXtraDBCluster) deleteBackupJob(name string) {
	job, ok := r.crons.backupJobs.LoadAndDelete(name)
	if !ok {
		return
	}
	r.crons.crons.Remove(job.(BackupScheduleJob).JobID)
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

func (r *ReconcilePerconaXtraDBCluster) deletePITR(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	collectorDeployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.BinlogCollectorDeploymentName(cr),
			Namespace: cr.Namespace,
		},
	}

	if err := r.client.Delete(ctx, &collectorDeployment); err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "delete collector deployment")
	}

	if !cr.Spec.Backup.PITR.Enabled {
		if err := r.client.Delete(ctx, binlogcollector.GetService(cr)); err != nil && !k8serrors.IsNotFound(err) {
			return errors.Wrap(err, "delete collector service")
		}
	}

	return nil
}
