package backup

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
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

// GetHostForSidecarBackup returns the host to be used for performing sidecar backups.
// Returns: (podName, hostName, error)
func GetHostForSidecarBackup(
	ctx context.Context,
	cl client.Client,
	cluster *api.PerconaXtraDBCluster,
	backup *api.PerconaXtraDBClusterBackup,
) (string, string, error) {
	pod := ""
	switch {
	case backup.Spec.SourcePod != "":
		pod = backup.Spec.SourcePod
	case cluster.HAProxyEnabled():
		if target, err := getHAProxyBackupTargetPod(ctx, cl, cluster); err != nil {
			return "", "", errors.Wrap(err, "failed to get HAProxy backup target pod")
		} else {
			pod = target
		}
	case cluster.ProxySQLEnabled():
		if target, err := getProxySQLBackupTargetPod(ctx, cl, cluster); err != nil {
			return "", "", errors.Wrap(err, "failed to get ProxySQL backup target pod")
		} else {
			pod = target
		}

	default:
		if target, err := pxc.WaitAndGetFirstReadyPod(ctx, cl, cluster); err != nil {
			return "", "", errors.Wrap(err, "failed to get first ready pod")
		} else {
			pod = target
		}
	}
	return pod, pxc.PodFQDN(pod, statefulset.NewNode(cluster).StatefulSet()), nil
}

func getHAProxyBackupTargetPod(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := pxc.GetProxyConnection(ctx, cr, cl)
	if err != nil {
		return "", errors.Wrap(err, "failed to get proxy connection")
	}
	defer conn.Close()

	node := statefulset.NewNode(cr)
	primaryPod, err := conn.Hostname() // assumes we get a pod name (not FQDN)
	if err != nil {
		return "", errors.Wrap(err, "failed to get primary host")
	}

	podList := new(corev1.PodList)
	if err := cl.List(ctx, podList, &client.ListOptions{
		Namespace:     cr.Namespace,
		LabelSelector: labels.SelectorFromSet(node.Labels()),
	}); err != nil {
		return "", errors.Wrap(err, "get pod list")
	}

	for _, pod := range podList.Items {
		if pod.GetName() == primaryPod || !k8s.IsPodReady(pod) {
			continue
		}

		return pod.GetName(), nil
	}
	// None of the non-primary pods are reachable, use the primary host
	return primaryPod, nil
}
func getProxySQLBackupTargetPod(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := pxc.GetProxyConnection(ctx, cr, cl)
	if err != nil {
		return "", errors.Wrap(err, "failed to get proxy connection")
	}
	defer conn.Close()

	node := statefulset.NewNode(cr)
	primaryHost, err := conn.PrimaryHost()
	if err != nil {
		return "", errors.Wrap(err, "failed to get primary host")
	}

	podList := new(corev1.PodList)
	if err := cl.List(ctx, podList, &client.ListOptions{
		Namespace:     cr.Namespace,
		LabelSelector: labels.SelectorFromSet(node.Labels()),
	}); err != nil {
		return "", errors.Wrap(err, "get pod list")
	}

	primaryPod := ""
	for _, pod := range podList.Items {
		if strings.Contains(primaryHost, pod.GetName()) || !k8s.IsPodReady(pod) {
			primaryPod = pod.GetName()
			continue
		}
		return pod.GetName(), nil
	}

	if primaryPod == "" {
		return "", errors.New("no backup hosts found")
	}
	return primaryPod, nil
}
