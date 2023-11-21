package pxc

import (
	"context"
	"strings"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *ReconcilePerconaXtraDBCluster) reconcilePersistentVolumes(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	pxcSet := statefulset.NewNode(cr)
	sts := pxcSet.StatefulSet()

	err := r.client.Get(ctx, client.ObjectKeyFromObject(sts), sts)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "get statefulset/%s", sts.Name)
	}

	if cr.Spec.PXC.VolumeSpec.PersistentVolumeClaim == nil {
		return nil
	}

	var volumeTemplate corev1.PersistentVolumeClaim
	for _, vct := range sts.Spec.VolumeClaimTemplates {
		if vct.Name == "datadir" {
			volumeTemplate = vct
		}
	}

	requested := cr.Spec.PXC.VolumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage]
	actual := volumeTemplate.Spec.Resources.Requests[corev1.ResourceStorage]

	if requested.Cmp(actual) < 0 {
		return errors.Wrap(err, "requested storage is less than actual")
	}

	if requested.Cmp(actual) == 0 {
		return nil
	}

	log.Info("Resizing PVCs", "requested", requested, "actual", actual)

	log.Info("Deleting statefulset", "name", sts.Name)

	err = r.client.Delete(ctx, sts, client.PropagationPolicy("Orphan"))
	if err != nil {
		return errors.Wrapf(err, "delete statefulset/%s", sts.Name)
	}

	pvcList := corev1.PersistentVolumeClaimList{}
	err = r.client.List(ctx, &pvcList, client.InNamespace(cr.Namespace))
	if err != nil {
		return errors.Wrap(err, "list persistentvolumeclaims")
	}

	for _, pvc := range pvcList.Items {
		if strings.HasPrefix(pvc.Name, "datadir-"+sts.Name) {
			log.Info("Resizing PVC", "name", pvc.Name)
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = requested
			err = r.client.Update(ctx, &pvc)
			if err != nil {
				return errors.Wrapf(err, "update persistentvolumeclaim/%s", pvc.Name)
			}
		}
	}

	return nil
}
