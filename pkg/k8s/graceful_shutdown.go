package k8s

import (
	"context"
	"time"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var (
	stopCH = make(chan struct{}, 0)
	log    = logf.Log
)

// StartStopSignalHandler starts gorutine which is waiting for
// termination signal and returns chan for indication when operator
// can really stop.
func StartStopSignalHandler(client client.Client) (<-chan struct{}, error) {
	opNS, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return nil, errors.Wrap(err, "get operator namespace")
	}
	go handleStopSignal(client, opNS)
	return stopCH, nil
}

func handleStopSignal(client client.Client, ns string) {
	<-signals.SetupSignalHandler()
	stop(client, ns)
	close(stopCH)
}

// Stop is used to understand, when we need to stop operator(usially SIGTERM)
// to start cleanup process and delete required pxc clusters in current(operator)
// namespace. See K8SPXC-529
func stop(cl client.Client, ns string) {
	log.Info("Got stop signal, starting to list clusters")

	for {
		time.Sleep(5 * time.Second)
		clusterList := &api.PerconaXtraDBClusterList{}

		err := cl.List(context.TODO(), clusterList, &client.ListOptions{
			Namespace: ns,
		})
		if err != nil {
			log.Error(err, "list clusters in current ns", "ns", ns)
			continue
		}

		deleteInProgress := 0

		for _, v := range clusterList.Items {
			if v.ObjectMeta.DeletionTimestamp != nil {
				log.Info("got deletion timestamp,check if cluster ready to delete", "name", v.Name)
				deleteInProgress++
				if len(v.Finalizers) == 0 {
					deleteInProgress--
				}
			}
		}
		if deleteInProgress == 0 {
			log.Info("all clusters are done,exiting")
			return
		}
	}
}
