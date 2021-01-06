package pxc

import (
	"container/heap"
	"context"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/deployment"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileBackups(cr *api.PerconaXtraDBCluster) error {
	backups := make(map[string]api.PXCScheduledBackupSchedule)
	operatorPod, err := k8s.OperatorPod(r.client)
	if err != nil {
		return errors.Wrap(err, "get operator deployment")
	}

	if cr.Spec.Backup != nil {
		bcpObj := backup.New(cr)

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
			err = r.deletePITR(cr)
			if err != nil {
				return errors.Wrap(err, "delete pitr")
			}
		}

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
			err = r.client.Get(context.TODO(), types.NamespacedName{
				Name:      bcpjob.Name,
				Namespace: bcpjob.Namespace,
			}, currentBcpJob)
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Wrapf(err, "create scheduled backup %s", bcp.Name)
			}

			if k8serrors.IsNotFound(err) {
				err = r.client.Create(context.TODO(), bcpjob)
				if err != nil {
					return errors.Wrapf(err, "create scheduled backup %s", bcp.Name)
				}
			} else if !reflect.DeepEqual(currentBcpJob.Spec, bcpjob.Spec) {
				err = r.client.Update(context.TODO(), bcpjob)
				if err != nil {
					return errors.Wrapf(err, "update backup schedule %s", bcp.Name)
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
