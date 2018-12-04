package pxc

import (
	"fmt"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
)

func (*PXC) backup(bcp *api.PXCBackup) error {
	pvc, err := backup.PVC(&bcp.Spec)
	if err != nil {
		return fmt.Errorf("volume error: %v", err)
	}

	sdk.Create(&pvc)

	sdk.Create(&backup.Job(&bcp.Spec))

	return nil
}
