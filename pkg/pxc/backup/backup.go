package backup

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/pkg/errors"
)

type Backup struct {
	cluster            string
	namespace          string
	image              string
	imagePullSecrets   []corev1.LocalObjectReference
	imagePullPolicy    corev1.PullPolicy
	serviceAccountName string
}

func New(cr *api.PerconaXtraDBCluster) *Backup {
	return &Backup{
		cluster:            cr.Name,
		namespace:          cr.Namespace,
		image:              cr.Spec.Backup.Image,
		imagePullSecrets:   cr.Spec.Backup.ImagePullSecrets,
		imagePullPolicy:    cr.Spec.Backup.ImagePullPolicy,
		serviceAccountName: cr.Spec.Backup.ServiceAccountName,
	}
}

// GetHostForSidecarBackup returns the PXC pod a sidecar backup should run
// against, as (podName, hostName, error).
func GetHostForSidecarBackup(
	ctx context.Context,
	cl client.Client,
	cluster *api.PerconaXtraDBCluster,
	backup *api.PerconaXtraDBClusterBackup,
) (string, string, error) {
	if pinned := pinnedSourcePod(backup); pinned != "" {
		node := pxc.NewSourceNode(cluster, pinned)
		return node.Pod, node.FQDN, nil
	}

	node, err := pxc.GetNonPrimarySourceNode(ctx, cl, cluster)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to select a backup source node")
	}

	return node.Pod, node.FQDN, nil
}

// pinnedSourcePod returns the pod this backup is already tied to, or "" when it
// is free to pick one.
func pinnedSourcePod(backup *api.PerconaXtraDBClusterBackup) string {
	if backup.Status.SourcePod != "" {
		return backup.Status.SourcePod
	}
	return backup.Spec.SourcePod
}
