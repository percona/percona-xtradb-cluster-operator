package main

import (
	"context"
	"runtime"
	"time"

	stub "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/stub"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/version"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

	"github.com/sirupsen/logrus"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	printVersion()

	sv, err := version.Server()
	if err != nil {
		logrus.Fatalf("Unable to define server version: %v", err)
	}
	logrus.Infof("Server: %s, %v", sv.Platform, sv.Info)

	sdk.ExposeMetricsPort()

	resource := "pxc.percona.com/v1alpha1"
	kind := "PerconaXtraDBCluster"
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Fatalf("failed to get watch namespace: %v", err)
	}

	resyncPeriod := 5 * time.Second
	logrus.Infof("Watching %s, %s, %s, %v", resource, kind, namespace, resyncPeriod)
	sdk.Watch(resource, kind, namespace, resyncPeriod)
	sdk.Handle(stub.NewHandler(*sv))
	sdk.Run(context.TODO())
}
