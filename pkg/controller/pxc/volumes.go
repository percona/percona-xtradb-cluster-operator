package pxc

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

func validatePVCName(pvc corev1.PersistentVolumeClaim, sts *appsv1.StatefulSet) bool {
	return strings.HasPrefix(pvc.Name, "datadir-"+sts.Name)
}

func (r *ReconcilePerconaXtraDBCluster) reconcilePersistentVolumes(ctx context.Context, cr *pxcv1.PerconaXtraDBCluster) error {
	pxcSet := statefulset.NewNode(cr)
	sts := pxcSet.StatefulSet()

	ls := map[string]string{
		"app.kubernetes.io/component":  "pxc",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}

	log := logf.FromContext(ctx).WithName("PVCResize").WithValues("sts", sts.Name)

	pvcList := &corev1.PersistentVolumeClaimList{}
	err := r.client.List(ctx, pvcList, &client.ListOptions{
		Namespace:     sts.Namespace,
		LabelSelector: labels.SelectorFromSet(ls),
	})
	if err != nil {
		return errors.Wrap(err, "list PVCs")
	}

	if len(pvcList.Items) == 0 {
		return nil
	}

	podList := corev1.PodList{}
	if err := r.client.List(ctx, &podList, client.InNamespace(cr.Namespace), client.MatchingLabels(ls)); err != nil {
		return errors.Wrap(err, "list pods")
	}

	podNames := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		podNames = append(podNames, pod.Name)
	}

	pvcsToUpdate := make([]string, 0, len(pvcList.Items))
	for _, pvc := range pvcList.Items {
		if !validatePVCName(pvc, sts) {
			continue
		}

		podName := strings.SplitN(pvc.Name, "-", 2)[1]
		if !slices.Contains(podNames, podName) {
			continue
		}

		pvcsToUpdate = append(pvcsToUpdate, pvc.Name)
	}

	if len(pvcsToUpdate) == 0 {
		return nil
	}

	var actual resource.Quantity
	for _, pvc := range pvcList.Items {
		if !validatePVCName(pvc, sts) {
			continue
		}

		if pvc.Status.Capacity == nil || pvc.Status.Capacity.Storage() == nil {
			continue
		}

		// we need to find the smallest size among all PVCs
		// since it indicates a failed resize operation
		if actual.IsZero() || pvc.Status.Capacity.Storage().Cmp(actual) < 0 {
			actual = *pvc.Status.Capacity.Storage()
		}
	}

	if actual.IsZero() {
		return nil
	}

	sts = sts.DeepCopy()
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "get statefulset %s", client.ObjectKeyFromObject(sts))
	}

	var volumeTemplate corev1.PersistentVolumeClaim
	for _, vct := range sts.Spec.VolumeClaimTemplates {
		if vct.Name == "datadir" {
			volumeTemplate = vct
		}
	}

	configured := volumeTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
	requested := cr.Spec.PXC.VolumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage]

	if requested.Format == resource.DecimalSI {
		requested, err = resource.ParseQuantity(requested.String() + "i")
		if err != nil {
			return errors.Wrap(err, "parse requested storage size")
		}
	}

	if cr.PVCResizeInProgress() {
		resizeStartedAt, err := time.Parse(time.RFC3339, cr.GetAnnotations()[pxcv1.AnnotationPVCResizeInProgress])
		if err != nil {
			return errors.Wrap(err, "parse annotation")
		}

		updatedPVCs := 0
		for _, pvc := range pvcList.Items {
			if !validatePVCName(pvc, sts) {
				continue
			}

			if pvc.Status.Capacity.Storage().Cmp(requested) == 0 {
				updatedPVCs++
				log.Info("PVC resize finished", "name", pvc.Name, "size", pvc.Status.Capacity.Storage())
				continue
			}

			for _, condition := range pvc.Status.Conditions {
				if condition.Status != corev1.ConditionTrue {
					continue
				}

				switch condition.Type {
				case corev1.PersistentVolumeClaimResizing, corev1.PersistentVolumeClaimFileSystemResizePending:
					log.V(1).Info(condition.Message, "pvc", pvc.Name, "type", condition.Type, "lastTransitionTime", condition.LastTransitionTime)
					log.Info("PVC resize in progress", "pvc", pvc.Name, "lastTransitionTime", condition.LastTransitionTime)
				}
			}

			events := &eventsv1.EventList{}
			if err := r.client.List(ctx, events, &client.ListOptions{
				Namespace:     sts.Namespace,
				FieldSelector: fields.SelectorFromSet(map[string]string{"regarding.name": pvc.Name}),
			}); err != nil {
				return errors.Wrapf(err, "list events for pvc/%s", pvc.Name)
			}

			for _, event := range events.Items {
				eventTime := event.EventTime.Time
				if event.EventTime.IsZero() {
					eventTime = event.DeprecatedFirstTimestamp.Time
				}

				if eventTime.Before(resizeStartedAt) {
					continue
				}

				switch event.Reason {
				case "Resizing", "ExternalExpanding", "FileSystemResizeRequired":
					log.Info("PVC resize in progress", "pvc", pvc.Name, "reason", event.Reason, "message", event.Note)
				case "FileSystemResizeSuccessful":
					log.Info("PVC resize completed", "pvc", pvc.Name, "reason", event.Reason, "message", event.Note)
				case "VolumeResizeFailed":
					log.Error(nil, "PVC resize failed", "pvc", pvc.Name, "reason", event.Reason, "message", event.Note)

					if err := r.handlePVCResizeFailure(ctx, cr, configured); err != nil {
						return err
					}

					return errors.Errorf("volume resize failed: %s", event.Note)
				}
			}
		}

		resizeSucceeded := updatedPVCs == len(pvcsToUpdate)
		if resizeSucceeded {
			log.Info("Deleting statefulset")

			if err := r.client.Delete(ctx, sts, client.PropagationPolicy("Orphan")); err != nil {
				if k8serrors.IsNotFound(err) {
					return nil
				}
				return errors.Wrapf(err, "delete statefulset/%s", sts.Name)
			}

			if err := k8s.DeannotateObject(ctx, r.client, cr, pxcv1.AnnotationPVCResizeInProgress); err != nil {
				return errors.Wrap(err, "deannotate pxc")
			}

			log.Info("PVC resize completed")

			return nil
		}
	}

	if requested.Cmp(actual) < 0 {
		return errors.Errorf("requested storage (%s) is less than actual storage (%s)", requested.String(), actual.String())
	}

	if requested.Cmp(configured) == 0 || requested.Cmp(actual) == 0 {
		return nil
	}

	err = k8s.AnnotateObject(ctx, r.client, cr, map[string]string{pxcv1.AnnotationPVCResizeInProgress: metav1.Now().Format(time.RFC3339)})
	if err != nil {
		return errors.Wrap(err, "annotate pxc")
	}

	log.Info("Resizing PVCs", "requested", requested, "actual", actual, "pvcList", strings.Join(pvcsToUpdate, ","))

	for _, pvc := range pvcList.Items {
		if !slices.Contains(pvcsToUpdate, pvc.Name) {
			continue
		}

		if pvc.Status.Capacity.Storage().Cmp(requested) == 0 {
			log.Info("PVC already resized", "name", pvc.Name, "actual", pvc.Status.Capacity.Storage(), "requested", requested)
			continue
		}

		log.Info("Resizing PVC", "name", pvc.Name, "actual", pvc.Status.Capacity.Storage(), "requested", requested)
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = requested

		if err := r.client.Update(ctx, &pvc); err != nil {
			switch {
			case strings.Contains(err.Error(), "exceeded quota"):
				log.Error(err, "PVC resize failed", "reason", "ExceededQuota", "message", err.Error())

				if err := r.handlePVCResizeFailure(ctx, cr, configured); err != nil {
					return err
				}

				return errors.Wrapf(err, "update persistentvolumeclaim/%s", pvc.Name)
			case strings.Contains(err.Error(), "the storageclass that provisions the pvc must support resize"):
				log.Error(err, "PVC resize failed", "reason", "StorageClassNotSupportResize", "message", err.Error())

				if err := r.handlePVCResizeFailure(ctx, cr, configured); err != nil {
					return err
				}

				return errors.Wrapf(err, "update persistentvolumeclaim/%s", pvc.Name)
			default:
				return errors.Wrapf(err, "update persistentvolumeclaim/%s", pvc.Name)
			}
		}

		log.Info("PVC resize started", "pvc", pvc.Name, "requested", requested)
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) handlePVCResizeFailure(ctx context.Context, cr *pxcv1.PerconaXtraDBCluster, originalSize resource.Quantity) error {
	if err := r.revertVolumeTemplate(ctx, cr, originalSize); err != nil {
		return errors.Wrapf(err, "revert volume template in pxc/%s", cr.Name)
	}

	if err := k8s.DeannotateObject(ctx, r.client, cr, pxcv1.AnnotationPVCResizeInProgress); err != nil {
		return errors.Wrapf(err, "deannotate pxc/%s", cr.Name)
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) revertVolumeTemplate(ctx context.Context, cr *pxcv1.PerconaXtraDBCluster, originalSize resource.Quantity) error {
	log := logf.FromContext(ctx)

	orig := cr.DeepCopy()

	log.Info("Reverting volume template for PXC", "originalSize", originalSize)
	cr.Spec.PXC.VolumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage] = originalSize

	if err := r.client.Patch(ctx, cr.DeepCopy(), client.MergeFrom(orig)); err != nil {
		return errors.Wrapf(err, "patch pxc/%s", cr.Name)
	}

	return nil
}
