package main

import (
	"context"
	"runtime"
	"time"

	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/sirupsen/logrus"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/version"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	const (
		resource = "pxc.percona.com/v1alpha1"
		kindPXC  = "PerconaXtraDBCluster"
		kindBCP  = "PerconaXtraDBBackup"
	)

	printVersion()

	sv, err := version.Server()
	if err != nil {
		logrus.Fatalf("Unable to define server version: %v", err)
	}
	logrus.WithFields(logrus.Fields{
		"platform": sv.Platform,
		"version":  sv.Info,
	}).Infof("Server")

	sdk.ExposeMetricsPort()

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Fatalf("failed to get watch namespace: %v", err)
	}

	resyncPeriod := 5 * time.Second

	logrus.WithFields(logrus.Fields{
		"resource":  resource,
		"objects":   []string{kindPXC, kindBCP},
		"namespace": namespace,
		"sync":      resyncPeriod,
	}).Infof("Watching")

	sdk.Watch(resource, kindPXC, namespace, resyncPeriod)
	sdk.Watch(resource, kindBCP, namespace, resyncPeriod)

	sdk.Handle(pxc.New(*sv))
	sdk.Run(context.TODO())
}
