package k8s

import (
	"context"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var log = logf.Log

// StartStopSignalHandler starts gorutine which is waiting for
// termination signal and returns chan for indication when operator
// can really stop.
func StartStopSignalHandler(client client.Client, namespaces []string) <-chan struct{} {
	stopCH := make(chan struct{})
	go handleStopSignal(client, namespaces, stopCH)
	return stopCH
}

func handleStopSignal(client client.Client, namespaces []string, stopCH chan struct{}) {
	<-signals.SetupSignalHandler()
	stop(client, namespaces)
	close(stopCH)
}

// Stop is used to understand, when we need to stop operator(usially SIGTERM)
// to start cleanup process and delete required pxc clusters in current(operator)
// namespace. See K8SPXC-529
func stop(cl client.Client, namespaces []string) {
	log.Info("Got stop signal, starting to list clusters")

	readyToDelete := false

	for !readyToDelete {
		time.Sleep(5 * time.Second)
		ready, err := checkClusters(cl, namespaces)
		if err != nil {
			log.Error(err, "delete clusters")
		}
		readyToDelete = ready
	}
}

func checkClusters(cl client.Client, namespaces []string) (bool, error) {
	for _, ns := range namespaces {

		clusterList := &api.PerconaXtraDBClusterList{}

		err := cl.List(context.TODO(), clusterList, &client.ListOptions{
			Namespace: ns,
		})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return false, errors.Wrapf(err, "list clusters in namespace: %s", ns)
		}

		if !isClustersReadyToDelete(clusterList.Items) {
			return false, nil
		}
	}
	return true, nil
}

func isClustersReadyToDelete(list []api.PerconaXtraDBCluster) bool {
	for _, v := range list {
		if v.ObjectMeta.DeletionTimestamp != nil && len(v.Finalizers) != 0 {
			return false
		}
	}
	return true
}
