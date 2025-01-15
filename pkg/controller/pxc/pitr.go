package pxc

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/deployment"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileBinlogCollector(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	initImage, err := k8s.GetInitImage(ctx, cr, r.client)
	if err != nil {
		return errors.Wrap(err, "failed to get init image")
	}

	binlogCollector, err := deployment.GetBinlogCollectorDeployment(cr, initImage)
	if err != nil {
		return errors.Wrapf(err, "get binlog collector deployment for cluster '%s'", cr.Name)
	}

	err = setControllerReference(cr, &binlogCollector, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "set controller reference for binlog collector deployment '%s'", binlogCollector.Name)
	}

	res, err := controllerutil.CreateOrUpdate(ctx, r.client, &binlogCollector, func() error { return nil })
	if err != nil {
		return errors.Wrap(err, "create or update binlog collector")
	}

	switch res {
	case controllerutil.OperationResultCreated:
		log.Info("Created binlog collector", "name", binlogCollector.Name)
	case controllerutil.OperationResultUpdated:
		log.Info("Updated binlog collector", "name", binlogCollector.Name)
	}

	return nil
}
