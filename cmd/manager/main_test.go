package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/features"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func parseEnvConfig(t *testing.T) *envConfig {
	t.Helper()
	envs := new(envConfig)
	require.NoError(t, envconfig.Process("", envs))
	return envs
}

func TestConfigureLeaderElection(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		envs := parseEnvConfig(t)
		options := ctrl.Options{}
		err := configureLeaderElection(&options, envs, "test-ns")
		require.NoError(t, err)

		assert.True(t, options.LeaderElection)
		assert.Equal(t, defaultElectionID, options.LeaderElectionID)
		assert.Equal(t, 15*time.Second, *options.LeaseDuration)
		assert.Equal(t, 10*time.Second, *options.RenewDeadline)
		assert.Equal(t, 2*time.Second, *options.RetryPeriod)
		assert.Empty(t, options.LeaderElectionNamespace)
	})

	t.Run("custom durations", func(t *testing.T) {
		t.Setenv("PXCO_LEADER_ELECTION_LEASE_DURATION", "120s")
		t.Setenv("PXCO_LEADER_ELECTION_RENEW_DEADLINE", "80s")
		t.Setenv("PXCO_LEADER_ELECTION_RETRY_PERIOD", "20s")

		envs := parseEnvConfig(t)
		options := ctrl.Options{}
		err := configureLeaderElection(&options, envs, "test-ns")
		require.NoError(t, err)

		assert.Equal(t, 120*time.Second, *options.LeaseDuration)
		assert.Equal(t, 80*time.Second, *options.RenewDeadline)
		assert.Equal(t, 20*time.Second, *options.RetryPeriod)
	})

	t.Run("invalid duration", func(t *testing.T) {
		t.Setenv("PXCO_LEADER_ELECTION_LEASE_DURATION", "invalid")

		envs := new(envConfig)
		err := envconfig.Process("", envs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PXCO_LEADER_ELECTION_LEASE_DURATION")
	})

	t.Run("leader election disabled", func(t *testing.T) {
		t.Setenv("PXCO_LEADER_ELECTION_ENABLED", "false")

		envs := parseEnvConfig(t)
		options := ctrl.Options{}
		err := configureLeaderElection(&options, envs, "test-ns")
		require.NoError(t, err)

		assert.False(t, options.LeaderElection)
		assert.Empty(t, options.LeaderElectionID)
	})

	t.Run("invalid boolean for enabled", func(t *testing.T) {
		t.Setenv("PXCO_LEADER_ELECTION_ENABLED", "not-a-bool")

		envs := new(envConfig)
		err := envconfig.Process("", envs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PXCO_LEADER_ELECTION_ENABLED")
	})

	t.Run("custom lease name valid", func(t *testing.T) {
		t.Setenv("PXCO_LEADER_ELECTION_LEASE_NAME", "my-custom-lease")

		envs := parseEnvConfig(t)
		options := ctrl.Options{}
		err := configureLeaderElection(&options, envs, "operator-ns")
		require.NoError(t, err)

		assert.True(t, options.LeaderElection)
		assert.Equal(t, "my-custom-lease", options.LeaderElectionID)
		assert.Equal(t, "operator-ns", options.LeaderElectionNamespace)
	})

	t.Run("custom lease name invalid", func(t *testing.T) {
		t.Setenv("PXCO_LEADER_ELECTION_LEASE_NAME", "INVALID_NAME")

		envs := parseEnvConfig(t)
		options := ctrl.Options{}
		err := configureLeaderElection(&options, envs, "test-ns")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PXCO_LEADER_ELECTION_LEASE_NAME")
	})

	t.Run("invalid lease name with election disabled", func(t *testing.T) {
		t.Setenv("PXCO_LEADER_ELECTION_ENABLED", "false")
		t.Setenv("PXCO_LEADER_ELECTION_LEASE_NAME", "INVALID_NAME")

		envs := parseEnvConfig(t)
		options := ctrl.Options{}
		err := configureLeaderElection(&options, envs, "test-ns")
		require.NoError(t, err)

		assert.False(t, options.LeaderElection)
	})
}

func TestConfigureGroupKindConcurrency(t *testing.T) {
	tests := map[string]struct {
		envValue      string
		expectedVal   map[string]int
		expectedError bool
	}{
		"default concurrency when env not set": {
			envValue: "",
			expectedVal: map[string]int{
				"PerconaXtraDBCluster." + pxcv1.SchemeGroupVersion.Group:        1,
				"PerconaXtraDBClusterBackup." + pxcv1.SchemeGroupVersion.Group:  1,
				"PerconaXtraDBClusterRestore." + pxcv1.SchemeGroupVersion.Group: 1,
			},
		},
		"valid custom concurrency": {
			envValue: "5",
			expectedVal: map[string]int{
				"PerconaXtraDBCluster." + pxcv1.SchemeGroupVersion.Group:        5,
				"PerconaXtraDBClusterBackup." + pxcv1.SchemeGroupVersion.Group:  5,
				"PerconaXtraDBClusterRestore." + pxcv1.SchemeGroupVersion.Group: 5,
			},
		},
		"zero value rejected": {
			envValue:      "0",
			expectedError: true,
		},
		"negative value rejected": {
			envValue:      "-1",
			expectedError: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("MAX_CONCURRENT_RECONCILES", tt.envValue)
			}

			envs := parseEnvConfig(t)
			options := ctrl.Options{
				Scheme: scheme,
				Metrics: metricsServer.Options{
					BindAddress: "bind-address",
				},
				HealthProbeBindAddress: "probe-address",
				LeaderElection:         true,
				LeaderElectionID:       "election-id",
			}

			err := configureGroupKindConcurrency(&options, envs)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// ensure that the original options are not affected
			assert.Equal(t, scheme, options.Scheme)
			assert.Equal(t, metricsServer.Options{
				BindAddress: "bind-address",
			}, options.Metrics)
			assert.Equal(t, "probe-address", options.HealthProbeBindAddress)
			assert.Equal(t, "election-id", options.LeaderElectionID)
			assert.True(t, options.LeaderElection)
			assert.Equal(t, tt.expectedVal, options.Controller.GroupKindConcurrency)
		})
	}

	t.Run("invalid non-integer value", func(t *testing.T) {
		t.Setenv("MAX_CONCURRENT_RECONCILES", "invalid")

		envs := new(envConfig)
		err := envconfig.Process("", envs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "MAX_CONCURRENT_RECONCILES")
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestConfigureCacheWithoutSecretMetadataCache(t *testing.T) {
	assert.False(t, features.NewGate().Enabled(features.SecretMetadataCache), "the Secret metadata cache must be off by default")

	options := ctrl.Options{}
	configureCache(&options, "pxc", "default", false)
	assert.Nil(t, options.NewCache, "the stock cache must be used")
	assert.Nil(t, options.NewClient, "the stock client must be used")
	assert.Len(t, options.Cache.DefaultNamespaces, 2, "WATCH_NAMESPACE must still apply")
}

func TestConfigureCache(t *testing.T) {
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testEnv.Stop()) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	k8sClient, err := client.New(cfg, client.Options{})
	require.NoError(t, err)

	const namespace, otherNamespace, operatorNamespace = "pxc", "other", "default"
	for _, ns := range []string{namespace, otherNamespace} {
		require.NoError(t, k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}))
	}
	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1-secrets", Namespace: namespace},
		StringData: map[string]string{"root": "password"},
	}
	unwatchedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "unwatched", Namespace: otherNamespace},
	}
	for _, secret := range []*corev1.Secret{userSecret, unwatchedSecret} {
		require.NoError(t, k8sClient.Create(ctx, secret))
	}

	tests := map[string]struct {
		watchNamespace     string
		expectedNamespaces []string
		unwatchedReadable  bool
	}{
		"cluster-wide": {
			unwatchedReadable: true,
		},
		"watch namespaces": {
			watchNamespace:     namespace,
			expectedNamespaces: []string{namespace, operatorNamespace},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			options := ctrl.Options{
				Scheme:                 scheme,
				Metrics:                metricsServer.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				Controller:             config.Controller{SkipNameValidation: ptr.To(true)},
			}
			configureCache(&options, tt.watchNamespace, operatorNamespace, true)

			var namespaces []string
			for ns := range options.Cache.DefaultNamespaces {
				namespaces = append(namespaces, ns)
			}
			assert.ElementsMatch(t, tt.expectedNamespaces, namespaces)

			var fullSecretWatches atomic.Int64
			managerCfg := rest.CopyConfig(cfg)
			managerCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
				return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if strings.HasSuffix(req.URL.Path, "/secrets") && req.URL.Query().Get("watch") == "true" &&
						!strings.Contains(req.Header.Get("Accept"), "PartialObjectMetadata") {
						fullSecretWatches.Add(1)
					}
					return rt.RoundTrip(req)
				})
			}

			mgr, err := ctrl.NewManager(managerCfg, options)
			require.NoError(t, err, "the manager must build with these options, as in main")

			reconciled := make(chan reconcile.Request, 16)
			require.NoError(t, ctrl.NewControllerManagedBy(mgr).
				Named("secret-watch").
				For(&corev1.Secret{}, builder.OnlyMetadata).
				Complete(reconcile.Func(func(_ context.Context, req reconcile.Request) (reconcile.Result, error) {
					reconciled <- req
					return reconcile.Result{}, nil
				})))

			managerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			stopped := make(chan error, 1)
			go func() { stopped <- mgr.Start(managerCtx) }()
			defer func() {
				cancel()
				assert.NoError(t, <-stopped, "the manager must stop cleanly")
			}()
			require.True(t, mgr.GetCache().WaitForCacheSync(managerCtx), "cache did not sync")

			operatorClient := mgr.GetClient()
			secret := &corev1.Secret{}
			require.NoError(t, operatorClient.Get(managerCtx, client.ObjectKeyFromObject(userSecret), secret))
			assert.Equal(t, []byte("password"), secret.Data["root"])

			err = operatorClient.Get(managerCtx, client.ObjectKey{Namespace: namespace, Name: "missing"}, &corev1.Secret{})
			assert.True(t, k8serrors.IsNotFound(err), "a missing Secret must be NotFound, got %v", err)

			err = operatorClient.Get(managerCtx, client.ObjectKeyFromObject(unwatchedSecret), &corev1.Secret{})
			if tt.unwatchedReadable {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err, "a Secret outside WATCH_NAMESPACE must not be read")
			}

			secrets := &corev1.SecretList{}
			require.NoError(t, operatorClient.List(managerCtx, secrets), "Secrets must be listable without a namespace")
			var listed []client.ObjectKey
			for i := range secrets.Items {
				listed = append(listed, client.ObjectKeyFromObject(&secrets.Items[i]))
			}
			assert.Contains(t, listed, client.ObjectKeyFromObject(userSecret))
			err = operatorClient.List(managerCtx, &corev1.SecretList{}, client.InNamespace(unwatchedSecret.Namespace))
			if tt.unwatchedReadable {
				assert.Contains(t, listed, client.ObjectKeyFromObject(unwatchedSecret))
				assert.NoError(t, err)
			} else {
				assert.NotContains(t, listed, client.ObjectKeyFromObject(unwatchedSecret))
				assert.Error(t, err, "Secrets outside WATCH_NAMESPACE must not be listed")
			}

			written := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "written-" + strings.ReplaceAll(name, " ", "-"), Namespace: namespace},
				Data:       map[string][]byte{"root": []byte("written")},
			}
			require.NoError(t, operatorClient.Create(managerCtx, written))
			defer func() { assert.NoError(t, operatorClient.Delete(ctx, written)) }()
			secret = &corev1.Secret{}
			require.NoError(t, operatorClient.Get(managerCtx, client.ObjectKeyFromObject(written), secret),
				"the operator must read the Secret it has just created")
			assert.Equal(t, []byte("written"), secret.Data["root"])

			userSecretRequest := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(userSecret)}
			require.Eventually(t, func() bool {
				for {
					select {
					case req := <-reconciled:
						if req == userSecretRequest {
							return true
						}
					default:
						return false
					}
				}
			}, 10*time.Second, 50*time.Millisecond, "the metadata watch must deliver Secret events")

			assert.Zero(t, fullSecretWatches.Load(), "Secrets must only be watched as metadata")
		})
	}
}

func TestOperatorControllersWithConfigureCache(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testEnv.Stop()) })

	// The restore controller builds its exec client from the kubeconfig.
	user, err := testEnv.AddUser(envtest.User{Name: "operator", Groups: []string{"system:masters"}}, cfg)
	require.NoError(t, err)
	kubeconfig, err := user.KubeConfig()
	require.NoError(t, err)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigPath, kubeconfig, 0o600))
	t.Setenv("KUBECONFIG", kubeconfigPath)
	t.Setenv("DISABLE_TELEMETRY", "true")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	crYAML, err := os.ReadFile(filepath.Join("..", "..", "deploy", "cr.yaml"))
	require.NoError(t, err)

	// A controller watching Secrets with data under the metadata cache never
	// syncs, and the manager stops after this timeout.
	const cacheSyncTimeout = 3 * time.Second

	tests := map[string]struct {
		secretMetadataCache bool
		watchNamespace      bool
		fullSecretWatched   bool
	}{
		"secret metadata cache":                        {secretMetadataCache: true},
		"secret metadata cache in a watched namespace": {secretMetadataCache: true, watchNamespace: true},
		"stock cache for rollback":                     {fullSecretWatched: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			namespace := strings.ReplaceAll(name, " ", "-")
			require.NoError(t, k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}))

			options := ctrl.Options{
				Scheme:                 scheme,
				Metrics:                metricsServer.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				Controller:             config.Controller{SkipNameValidation: ptr.To(true), CacheSyncTimeout: cacheSyncTimeout},
			}
			watchNamespace := ""
			if tt.watchNamespace {
				watchNamespace = namespace
			}
			configureCache(&options, watchNamespace, "default", tt.secretMetadataCache)

			var fullSecretWatches, metadataSecretWatches atomic.Int64
			managerCfg := rest.CopyConfig(cfg)
			managerCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
				return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if strings.HasSuffix(req.URL.Path, "/secrets") && req.URL.Query().Get("watch") == "true" {
						if strings.Contains(req.Header.Get("Accept"), "PartialObjectMetadata") {
							metadataSecretWatches.Add(1)
						} else {
							fullSecretWatches.Add(1)
						}
					}
					return rt.RoundTrip(req)
				})
			}
			mgr, err := ctrl.NewManager(managerCfg, options)
			require.NoError(t, err)
			require.NoError(t, controller.AddToManager(ctx, mgr))
			// The webhook creates its certificate Secret before the manager
			// starts.
			beforeStart := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "created-before-start", Namespace: namespace},
				Data:       map[string][]byte{"tls.crt": []byte("cert")},
			}
			require.NoError(t, mgr.GetClient().Create(ctx, beforeStart))

			managerCtx, stop := context.WithCancel(ctx)
			stopped := make(chan error, 1)
			started := time.Now()
			go func() { stopped <- mgr.Start(managerCtx) }()
			defer func() {
				stop()
				assert.NoError(t, <-stopped, "the manager must stop cleanly")
			}()

			cr := &pxcv1.PerconaXtraDBCluster{}
			require.NoError(t, yaml.Unmarshal(crYAML, cr))
			cr.Namespace = namespace
			cr.Spec.InitImage = "perconalab/percona-xtradb-cluster-operator:main"
			cr.Spec.PXC.AutoRecovery = ptr.To(false)
			cr.Spec.Unsafe.TLS = true
			cr.Spec.TLS.Enabled = ptr.To(false)
			require.NoError(t, k8sClient.Create(ctx, cr))

			for name, obj := range map[string]client.Object{
				cr.Name + "-secrets":  &corev1.Secret{},
				"internal-" + cr.Name: &corev1.Secret{},
				cr.Name + "-pxc":      &appsv1.StatefulSet{},
			} {
				require.Eventually(t, func() bool {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj) == nil
				}, 30*time.Second, 200*time.Millisecond, "the cluster controller must reconcile and create %s %s", name, reflect.TypeOf(obj))
			}

			secret := &corev1.Secret{}
			require.NoError(t, mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(beforeStart), secret))
			assert.Equal(t, beforeStart.Data, secret.Data)

			select {
			case err := <-stopped:
				stopped <- err
				require.Failf(t, "the manager stopped", "every controller's caches must sync: %v", err)
			// Controllers start once the caches have synced, and their own sync
			// timeout runs from then.
			case <-time.After(time.Until(started.Add(2 * cacheSyncTimeout))):
			}

			assert.Equal(t, tt.fullSecretWatched, fullSecretWatches.Load() > 0, "full Secret watches: %d", fullSecretWatches.Load())
			assert.Equal(t, tt.secretMetadataCache, metadataSecretWatches.Load() > 0, "Secret metadata watches: %d", metadataSecretWatches.Load())
		})
	}
}
