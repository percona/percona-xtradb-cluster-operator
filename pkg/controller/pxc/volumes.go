package pxc

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

func (r *ReconcilePerconaXtraDBCluster) reconcilePersistentVolumes(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	pxcSet := statefulset.NewNode(cr)
	sts := pxcSet.StatefulSet()

	labels := map[string]string{
		"app.kubernetes.io/component":  "pxc",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}

	pvcList := corev1.PersistentVolumeClaimList{}
	if err := r.client.List(ctx, &pvcList, client.InNamespace(cr.Namespace), client.MatchingLabels(labels)); err != nil {
		return errors.Wrap(err, "list persistentvolumeclaims")
	}

	if cr.PVCResizeInProgress() {
		resizeInProgress := false
		for _, pvc := range pvcList.Items {
			if !strings.HasPrefix(pvc.Name, "datadir-"+sts.Name) {
				continue
			}

			for _, condition := range pvc.Status.Conditions {
				if condition.Status != corev1.ConditionTrue {
					continue
				}

				switch condition.Type {
				case corev1.PersistentVolumeClaimResizing, corev1.PersistentVolumeClaimFileSystemResizePending:
					resizeInProgress = true
					log.Info(condition.Message, "reason", condition.Reason, "pvc", pvc.Name, "lastProbeTime", condition.LastProbeTime, "lastTransitionTime", condition.LastTransitionTime)
				}
			}
		}

		if !resizeInProgress {
			if err := k8s.DeannotateObject(ctx, r.client, cr, api.AnnotationPVCResizeInProgress); err != nil {
				return errors.Wrap(err, "deannotate pxc")
			}

			log.Info("PVC resize completed")
		}
	}

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

	err = k8s.AnnotateObject(ctx, r.client, cr, map[string]string{api.AnnotationPVCResizeInProgress: "true"})
	if err != nil {
		return errors.Wrap(err, "annotate pxc")
	}

	pvcNames := make([]string, 0, len(pvcList.Items))
	for _, pvc := range pvcList.Items {
		pvcNames = append(pvcNames, pvc.Name)
	}

	log.Info("Resizing PVCs", "requested", requested, "actual", actual, "pvcList", strings.Join(pvcNames, ","))

	log.Info("Deleting statefulset", "name", sts.Name)

	if err := r.client.Delete(ctx, sts, client.PropagationPolicy("Orphan")); err != nil {
		return errors.Wrapf(err, "delete statefulset/%s", sts.Name)
	}

	for _, pvc := range pvcList.Items {
		if !strings.HasPrefix(pvc.Name, "datadir-"+sts.Name) {
			continue
		}

		log.Info("Resizing PVC", "name", pvc.Name)
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = requested

		if err := r.client.Update(ctx, &pvc); err != nil {
			return errors.Wrapf(err, "update persistentvolumeclaim/%s", pvc.Name)
		}
	}

	return nil
}
