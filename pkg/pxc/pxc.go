package pxc

import (
	"context"
	"fmt"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const appName = "pxc"

var NoProxyDetectedError = errors.New("can't detect enabled proxy, please enable HAProxy or ProxySQL")

func getFirstReadyPodFQDN(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
	node := statefulset.NewNode(cr)
	sts := node.StatefulSet()

	podList := new(corev1.PodList)
	if err := cl.List(ctx, podList, &client.ListOptions{
		Namespace:     cr.Namespace,
		LabelSelector: labels.SelectorFromSet(node.Labels()),
	}); err != nil {
		return "", errors.Wrap(err, "get pod list")
	}

	readyPods := make([]corev1.Pod, 0)
	for _, pod := range podList.Items {
		if k8s.IsPodReady(pod) {
			readyPods = append(readyPods, pod)
		}
	}
	if len(readyPods) == 0 {
		return "", errors.New("no ready pxc pods")
	}
	if len(readyPods) != int(cr.Spec.PXC.Size) {
		return "", errors.New("waiting for all pxc pods to be ready")
	}
	return podFQDN(readyPods[0].GetName(), sts), nil
}

// GetPrimaryPod returns the primary pod
func GetPrimaryPod(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := GetProxyConnection(cr, cl)
	if errors.Is(err, NoProxyDetectedError) && cr.Spec.PXC.Size == 1 {
		host, err := getFirstReadyPodFQDN(ctx, cl, cr)
		if err != nil {
			return "", errors.Wrap(err, "failed to get first ready pod")
		}
		return host, nil
	} else if err != nil {
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

// GetHostForSidecarBackup returns a pod that can be used for performing backups via xtradb sidecar
func GetHostForSidecarBackup(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBCluster) (string, error) {
	switch {
	case cr.HAProxyEnabled():
		return getNonPrimaryHAProxy(ctx, cl, cr)
	case cr.ProxySQLEnabled():
		return getNonPrimaryProxySQL(cl, cr)
	}
	return getFirstReadyPodFQDN(ctx, cl, cr)
}

func podFQDN(pod string, sts *appsv1.StatefulSet) string {
	return fmt.Sprintf("%s.%s.%s", pod, sts.Name, sts.Namespace)
}

func getNonPrimaryHAProxy(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := GetProxyConnection(cr, cl)
	if err != nil {
		return "", errors.Wrap(err, "failed to get proxy connection")
	}
	defer conn.Close()

	node := statefulset.NewNode(cr)
	sts := node.StatefulSet()
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
		return podFQDN(pod.GetName(), sts), nil
	}
	// None of the non-primary pods are reachable, use the primary host
	return podFQDN(primaryPod, sts), nil
}

func getNonPrimaryProxySQL(cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := GetProxyConnection(cr, cl)
	if err != nil {
		return "", errors.Wrap(err, "failed to get proxy connection")
	}
	defer conn.Close()

	// Try to find a reachable host and return the first one that is reachable
	hosts, err := conn.NonPrimaryHostsProxySQL() // assumes we get a list of pod FQDNs
	if err != nil {
		return "", fmt.Errorf("failed to get non-primary hosts: %w", err)
	}
	if len(hosts) == 0 {
		// No non-primary hosts are reachable, use the primary host
		return conn.PrimaryHost()
	}
	return hosts[0], nil
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
