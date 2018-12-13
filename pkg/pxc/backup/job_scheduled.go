package backup

// import (
// 	"github.com/operator-framework/operator-sdk/pkg/sdk"
// 	batchv1 "k8s.io/api/batch/v1beta1"
// 	"k8s.io/apimachinery/pkg/api/errors"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// 	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
// )

// type JobScheduled struct {
// 	name string
// 	obj  *batchv1.CronJob
// }

// func NewJobScheduled(cr *api.PerconaXtraDBBackup) *JobScheduled {
// 	jb := &batchv1.CronJob{
// 		TypeMeta: metav1.TypeMeta{
// 			APIVersion: "batch/v1beta1",
// 			Kind:       "CronJob",
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      cr.Spec.PXCCluster + "-xtrabackup." + cr.Name,
// 			Namespace: cr.Namespace,
// 		},
// 	}

// 	jb.SetOwnerReferences(append(jb.GetOwnerReferences(), cr.OwnerRef()))

// 	return &JobScheduled{
// 		name: cr.Name,
// 		obj:  jb,
// 	}
// }

// // Create creates the backup job
// func (j *JobScheduled) Create(spec api.PXCBackupSpec) error {
// 	j.obj.Spec.Schedule = *spec.Schedule
// 	j.obj.Spec.JobTemplate.Spec = jobSpec(spec, j.name)

// 	err := sdk.Create(j.obj)
// 	if err != nil && !errors.IsAlreadyExists(err) {
// 		return err
// 	}

// 	return nil
// }

// // UpdateStatus updates `Status` of the given CR with current job the state
// func (j *JobScheduled) UpdateStatus(cr *api.PerconaXtraDBBackup) {
// 	sdk.Get(j.obj)
// 	status := &api.PXCBackupStatus{
// 		State: api.BackupStarting,
// 	}

// 	switch {
// 	case len(j.obj.Status.Active) > 0:
// 		status.State = api.BackupRunning
// 		status.LastScheduled = j.obj.Status.LastScheduleTime

// 		// case j.obj.Status.Succeeded == 1:
// 		// 	status.State = api.BackupSucceeded
// 		// 	status.CompletedAt = j.obj.Status.CompletionTime
// 		// case j.obj.Status.Failed == 1:
// 		// 	status.State = api.BackupFailed
// 	}

// 	updateStatus(cr, status)
// }
