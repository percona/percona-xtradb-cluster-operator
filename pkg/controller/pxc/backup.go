package pxc

import (
	"container/heap"
	"context"
	"fmt"

	"github.com/pkg/errors"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileBackups(cr *api.PerconaXtraDBCluster) error {
	backups := make(map[string]api.PXCScheduledBackupSchedule)
	operatorPod, err := r.operatorPod()
	if err != nil {
		return errors.Wrap(err, "get operator deployment")
	}

	if cr.Spec.Backup != nil {
		bcpObj := backup.New(cr)

		for _, bcp := range cr.Spec.Backup.Schedule {
			backups[bcp.Name] = bcp
			strg, ok := cr.Spec.Backup.Storages[bcp.StorageName]
			if !ok {
				return fmt.Errorf("storage %s doesn't exist", bcp.StorageName)
			}

			bcpjob, err := bcpObj.Scheduled(&bcp, strg, operatorPod)
			if err != nil {
				return fmt.Errorf("unable to schedule backup: %w", err)
			}
			err = setControllerReference(cr, bcpjob, r.scheme)
			if err != nil {
				return fmt.Errorf("set owner ref to backup %s: %v", bcp.Name, err)
			}

			// Check if this Job already exists
			currentBcpJob := new(batchv1beta1.CronJob)
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: bcpjob.Name, Namespace: bcpjob.Namespace}, currentBcpJob)
			if err != nil && k8serrors.IsNotFound(err) {
				// reqLogger.Info("Creating a new backup job", "Namespace", bcpjob.Namespace, "Name", bcpjob.Name)
				err = r.client.Create(context.TODO(), bcpjob)
				if err != nil {
					return fmt.Errorf("create scheduled backup '%s': %v", bcp.Name, err)
				}
			} else if err != nil {
				return fmt.Errorf("create scheduled backup '%s': %v", bcp.Name, err)
			} else {
				err = r.client.Update(context.TODO(), bcpjob)
				if err != nil {
					return fmt.Errorf("update backup schedule '%s': %v", bcp.Name, err)
				}
			}
		}
	}

	// Reconcile backups list
	bcpList := batchv1beta1.CronJobList{}
	err = r.client.List(context.TODO(),
		&bcpList,
		&client.ListOptions{
			Namespace: operatorPod.ObjectMeta.Namespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"cluster": cr.Name,
				"type":    "cron",
			}),
		},
	)
	if err != nil {
		return fmt.Errorf("get backups list: %v", err)
	}

	for _, item := range bcpList.Items {
		if spec, ok := backups[item.Name]; ok {
			if spec.Keep > 0 {
				oldjobs, err := r.oldScheduledBackups(cr, item.Name, spec.Keep)
				if err != nil {
					return fmt.Errorf("remove old backups: %v", err)
				}

				for _, todel := range oldjobs {
					_ = r.client.Delete(context.TODO(), &todel)
				}
			}
		} else {
			_ = r.client.Delete(context.TODO(), &item)
		}
	}

	return nil
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
