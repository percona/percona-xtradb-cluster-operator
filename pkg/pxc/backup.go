package pxc

import (
	"fmt"

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

	switch vstatus {
	case backup.VolumeBound:
		job := backup.Job(bcp)
		addOwnerRefToObject(job, bcp.OwnerRef())

		err = sdk.Create(job)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("job create: %v", err)
		}
	default:
		return fmt.Errorf("volume not ready, status: %s", vstatus)
	}
	return nil
}
