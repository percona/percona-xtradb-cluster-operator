package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	stdErrors "errors"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
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

	err = waitForPVCShutdown(ctx, cl, ls, cr.Namespace)
	if err != nil {
		return errors.Wrap(err, "shutdown pvc")
	}

	return nil
}

var NoProxyDetectedError = errors.New("can't detect enabled proxy, please enable HAProxy or ProxySQL")

// GetPrimaryPod returns the IP/host of the primary pod for the given cluster
func GetPrimaryPod(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := GetProxyConnection(cr, cl)
	if err != nil {
		if errors.Is(err, NoProxyDetectedError) && cr.Spec.PXC.Size == 1 {
			firstReadyPod := func() (string, error) {
				sts := statefulset.NewNode(cr)

				podList := new(corev1.PodList)
				if err := cl.List(ctx, podList, &client.ListOptions{
					Namespace:     cr.Namespace,
					LabelSelector: labels.SelectorFromSet(sts.Labels()),
				}); err != nil {
					return "", errors.Wrap(err, "get pod list")
				}

				readyPods := make([]corev1.Pod, 0)
				for _, pod := range podList.Items {
					if IsPodReady(pod) {
						readyPods = append(readyPods, pod)
					}
				}
				if len(readyPods) == 0 {
					return "", errors.New("no ready pxc pods")
				}
				if len(readyPods) != int(cr.Spec.PXC.Size) {
					return "", errors.New("waiting for pxc resize")
				}

				return readyPods[0].Status.PodIP, nil
			}
			host, rerr := firstReadyPod()
			if rerr == nil {
				return host, nil
			}

			err = stdErrors.Join(rerr, err)
		}
		return "", errors.Wrap(err, "failed to get proxy connection")
	}
	defer conn.Close()

	if cr.HAProxyEnabled() {
		host, err := conn.Hostname()
		if err != nil {
			return "", err
		}

		return host, nil
	}

	return conn.PrimaryHost()
}

// GetProxyConnection returns a new connection through the proxy (ProxySQL or HAProxy)
func GetProxyConnection(cr *api.PerconaXtraDBCluster, cl client.Client) (queries.Database, error) {
	var database queries.Database
	var user, host string
	var port, proxySize int32

	if cr.ProxySQLEnabled() {
		user = users.ProxyAdmin
		host = fmt.Sprintf("%s-proxysql-unready.%s", cr.ObjectMeta.Name, cr.Namespace)
		proxySize = cr.Spec.ProxySQL.Size
		port = 6032
	} else if cr.HAProxyEnabled() {
		user = users.Monitor
		host = fmt.Sprintf("%s-haproxy.%s", cr.Name, cr.Namespace)
		proxySize = cr.Spec.HAProxy.Size

		hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
		if err != nil {
			return database, errors.Wrap(err, "check if config has proxy_protocol_networks key")
		}

		port = 3306
		if hasKey && cr.CompareVersionWith("1.6.0") >= 0 {
			port = 33062
		}
	} else {
		return database, NoProxyDetectedError
	}

	secrets := cr.Spec.SecretsName
	if cr.CompareVersionWith("1.6.0") >= 0 {
		secrets = "internal-" + cr.Name
	}

	for i := 0; ; i++ {
		db, err := queries.New(cl, cr.Namespace, secrets, user, host, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
		if err != nil && i < int(proxySize) {
			time.Sleep(time.Second)
		} else if err != nil && i == int(proxySize) {
			return database, err
		} else {
			database = db
			break
		}
	}

	return database, nil
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

func waitForPVCShutdown(ctx context.Context, cl client.Client, ls map[string]string, namespace string) error {
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

		if len(pvcs.Items) == 1 {
			return nil
		}

		time.Sleep(time.Second * 1)
	}

	return errors.Errorf("exceeded wait limit")
}
