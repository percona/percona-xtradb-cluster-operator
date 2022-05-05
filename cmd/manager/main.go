package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	certmgrscheme "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/scheme"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

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
	BuildTime string
	scheme    = k8sruntime.NewScheme()
	setupLog  = ctrl.Log.WithName("setup")
)

func printVersion() {
	setupLog.Info(fmt.Sprintf("Git commit: %s Git branch: %s Build time: %s", GitCommit, GitBranch, BuildTime))
	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	sv, err := version.Server()
	if err != nil {
		setupLog.Error(err, "unable to define server version")
		os.Exit(1)
	}
	setupLog.Info("Runs on", "platform", sv.Platform, "version", sv.Info)

	printVersion()

	namespace, err := k8s.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "failed to get watch namespace")
		os.Exit(1)
	}

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "08db1feb.percona.com",
		Namespace:              namespace,
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE
	if strings.Contains(namespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(namespace, ","))
	}

	// Get a config to talk to the apiserver
	config, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(config, options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("Registering Components.")

	// Setup Scheme for k8s resources
	if err := clientgoscheme.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// Setup Scheme for PXC resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// Setup Scheme for cert-manager resources
	if err := certmgrscheme.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	err = webhook.SetupWebhook(mgr)
	if err != nil {
		setupLog.Error(err, "set up validation webhook")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting the Cmd.")

	ctx := k8s.StartStopSignalHandler(mgr.GetClient(), strings.Split(namespace, ","))

	// Start the Cmd
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "manager exited non-zero")
		os.Exit(1)
	}
}
