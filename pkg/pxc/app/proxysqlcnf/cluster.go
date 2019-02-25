package proxysqlcnf

import (
	"context"
	"fmt"
	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type ClusterManager struct {
	cli client.Client
}

func NewClusterManager(client client.Client) *ClusterManager {
	return &ClusterManager{
		cli: client,
	}
}

func (c *ClusterManager) InitiateCluster(cr *api.PerconaXtraDBCluster) error {
	proxyMembers, err := c.fetchProxyMembers(cr)
	if err != nil {
		return errors.Wrap(err, "failed to initiate the cluster")
	}

	if len(proxyMembers.Items) < 2 {
		return errors.New("can't initialize the cluster. Not enough proxysql nodes")
	}

	hostnameList, err := c.podsHostnameList(proxyMembers.Items)
	if err != nil {
		return errors.Wrap(err, "failed to initiate the cluster")
	}

	for _, proxy := range proxyMembers.Items {

		// get pod hostname
		hostname, err := c.podHostname(proxy)
		if err != nil {
			return errors.Wrap(err, "failed to initiate the cluster")
		}

		// connect to proxysql node
		db, err := NewDB(hostname)
		if err != nil {
			return errors.Wrap(err, "failed to initiate the cluster")
		}

		// initialize node
		if err := db.initializeNode(hostname, hostnameList); err != nil {
			return errors.Wrap(err, "failed to initiate the cluster")
		}
	}
	return nil
}

func (c *ClusterManager) fetchProxyMembers(cr *api.PerconaXtraDBCluster) (*v1.PodList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pods := &v1.PodList{}

	proxy := statefulset.NewProxy(cr)

	if err := c.cli.List(ctx, &client.ListOptions{LabelSelector: labels.SelectorFromSet(proxy.Labels())}, pods); err != nil {
		return nil, errors.Wrap(err, "can't fetch proxysql pods")
	}

	return pods, nil
}

func (c *ClusterManager) fetchPXCMembers(cr *api.PerconaXtraDBCluster) (*v1.PodList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pods := &v1.PodList{}

	node := statefulset.NewNode(cr)

	if err := c.cli.List(ctx, &client.ListOptions{LabelSelector: labels.SelectorFromSet(node.Labels())}, pods); err != nil {
		return nil, fmt.Errorf("can't fetch PXC pods: %v", err)
	}

	return pods, nil
}

// TODO do real work
func (c *ClusterManager) podHostname(pod v1.Pod) (string, error) {
	if pod.Spec.Hostname != "" {
		return pod.Spec.Hostname, nil
	}
	return "", errors.Errorf("can't get hostname from pod %s", pod.Name)
}

func (c *ClusterManager) podsHostnameList(pods []v1.Pod) ([]string, error) {
	list := make([]string, 0)

	for _, pod := range pods {
		hostname, err := c.podHostname(pod)
		if err != nil {
			return nil, errors.Wrap(err, "can't get list of pods hostname")
		}
		list = append(list, hostname)
	}

	return nil, errors.New("can't get pods hostname list")
}
