package k8s

import (
	"context"
	"time"

	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

var log = logf.Log

// StartStopSignalHandler starts gorutine which is waiting for termination
// signal and returns a context which is cancelled when operator can really
// stop.
func StartStopSignalHandler(client client.Client, namespaces []string) context.Context {
	ctx, shutdownFunc := context.WithCancel(context.Background())
	go handleStopSignal(ctx, client, namespaces, shutdownFunc)
	return ctx
}

func handleStopSignal(ctx context.Context, client client.Client, namespaces []string, shutdownFunc context.CancelFunc) {
	<-signals.SetupSignalHandler().Done()
	stop(ctx, client, namespaces)
	shutdownFunc()
}

// Stop is used to understand, when we need to stop operator(usially SIGTERM)
// to start cleanup process and delete required pxc clusters in current(operator)
// namespace. See K8SPXC-529
func stop(ctx context.Context, cl client.Client, namespaces []string) {
	log.Info("Got stop signal, starting to list clusters")

	readyToDelete := false

	for !readyToDelete {
		time.Sleep(5 * time.Second)
		ready, err := checkClusters(ctx, cl, namespaces)
		if err != nil {
			log.Error(err, "delete clusters")
		}
		readyToDelete = ready
	}
}

func checkClusters(ctx context.Context, cl client.Client, namespaces []string) (bool, error) {
	for _, ns := range namespaces {

		clusterList := &api.PerconaXtraDBClusterList{}

		err := cl.List(ctx, clusterList, &client.ListOptions{Namespace: ns})
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, errors.Wrapf(err, "list clusters in namespace: %s", ns)
		}

		if clusterList.HasUnfinishedFinalizers() {
			return false, nil
		}

		bcpList := api.PerconaXtraDBClusterBackupList{}

		err = cl.List(ctx, &bcpList, &client.ListOptions{Namespace: ns})
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, errors.Wrap(err, "failed to get backup object")
		}

		if bcpList.HasUnfinishedFinalizers() {
			return false, nil
		}
	}

	return true, nil
}
