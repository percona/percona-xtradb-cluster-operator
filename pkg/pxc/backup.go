package pxc

import (
	"fmt"
	"reflect"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"k8s.io/apimachinery/pkg/api/errors"
)

func (h *PXC) backup(bcp *api.PerconaXtraDBBackup) error {
	pvc := backup.NewPVC(bcp)
	job := backup.Job(bcp)
	addOwnerRefToObject(job, bcp.OwnerRef())

	vstatus, err := pvc.Create(bcp.Spec.Volume)
	if err != nil {
		return fmt.Errorf("pvc create: %v", err)
	}

	switch vstatus {
	case backup.VolumeBound:
		err = sdk.Create(job)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("job create: %v", err)
		}
	default:
		return fmt.Errorf("volume not ready, status: %s", vstatus)
	}

	sdk.Get(job)
	status := &api.PXCBackupStatus{
		State: api.BackupStarting,
	}

	switch {
	case job.Status.Active == 1:
		status.State = api.BackupRunning
	case job.Status.Succeeded == 1:
		status.State = api.BackupSucceeded
		status.CompletedAt = job.Status.CompletionTime
	case job.Status.Failed == 1:
		status.State = api.BackupFailed
	}
	// jjj, _ := json.Marshal(job.Status)
	// fmt.Printf("\n\n%s\n\n", jjj)
	updateBackupStatus(bcp, status)

	return nil
}

func updateBackupStatus(bcp *api.PerconaXtraDBBackup, status *api.PXCBackupStatus) error {
	// don't update the status if there aren't any changes.
	if reflect.DeepEqual(bcp.Status, *status) {
		return nil
	}
	bcp.Status = *status
	return sdk.Update(bcp)
}
