package backup

// import (
// 	"github.com/operator-framework/operator-sdk/pkg/sdk"
// 	batchv1 "k8s.io/api/batch/v1"
// 	"k8s.io/apimachinery/pkg/api/errors"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// 	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
// )

// type Job struct {
// 	name string
// 	obj  *batchv1.Job
// }

// func NewJob(cr *api.PerconaXtraDBBackup) *Job {
// 	jb := &batchv1.Job{
// 		TypeMeta: metav1.TypeMeta{
// 			APIVersion: "batch/v1",
// 			Kind:       "Job",
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      cr.Spec.PXCCluster + "-xtrabackup." + cr.Name,
// 			Namespace: cr.Namespace,
// 		},
// 	}

// 	jb.SetOwnerReferences(append(jb.GetOwnerReferences(), cr.OwnerRef()))

// 	return &Job{
// 		name: cr.Name,
// 		obj:  jb,
// 	}
// }

// // Create creates the backup job
// func (j *Job) Create(spec api.PXCBackupSpec) error {
// 	j.obj.Spec = jobSpec(spec, j.name)
// 	bflim := int32(4)
// 	j.obj.Spec.BackoffLimit = &bflim

// 	err := sdk.Create(j.obj)
// 	if err != nil && !errors.IsAlreadyExists(err) {
// 		return err
// 	}

// 	return nil
// }

// // UpdateStatus updates `Status` of the given CR with current job the state
// func (j *Job) UpdateStatus(cr *api.PerconaXtraDBBackup) {
// 	sdk.Get(j.obj)
// 	status := &api.PXCBackupStatus{
// 		State: api.BackupStarting,
// 	}

// 	switch {
// 	case j.obj.Status.Active == 1:
// 		status.State = api.BackupRunning
// 	case j.obj.Status.Succeeded == 1:
// 		status.State = api.BackupSucceeded
// 		status.CompletedAt = j.obj.Status.CompletionTime
// 	case j.obj.Status.Failed == 1:
// 		status.State = api.BackupFailed
// 	}

// 	updateStatus(cr, status)
// }
