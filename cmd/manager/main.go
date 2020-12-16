package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	certmgrscheme "github.com/jetstack/cert-manager/pkg/client/clientset/versioned/scheme"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/ready"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	_ "github.com/Percona-Lab/percona-version-service/api"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/webhook"
	"github.com/percona/percona-xtradb-cluster-operator/version"
)

var (
	GitCommit string
	GitBranch string
	log       = logf.Log.WithName("cmd")
)

func printVersion() {
	log.Info(fmt.Sprintf("Git commit: %s Git branch: %s", GitCommit, GitBranch))
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("operator-sdk Version: %v", sdkVersion.Version))
}

func main() {
	flag.Parse()

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(logf.ZapLogger(false))

	sv, err := version.Server()
	if err != nil {
		log.Error(err, "unable to define server version")
		os.Exit(1)
	}
	log.Info("Runs on", "platform", sv.Platform, "version", sv.Info)

	printVersion()

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "failed to get watch namespace")
		os.Exit(1)
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE
	options := manager.Options{
		Namespace: namespace,
	}

	if strings.Contains(namespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(namespace, ","))
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Become the leader before proceeding
	leader.Become(context.TODO(), "percona-xtradb-cluster-operator-lock")

	r := ready.NewFileReady()
	err = r.Set()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
	defer r.Unset()

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, options)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup Scheme for cert-manager resources
	if err := certmgrscheme.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	err = webhook.SetupWebhook(mgr)
	if err != nil {
		log.Error(err, "set up validation webhook")
		os.Exit(1)
	}

	log.Info("Starting the Cmd.")

	stopCH := k8s.StartStopSignalHandler(mgr.GetClient(), strings.Split(namespace, ","))

	// Start the Cmd
	if err := mgr.Start(stopCH); err != nil {
		log.Error(err, "manager exited non-zero")
		os.Exit(1)
	}
}
