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

	vstatus, err := pvc.Create(bcp.Spec.Volume)
	if err != nil {
		return fmt.Errorf("pvc create: %v", err)
	}

	var job backup.Jobster

	if bcp.Spec.Schedule == nil {
		job = backup.NewJob(bcp)
	} else {
		job = backup.NewJobScheduled(bcp)
	}

	switch vstatus {
	case backup.VolumeBound:
		err = job.Create(bcp.Spec)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("job create: %v", err)
		}
	default:
		return fmt.Errorf("volume not ready, status: %s", vstatus)
	}

	job.UpdateStatus(bcp)

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
