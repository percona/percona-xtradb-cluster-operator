package pxc

import (
	"context"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/binlogcollector"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileBinlogCollector(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	initImage, err := k8s.GetInitImage(ctx, cr, r.client)
	if err != nil {
		return errors.Wrap(err, "failed to get init image")
	}

	if err := r.createOrUpdateService(ctx, cr, binlogcollector.GetService(cr), false); err != nil {
		return errors.Wrap(err, "create or update binlog collector")
	}

	existingDepl := &appsv1.Deployment{}
	binlogCollectorName := naming.BinlogCollectorDeploymentName(cr)
	err = r.client.Get(ctx, types.NamespacedName{Name: binlogCollectorName, Namespace: cr.Namespace}, existingDepl)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "get existing deployment")
		}
	}

	collector, err := binlogcollector.GetDeployment(cr, initImage, existingDepl.Spec.Selector.MatchLabels)
	if err != nil {
		return errors.Wrapf(err, "get binlog collector deployment for cluster '%s'", cr.Name)
	}

	err = k8s.SetControllerReference(cr, &collector, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "set controller reference for binlog collector deployment '%s'", collector.Name)
	}

	if err := r.createOrUpdate(ctx, cr, &collector); err != nil {
		return errors.Wrap(err, "create or update binlog collector")
	}

	return nil
}
