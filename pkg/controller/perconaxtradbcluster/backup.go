package perconaxtradbcluster

import (
	"container/heap"
	"context"
	"fmt"

	batchv1beta1 "k8s.io/api/batch/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/backup"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileBackups(cr *api.PerconaXtraDBCluster) error {
	backups := make(map[string]api.PXCScheduledBackup)
	if cr.Spec.Backup != nil {
		for _, bcp := range *cr.Spec.Backup {
			backups[bcp.Name] = bcp
			bcpjob := backup.NewScheduled(cr, &bcp)
			err := setControllerReference(cr, bcpjob, r.scheme)
			if err != nil {
				return fmt.Errorf("set owner ref to backup %s: %v", bcp.Name, err)
			}

			// Check if this Job already exists
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: bcpjob.Name, Namespace: bcpjob.Namespace}, bcpjob)
			if err != nil && errors.IsNotFound(err) {
				// reqLogger.Info("Creating a new backup job", "Namespace", bcpjob.Namespace, "Name", bcpjob.Name)
				err = r.client.Create(context.TODO(), bcpjob)
				if err != nil {
					return fmt.Errorf("create scheduled backup '%s': %v", bcp.Name, err)
				}
			} else if err != nil {
				return fmt.Errorf("create scheduled backup '%s': %v", bcp.Name, err)
			}

			if bcp.Schedule != bcpjob.Spec.Schedule {
				bcpjob.Spec.Schedule = bcp.Schedule
				err = r.client.Update(context.TODO(), bcpjob)
				if err != nil {
					return fmt.Errorf("update backup schedule '%s': %v", bcp.Name, err)
				}
			}
		}
	}

	// Reconcile backups list
	bcpList := batchv1beta1.CronJobList{}
	err := r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace: cr.Namespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"cluster": cr.Name,
				"type":    "cron",
			}),
		},
		&bcpList,
	)
	if err != nil {
		return fmt.Errorf("get backups list: %v", err)
	}

	for _, item := range bcpList.Items {
		if spec, ok := backups[item.Name]; ok {
			if spec.Keep > 0 {
				oldjobs, err := r.oldScheduledJobs(cr, item.Name, spec.Keep)
				if err != nil {
					return fmt.Errorf("remove old backups: %v", err)
				}

				for _, todel := range oldjobs {
					r.client.Delete(context.TODO(), &todel)
				}
			}
		} else {
			r.client.Delete(context.TODO(), &item)
		}
	}

	return nil
}

// oldScheduledJobs returns list of the most old bakup jobs that execeed `keep` limit
func (r *ReconcilePerconaXtraDBCluster) oldScheduledJobs(cr *api.PerconaXtraDBCluster, ancestor string, keep int) ([]api.PerconaXtraDBBackup, error) {
	bcpList := api.PerconaXtraDBBackupList{}
	err := r.client.List(context.TODO(),
		&client.ListOptions{
			Namespace: cr.Namespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"cluster":  cr.Name,
				"ancestor": ancestor,
			}),
		},
		&bcpList,
	)
	if err != nil {
		return []api.PerconaXtraDBBackup{}, err
	}

	// fast path
	if len(bcpList.Items) <= keep {
		return []api.PerconaXtraDBBackup{}, nil
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
		return []api.PerconaXtraDBBackup{}, nil
	}

	ret := make([]api.PerconaXtraDBBackup, 0, h.Len()-keep)
	for i := h.Len() - keep; i > 0; i-- {
		o := heap.Pop(h).(api.PerconaXtraDBBackup)
		ret = append(ret, o)
	}

	return ret, nil
}

// A minHeap is a min-heap of backup jobs.
type minHeap []api.PerconaXtraDBBackup

func (h minHeap) Len() int { return len(h) }
func (h minHeap) Less(i, j int) bool {
	return h[i].CreationTimestamp.Before(&h[j].CreationTimestamp)
}
func (h minHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *minHeap) Push(x interface{}) {
	*h = append(*h, x.(api.PerconaXtraDBBackup))
}

func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
