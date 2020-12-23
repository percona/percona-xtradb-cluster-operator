package k8s

import (
	"context"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	log = logf.Log
)

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

	for {
		time.Sleep(5 * time.Second)
		inProgress := len(namespaces)

		for _, ns := range namespaces {

			clusterList := &api.PerconaXtraDBClusterList{}

			err := cl.List(context.TODO(), clusterList, &client.ListOptions{
				Namespace: ns,
			})
			if err != nil {
				log.Error(err, "list clusters in current ns", "ns", ns)
				continue
			}

			clusterDeleteInProgress := 0

			for _, v := range clusterList.Items {
				if v.ObjectMeta.DeletionTimestamp != nil {
					log.Info("got deletion timestamp,check if cluster ready to delete", "name", v.Name)
					clusterDeleteInProgress++
					if len(v.Finalizers) == 0 {
						clusterDeleteInProgress--
					}
				}
			}
			if clusterDeleteInProgress == 0 {
				inProgress--
			}
		}

		if inProgress == 0 {
			return
		}
	}
}
