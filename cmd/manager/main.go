package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "github.com/Percona-Lab/percona-version-service/api"
	certmgrscheme "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/scheme"
	"github.com/go-logr/logr"
	"github.com/kelseyhightower/envconfig"
	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	eventsv1 "k8s.io/api/events/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlWebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/features"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/webhook"
)

var (
	GitCommit string
	GitBranch string
	BuildTime string
	scheme    = k8sruntime.NewScheme()
	setupLog  = ctrl.Log.WithName("setup")
)

func main() {
	var metricsAddr string
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	opts := zap.Options{
		Encoder: getLogEncoder(setupLog),
		Level:   getLogLevel(setupLog),
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	klog.SetLogger(ctrl.Log)

	sv, err := version.Server()
	if err != nil {
		setupLog.Error(err, "unable to define server version")
		os.Exit(1)
	}
	setupLog.Info("Runs on", "platform", sv.Platform, "version", sv.Info)

	setupLog.Info("Manager starting up", "gitCommit", GitCommit, "gitBranch", GitBranch,
		"buildTime", BuildTime, "goVersion", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH)

	namespace, err := k8s.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "failed to get watch namespace")
		os.Exit(1)
	}
	operatorNamespace, err := k8s.GetOperatorNamespace()
	if err != nil {
		setupLog.Error(err, "failed to get operators' namespace")
		os.Exit(1)
	}

	envs := new(envConfig)
	if err := envconfig.Process("", envs); err != nil {
		setupLog.Error(err, "failed to parse env vars")
		os.Exit(1)
	}

	fg := features.NewGate()
	if err := fg.Set(envs.FeatureGates); err != nil {
		setupLog.Error(err, "failed to set feature gates")
		os.Exit(1)
	}
	fgCtx := features.NewContextWithGate(context.Background(), fg)
	setupLog.Info("Feature gates",
		// These are set by the user
		"PXCO_FEATURE_GATES", features.ShowAssigned(fgCtx),
		// These are enabled, including features that are on by default
		"enabled", features.ShowEnabled(fgCtx),
	)

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsServer.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		WebhookServer: ctrlWebhook.NewServer(ctrlWebhook.Options{
			Port: 9443,
		}),
		BaseContext: func() context.Context {
			return features.NewContextWithGate(context.Background(), fg)
		},
	}

	err = configureLeaderElection(&options, envs, operatorNamespace)
	if err != nil {
		setupLog.Error(err, "failed to configure leader election")
		os.Exit(1)
	}

	err = configureGroupKindConcurrency(&options, envs)
	if err != nil {
		setupLog.Error(err, "failed to configure group kind concurrency")
		os.Exit(1)
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE
	if len(namespace) > 0 {
		namespaces := make(map[string]cache.Config)
		for _, ns := range append(strings.Split(namespace, ","), operatorNamespace) {
			namespaces[ns] = cache.Config{}
		}
		options.Cache.DefaultNamespaces = namespaces
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

	ctx := k8s.StartStopSignalHandler(mgr.GetClient(), strings.Split(namespace, ","))

	if err := webhook.SetupWebhook(ctx, mgr); err != nil {
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

	err = mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&eventsv1.Event{},
		"regarding.name",
		func(rawObj client.Object) []string {
			event := rawObj.(*eventsv1.Event)
			return []string{event.Regarding.Name}
		},
	)
	if err != nil {
		setupLog.Error(err, "unable to index field")
		os.Exit(1)
	}

	setupLog.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "manager exited non-zero")
		os.Exit(1)
	}
}

func getLogEncoder(log logr.Logger) zapcore.Encoder {
	consoleEnc := zapcore.NewConsoleEncoder(uzap.NewDevelopmentEncoderConfig())

	s, found := os.LookupEnv("LOG_STRUCTURED")
	if !found {
		return consoleEnc
	}

	useJson, err := strconv.ParseBool(s)
	if err != nil {
		log.Info("Can't parse LOG_STRUCTURED env var, using console logger", "envVar", s)
		return consoleEnc
	}
	if !useJson {
		return consoleEnc
	}

	return zapcore.NewJSONEncoder(uzap.NewProductionEncoderConfig())
}

func getLogLevel(log logr.Logger) zapcore.LevelEnabler {
	l, found := os.LookupEnv("LOG_LEVEL")
	if !found {
		return zapcore.InfoLevel
	}

	switch strings.ToUpper(l) {
	case "VERBOSE", "DEBUG":
		return zapcore.DebugLevel
	case "INFO":
		return zapcore.InfoLevel
	case "ERROR":
		return zapcore.ErrorLevel
	default:
		log.Info("Unsupported log level", "level", l)
		return zapcore.InfoLevel
	}
}

const defaultElectionID = "08db1feb.percona.com"

type envConfig struct {
	LeaderElection   bool          `default:"true" envconfig:"PXCO_LEADER_ELECTION_ENABLED"`
	LeaderElectionID string        `envconfig:"PXCO_LEADER_ELECTION_LEASE_NAME"`
	LeaseDuration    time.Duration `default:"15s" envconfig:"PXCO_LEADER_ELECTION_LEASE_DURATION"`
	RenewDeadline    time.Duration `default:"10s" envconfig:"PXCO_LEADER_ELECTION_RENEW_DEADLINE"`
	RetryPeriod      time.Duration `default:"2s" envconfig:"PXCO_LEADER_ELECTION_RETRY_PERIOD"`

	FeatureGates string `envconfig:"PXCO_FEATURE_GATES"`

	Workers *int `envconfig:"MAX_CONCURRENT_RECONCILES"`
}

func configureLeaderElection(options *ctrl.Options, envs *envConfig, operatorNamespace string) error {
	options.LeaderElection = envs.LeaderElection
	if envs.LeaderElection {
		options.LeaderElectionID = defaultElectionID
	}

	options.LeaseDuration = &envs.LeaseDuration
	options.RenewDeadline = &envs.RenewDeadline
	options.RetryPeriod = &envs.RetryPeriod

	if lease := envs.LeaderElectionID; envs.LeaderElection && len(lease) > 0 {
		if errs := validation.IsDNS1123Subdomain(lease); len(errs) > 0 {
			return fmt.Errorf("value for PXCO_LEADER_ELECTION_LEASE_NAME is invalid: %v", errs)
		}
		options.LeaderElectionID = lease
		options.LeaderElectionNamespace = operatorNamespace
	}

	return nil
}

func configureGroupKindConcurrency(options *ctrl.Options, envs *envConfig) error {
	groupKinds := []string{
		"PerconaXtraDBCluster." + pxcv1.SchemeGroupVersion.Group,
		"PerconaXtraDBClusterBackup." + pxcv1.SchemeGroupVersion.Group,
		"PerconaXtraDBClusterRestore." + pxcv1.SchemeGroupVersion.Group,
	}

	const defaultConcurrency = 1
	options.Controller.GroupKindConcurrency = make(map[string]int, len(groupKinds))
	for _, gk := range groupKinds {
		options.Controller.GroupKindConcurrency[gk] = defaultConcurrency
	}

	if envs.Workers != nil {
		if *envs.Workers <= 0 {
			return fmt.Errorf("MAX_CONCURRENT_RECONCILES must be a positive number: %d", *envs.Workers)
		}
		for _, gk := range groupKinds {
			options.Controller.GroupKindConcurrency[gk] = *envs.Workers
		}
	}
	return nil
}
