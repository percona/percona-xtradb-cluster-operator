package k8s

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

func PauseCluster(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (bool, error) {
	if !cr.Spec.Pause {
		cr := cr.DeepCopy() // calling patch will overwrite cr, removing values set by CheckNsetDefaults. We need to copy it into a new variable

		patch := client.MergeFrom(cr.DeepCopy())
		cr.Spec.Pause = true
		err := cl.Patch(ctx, cr, patch)
		if err != nil {
			return false, errors.Wrap(err, "shutdown pods")
		}
		time.Sleep(time.Second)
	}
	cr.Spec.Pause = true

	pods := corev1.PodList{}

	ls := statefulset.NewNode(cr).Labels()
	if err := cl.List(
		ctx,
		&pods,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(ls),
		},
	); err != nil {
		return false, errors.Wrap(err, "get pods list")
	}

	if len(pods.Items) != 0 {
		return false, nil
	}

	return true, nil
}

func UnpauseCluster(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (bool, error) {
	if cr.Spec.Pause {
		cr := cr.DeepCopy() // calling patch will overwrite cr, removing values set by CheckNsetDefaults. We need to copy it into a new variable

		patch := client.MergeFrom(cr.DeepCopy())
		cr.Spec.Pause = false
		err := cl.Patch(ctx, cr, patch)
		if err != nil {
			return false, errors.Wrap(err, "patch")
		}
	}
	cr.Spec.Pause = false

	ls := statefulset.NewNode(cr).Labels()
	pods := new(corev1.PodList)
	if err := cl.List(
		ctx,
		pods,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(ls),
		},
	); err != nil {
		return false, errors.Wrap(err, "get pods list")
	}

	if len(pods.Items) != int(cr.Spec.PXC.Size) {
		return false, nil
	}

	return true, nil
}

// Deprecated: PauseClusterWithWait is a function which blocks reconcile process. Use PauseCluster instead
func PauseClusterWithWait(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster, deletePVC bool) error {
	cr = cr.DeepCopy()
	var gracePeriodSec int64

	if cr.Spec.PXC != nil && cr.Spec.PXC.TerminationGracePeriodSeconds != nil {
		gracePeriodSec = int64(cr.Spec.PXC.Size) * *cr.Spec.PXC.TerminationGracePeriodSeconds
	}

	patch := client.MergeFrom(cr.DeepCopy())
	cr.Spec.Pause = true
	err := cl.Patch(ctx, cr, patch)
	if err != nil {
		return errors.Wrap(err, "shutdown pods")
	}

	ls := statefulset.NewNode(cr).Labels()
	err = waitForPodsShutdown(ctx, cl, ls, cr.Namespace, gracePeriodSec)
	if err != nil {
		return errors.Wrap(err, "shutdown pods")
	}
	if !deletePVC {
		return nil
	}

	pvcs := corev1.PersistentVolumeClaimList{}
	err = cl.List(
		ctx,
		&pvcs,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(ls),
		},
	)
	if err != nil {
		return errors.Wrap(err, "get pvc list")
	}
	pxcNode := statefulset.NewNode(cr)
	pvcNameTemplate := app.DataVolumeName + "-" + pxcNode.StatefulSet().Name
	for _, pvc := range pvcs.Items {
		// check prefix just in case, to be sure we're not going to delete a wrong pvc
		if pvc.Name == pvcNameTemplate+"-0" || !strings.HasPrefix(pvc.Name, pvcNameTemplate) {
			continue
		}

		err = cl.Delete(ctx, &pvc)
		if err != nil {
			return errors.Wrap(err, "delete pvc")
		}
	}

	err = waitForPVCShutdown(ctx, cl, ls, cr.Namespace, pvcNameTemplate)
	if err != nil {
		return errors.Wrap(err, "shutdown pvc")
	}

	return nil
}

func waitForPodsShutdown(ctx context.Context, cl client.Client, ls map[string]string, namespace string, gracePeriodSec int64) error {
	for i := int64(0); i < waitLimitSec+gracePeriodSec; i++ {
		pods := corev1.PodList{}

		err := cl.List(
			ctx,
			&pods,
			&client.ListOptions{
				Namespace:     namespace,
				LabelSelector: labels.SelectorFromSet(ls),
			},
		)
		if err != nil {
			return errors.Wrap(err, "get pods list")
		}

		if len(pods.Items) == 0 {
			return nil
		}

		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}

const waitLimitSec int64 = 300

func waitForPVCShutdown(ctx context.Context, cl client.Client, ls map[string]string, namespace, pvcNameTemplate string) error {
	for i := int64(0); i < waitLimitSec; i++ {
		pvcs := corev1.PersistentVolumeClaimList{}

		err := cl.List(
			ctx,
			&pvcs,
			&client.ListOptions{
				Namespace:     namespace,
				LabelSelector: labels.SelectorFromSet(ls),
			},
		)
		if err != nil {
			return errors.Wrap(err, "get pvc list")
		}

		filtered := []corev1.PersistentVolumeClaim{}
		for _, pvc := range pvcs.Items {
			// check prefix to filter out extraPVCs
			if strings.HasPrefix(pvc.Name, pvcNameTemplate) {
				filtered = append(filtered, pvc)
			}
		}

		if len(filtered) == 1 {
			return nil
		}

		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}

// Deprecated: UnpauseClusterWithWait is a function which blocks reconcile process. Use UnpauseCluster instead
func UnpauseClusterWithWait(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (err error) {
	// tryin several times just to avoid possible conflicts with the main controller
	err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		// need to get the object with latest version of meta-data for update
		current := new(api.PerconaXtraDBCluster)
		rerr := cl.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, current)
		if rerr != nil {
			return errors.Wrap(err, "get cluster")
		}
		current.Spec = cr.Spec
		return cl.Update(ctx, current)
	})
	if err != nil {
		return errors.Wrap(err, "update cluster")
	}

	// give time for process new state
	time.Sleep(10 * time.Second)

	var waitLimit int32 = 2 * 60 * 60 // 2 hours
	if cr.Spec.PXC.LivenessInitialDelaySeconds != nil {
		waitLimit = *cr.Spec.PXC.LivenessInitialDelaySeconds * cr.Spec.PXC.Size
	}

	for i := int32(0); i < waitLimit; i++ {
		current := new(api.PerconaXtraDBCluster)
		err = cl.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, current)
		if err != nil {
			return errors.Wrap(err, "get cluster")
		}
		if current.Status.ObservedGeneration == current.Generation && current.Status.PXC.Status == api.AppStateReady {
			return nil
		}
		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}
