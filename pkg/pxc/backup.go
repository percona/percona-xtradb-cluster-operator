package pxc

import (
	"fmt"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"k8s.io/apimachinery/pkg/api/errors"
)

func (*PXC) backup(bcp *api.PerconaXtraDBBackup) error {
	pvc, err := backup.PVC(bcp)
	if err != nil {
		return fmt.Errorf("volume error: %v", err)
	}

	err = sdk.Create(&pvc)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("pvc create: %v", err)
	}

	job := backup.Job(bcp)
	sdk.Create(&job)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("job create: %v", err)
	}

	return nil
}
