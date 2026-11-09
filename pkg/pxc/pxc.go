package pxc

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

var NoProxyDetectedError = errors.New("can't detect enabled proxy, please enable HAProxy or ProxySQL")

// waitAndGetFirstReadyPodFQDN waits for all pods to be ready and returns the FQDN of the first ready pod
func waitAndGetFirstReadyPodFQDN(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
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
	return PodFQDN(readyPods[0].GetName(), sts), nil
}

// GetPrimaryPod returns the primary pod
func GetPrimaryPod(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBCluster,
) (string, error) {
	conn, err := GetProxyConnection(ctx, cr, cl)
	if errors.Is(err, NoProxyDetectedError) && cr.Spec.PXC.Size == 1 {
		host, err := waitAndGetFirstReadyPodFQDN(ctx, cl, cr)
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
	host, err := conn.PrimaryHost()
	if err != nil && errors.Is(err, queries.ErrNotFound) && cr.IsReadOnly() {
		// Replica clusters have all nodes in the reader hostgroup (10).
		// There is no writer, so fall back to any online reader host.
		return conn.ReaderHost()
	}
	return host, err
}

// GetHostForSidecarBackup returns a pod that can be used for performing backups via xtradb sidecar
func GetHostForSidecarBackup(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBCluster,
) (string, error) {
	switch {
	case cr.HAProxyEnabled():
		return getNonPrimaryHAProxy(ctx, cl, cr)
	case cr.ProxySQLEnabled():
		return getNonPrimaryProxySQL(ctx, cl, cr)
	}
	return waitAndGetFirstReadyPodFQDN(ctx, cl, cr)
}

func PodFQDN(pod string, sts *appsv1.StatefulSet) string {
	return fmt.Sprintf("%s.%s.%s", pod, sts.Name, sts.Namespace)
}

func getNonPrimaryHAProxy(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := GetProxyConnection(ctx, cr, cl)
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
		return PodFQDN(pod.GetName(), sts), nil
	}
	// None of the non-primary pods are reachable, use the primary host
	return PodFQDN(primaryPod, sts), nil
}

func getNonPrimaryProxySQL(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (string, error) {
	conn, err := GetProxyConnection(ctx, cr, cl)
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

// ProxySQLPodsWithoutOnlineWriter returns the ProxySQL pods that have no
// ONLINE server in the writer hostgroup. The pods are not clustered, each one
// keeps its own runtime_mysql_servers, so every pod is checked separately.
// Rows with other statuses do not count: the scheduler keeps nodes in
// maintenance or mid-failover in the hostgroup as OFFLINE_SOFT, and ProxySQL
// marks unreachable ones SHUNNED. A replica cluster has no writer by design, so
// the caller is expected to check cr.IsReadOnly(). A pod that cannot be queried
// is skipped and reported in the returned error alongside the result.
func ProxySQLPodsWithoutOnlineWriter(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) ([]string, error) {
	if !cr.ProxySQLEnabled() {
		return nil, NoProxyDetectedError
	}

	var pods []string
	var errs []error
	for i := range cr.Spec.ProxySQL.Size {
		pod := fmt.Sprintf("%s-proxysql-%d", cr.Name, i)
		host := fmt.Sprintf("%s.%s-proxysql-unready.%s", pod, cr.Name, cr.Namespace)

		// The same budget the pod's own readiness probe has for the same query
		// on the same port. The field is defaulted only for CR >= 1.21.0, which
		// the caller is expected to check: 0 would mean no timeout at all.
		conn, err := queries.New(ctx, cl, cr.Namespace, "internal-"+cr.Name, users.ProxyAdmin, host, 6032, cr.Spec.ProxySQL.ReadinessProbes.TimeoutSeconds)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "connect to %s", pod))
			continue
		}

		count, err := conn.OnlineServersCount(queries.WriterHostgroupID)
		_ = conn.Close()
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "count online servers in writer hostgroup on %s", pod))
			continue
		}

		if count == 0 {
			pods = append(pods, pod)
		}
	}

	return pods, stderrors.Join(errs...)
}

// GetProxyConnection returns a new connection through the proxy (ProxySQL or HAProxy)
func GetProxyConnection(ctx context.Context, cr *api.PerconaXtraDBCluster, cl client.Client) (queries.Database, error) {
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
		db, err := queries.New(ctx, cl, cr.Namespace, secrets, user, host, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
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
