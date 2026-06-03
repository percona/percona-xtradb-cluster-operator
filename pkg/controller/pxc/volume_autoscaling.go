package pxc

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/metrics"
)

// reconcileStorageAutoscaling checks PVC disk usage and triggers resize if needed
func (r *ReconcilePerconaXtraDBCluster) reconcileStorageAutoscaling(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
) error {
	log := logf.FromContext(ctx).WithName("StorageAutoscaling")
	ctx = logf.IntoContext(ctx, log)

	autoscalingSpec := cr.Spec.StorageAutoscaling()
	if autoscalingSpec == nil || !autoscalingSpec.Enabled {
		return nil
	}

	if cr.Spec.StorageScaling != nil && cr.Spec.StorageScaling.VolumeExternalAutoscaling {
		log.V(1).Info("skipping storage autoscaling: external autoscaling is enabled")
		return nil
	}

	if !cr.Spec.IsVolumeExpansionEnabled() {
		log.V(1).Info("skipping storage autoscaling: volume expansion is disabled")
		return nil
	}

	volumeSpec := cr.Spec.PXC.VolumeSpec

	if volumeSpec == nil || volumeSpec.PersistentVolumeClaim == nil {
		log.V(1).Info("skipping storage autoscaling: not using PVC")
		return nil
	}

	sts := statefulset.NewNode(cr).StatefulSet()
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		if k8serrors.IsNotFound(err) {
			log.V(1).Info("skipping storage autoscaling: pxc statefulset not found yet")
			return nil
		}
		return errors.Wrap(err, "failed to get pxc sts")
	}

	if cr.PVCResizeInProgress() {
		log.V(1).Info("PVC resize already in progress")
		return nil
	}

	ls := naming.LabelsPXC(cr)
	pvcList := &corev1.PersistentVolumeClaimList{}
	err := r.client.List(ctx, pvcList, &client.ListOptions{
		Namespace:     cr.Namespace,
		LabelSelector: labels.SelectorFromSet(ls),
	})
	if err != nil {
		return errors.Wrap(err, "list PVCs for autoscaling")
	}

	podList := &corev1.PodList{}
	err = r.client.List(ctx, podList, &client.ListOptions{
		Namespace:     cr.Namespace,
		LabelSelector: labels.SelectorFromSet(ls),
	})
	if err != nil {
		return errors.Wrap(err, "list pods for autoscaling")
	}

	for _, pvc := range pvcList.Items {
		if !validatePVCName(pvc, sts) {
			continue
		}

		podName := extractPodNameFromPVC(pvc.Name, sts.Name)
		pod := findPodByName(podList, podName)
		if pod == nil {
			log.V(1).Info("pod not found for PVC", "pvc", pvc.Name, "pod", podName)
			continue
		}

		if err := r.checkAndResizePVC(ctx, cr, &pvc, pod, volumeSpec); err != nil {
			log.Error(err, "failed to check/resize PVC", "pvc", pvc.Name)
			r.updateAutoscalingStatus(ctx, cr, pvc.Name, nil, err)
		}
	}

	return nil
}

// checkAndResizePVC checks a single PVC and triggers resize if needed
func (r *ReconcilePerconaXtraDBCluster) checkAndResizePVC(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
	pvc *corev1.PersistentVolumeClaim,
	pod *corev1.Pod,
	volumeSpec *api.VolumeSpec,
) error {
	log := logf.FromContext(ctx).WithValues("pvc", pvc.Name)
	ctx = logf.IntoContext(ctx, log)

	podRunning := false
	if pod.Status.Phase == corev1.PodRunning {
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == naming.ContainerNamePXC && container.State.Running != nil {
				podRunning = true
			}
		}
	}
	if !podRunning {
		log.V(1).Info("skipping PVC metrics check: container and pod not running", "phase", pod.Status.Phase)
		return nil
	}

	usage, err := metrics.GetPVCUsage(ctx, r.clientcmd, pod, pvc.Name)
	if err != nil {
		return errors.Wrap(err, "get PVC usage from metrics")
	}

	r.updateAutoscalingStatus(ctx, cr, pvc.Name, usage, nil)

	if !r.shouldTriggerResize(ctx, cr, pvc, usage) {
		return nil
	}

	newSize := r.calculateNewSize(cr, pvc)

	log.Info("triggering storage autoscaling",
		"currentSize", pvc.Status.Capacity.Storage().String(),
		"newSize", newSize.String(),
		"usagePercent", usage.UsagePercent,
		"threshold", cr.Spec.StorageAutoscaling().TriggerThresholdPercent)

	return r.triggerResize(ctx, cr, pvc, newSize, volumeSpec)
}

// shouldTriggerResize determines if a PVC should be resized
func (r *ReconcilePerconaXtraDBCluster) shouldTriggerResize(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
	pvc *corev1.PersistentVolumeClaim,
	usage *metrics.PVCUsage,
) bool {
	log := logf.FromContext(ctx)
	config := cr.Spec.StorageAutoscaling()

	if usage.UsagePercent < config.TriggerThresholdPercent {
		return false
	}

	if !config.MaxSize.IsZero() {
		currentSize := pvc.Status.Capacity.Storage()
		if currentSize.Cmp(config.MaxSize) >= 0 {
			log.Info("PVC already at maxSize",
				"currentSize", currentSize.String(),
				"maxSize", config.MaxSize.String())
			return false
		}
	}

	for _, cond := range pvc.Status.Conditions {
		if (cond.Type == corev1.PersistentVolumeClaimResizing ||
			cond.Type == corev1.PersistentVolumeClaimFileSystemResizePending) &&
			cond.Status == corev1.ConditionTrue {
			log.V(1).Info("resize already in progress", "condition", cond.Type)
			return false
		}
	}

	return true
}

// calculateNewSize calculates the new PVC size based on current size and growth step
func (r *ReconcilePerconaXtraDBCluster) calculateNewSize(
	cr *api.PerconaXtraDBCluster,
	pvc *corev1.PersistentVolumeClaim,
) resource.Quantity {
	config := cr.Spec.StorageAutoscaling()
	currentSize := pvc.Status.Capacity.Storage()

	newSizeBytes := currentSize.Value() + config.GrowthStep.Value()
	newSize := *resource.NewQuantity(newSizeBytes, resource.BinarySI)

	if !config.MaxSize.IsZero() && newSize.Cmp(config.MaxSize) > 0 {
		newSize = config.MaxSize
	}

	return newSize
}

// triggerResize updates the CR volumeSpec to trigger a resize operation
func (r *ReconcilePerconaXtraDBCluster) triggerResize(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
	pvc *corev1.PersistentVolumeClaim,
	newSize resource.Quantity,
	volumeSpec *api.VolumeSpec,
) error {
	log := logf.FromContext(ctx)

	orig := cr.DeepCopy()

	volumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage] = newSize

	if err := r.client.Patch(ctx, cr.DeepCopy(), client.MergeFrom(orig)); err != nil {
		return errors.Wrap(err, "patch CR with new storage size")
	}

	log.Info("storage autoscaling initiated",
		"oldSize", pvc.Status.Capacity.Storage().String(),
		"newSize", newSize.String())

	return nil
}

// updateAutoscalingStatus updates the status for a specific PVC
func (r *ReconcilePerconaXtraDBCluster) updateAutoscalingStatus(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
	pvcName string,
	usage *metrics.PVCUsage,
	err error,
) {
	log := logf.FromContext(ctx)

	if pvcName == "" {
		log.V(1).Info("no pvc name specified")
		return
	}

	if cr.Status.StorageAutoscaling == nil {
		cr.Status.StorageAutoscaling = make(map[string]api.StorageAutoscalingStatus)
	}

	status := cr.Status.StorageAutoscaling[pvcName]

	if usage != nil {
		newSize := resource.NewQuantity(usage.TotalBytes, resource.BinarySI)
		if status.CurrentSize != "" {
			oldSize, parseErr := resource.ParseQuantity(status.CurrentSize)
			if parseErr == nil && newSize.Cmp(oldSize) > 0 {
				status.LastResizeTime = metav1.Time{Time: time.Now()}
				status.ResizeCount++
			}
		}
		status.CurrentSize = newSize.String()
		status.LastError = ""
	}

	if err != nil {
		status.LastError = err.Error()
	}

	cr.Status.StorageAutoscaling[pvcName] = status
}

// extractPodNameFromPVC extracts the pod name from a datadir PVC name.
// PVC format: "datadir-<statefulset-name>-<index>"
// Pod format: "<statefulset-name>-<index>"
func extractPodNameFromPVC(pvcName string, stsName string) string {
	prefix := naming.DataVolumeName + "-" + stsName + "-"
	if strings.HasPrefix(pvcName, prefix) {
		return stsName + "-" + strings.TrimPrefix(pvcName, prefix)
	}
	return ""
}

// findPodByName finds a pod in a list by name
func findPodByName(podList *corev1.PodList, podName string) *corev1.Pod {
	for i := range podList.Items {
		if podList.Items[i].Name == podName {
			return &podList.Items[i]
		}
	}
	return nil
}
