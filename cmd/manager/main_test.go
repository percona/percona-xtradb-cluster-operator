package main

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
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

func TestConfigureCacheExcludedSecretTypes(t *testing.T) {
	t.Run("unset caches every Secret", func(t *testing.T) {
		t.Setenv("PXCO_SECRET_CACHE_EXCLUDED_TYPES", "")
		envs := parseEnvConfig(t)
		assert.Empty(t, envs.SecretCacheExcludedTypes)

		options := ctrl.Options{}
		require.NoError(t, configureCache(&options, "", "", envs.SecretCacheExcludedTypes))
		assert.Empty(t, options.Cache.ByObject)
		assert.Nil(t, options.NewCache)
	})

	t.Run("whitespace caches every Secret", func(t *testing.T) {
		options := ctrl.Options{}
		require.NoError(t, configureCache(&options, "", "", " "))
		assert.Empty(t, options.Cache.ByObject)
		assert.Nil(t, options.NewCache)
	})

	t.Run("excludes Helm release Secrets", func(t *testing.T) {
		t.Setenv("PXCO_SECRET_CACHE_EXCLUDED_TYPES", "helm.sh/release.v1")
		envs := parseEnvConfig(t)

		options := ctrl.Options{}
		require.NoError(t, configureCache(&options, "", "", envs.SecretCacheExcludedTypes))
		require.Len(t, options.Cache.ByObject, 1)
		for _, byObject := range options.Cache.ByObject {
			assert.Equal(t, "type!=helm.sh/release.v1", byObject.Field.String())
		}
		assert.NotNil(t, options.NewCache)
	})

	t.Run("several types, spaces trimmed", func(t *testing.T) {
		options := ctrl.Options{}
		require.NoError(t, configureCache(&options, "", "", " helm.sh/release.v1, kubernetes.io/dockerconfigjson "))
		require.Len(t, options.Cache.ByObject, 1)
		for _, byObject := range options.Cache.ByObject {
			assert.True(t, byObject.Field.Matches(fields.Set{"type": string(corev1.SecretTypeOpaque)}))
			assert.False(t, byObject.Field.Matches(fields.Set{"type": "helm.sh/release.v1"}))
			assert.False(t, byObject.Field.Matches(fields.Set{"type": string(corev1.SecretTypeDockerConfigJson)}))
		}
	})

	for name, value := range map[string]string{
		"empty entry":     "helm.sh/release.v1,",
		"inner space":     "helm.sh/release v1",
		"inner newline":   "helm.sh/release.v1\nkubernetes.io/dockerconfigjson",
		"selector syntax": "type!=helm.sh/release.v1",
		"Opaque":          "Opaque",
		"TLS":             "kubernetes.io/tls",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			options := ctrl.Options{}
			err := configureCache(&options, "", "", value)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PXCO_SECRET_CACHE_EXCLUDED_TYPES")
		})
	}
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
	opaqueSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1-ssl", Namespace: namespace},
		Type:       corev1.SecretTypeOpaque,
	}
	helmReleaseSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1.app.v1", Namespace: namespace},
		Type:       "helm.sh/release.v1",
	}
	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1-secrets", Namespace: namespace},
		Type:       corev1.SecretTypeBasicAuth,
		StringData: map[string]string{corev1.BasicAuthUsernameKey: "root"},
	}
	unwatchedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "unwatched", Namespace: otherNamespace},
		Type:       corev1.SecretTypeOpaque,
	}
	for _, secret := range []*corev1.Secret{opaqueSecret, helmReleaseSecret, userSecret, unwatchedSecret} {
		require.NoError(t, k8sClient.Create(ctx, secret))
	}

	tests := map[string]struct {
		excludedTypes      string
		watchNamespace     string
		expectedNamespaces []string
		expectedSecrets    []string
		helmCached         bool
		userSecretExcluded bool
		unwatchedCached    bool
	}{
		"cluster-wide": {
			excludedTypes:   "helm.sh/release.v1",
			expectedSecrets: []string{opaqueSecret.Name, userSecret.Name},
			unwatchedCached: true,
		},
		"watch namespaces": {
			excludedTypes:      "helm.sh/release.v1",
			watchNamespace:     namespace,
			expectedNamespaces: []string{namespace, operatorNamespace},
			expectedSecrets:    []string{opaqueSecret.Name, userSecret.Name},
		},
		"referenced Secret of an excluded type": {
			excludedTypes:      "helm.sh/release.v1,kubernetes.io/basic-auth",
			expectedSecrets:    []string{opaqueSecret.Name},
			userSecretExcluded: true,
			unwatchedCached:    true,
		},
		"nothing excluded": {
			expectedSecrets: []string{opaqueSecret.Name, helmReleaseSecret.Name, userSecret.Name},
			helmCached:      true,
			unwatchedCached: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PXCO_SECRET_CACHE_EXCLUDED_TYPES", tt.excludedTypes)
			envs := parseEnvConfig(t)

			options := ctrl.Options{
				Scheme:                 scheme,
				Metrics:                metricsServer.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				Controller:             config.Controller{SkipNameValidation: ptr.To(true)},
			}
			require.NoError(t, configureCache(&options, tt.watchNamespace, operatorNamespace, envs.SecretCacheExcludedTypes))

			var namespaces []string
			for ns := range options.Cache.DefaultNamespaces {
				namespaces = append(namespaces, ns)
			}
			assert.ElementsMatch(t, tt.expectedNamespaces, namespaces)

			var secretRequests atomic.Int64
			managerCfg := rest.CopyConfig(cfg)
			managerCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
				return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if path := req.URL.Path; strings.HasSuffix(path, "/secrets") || strings.Contains(path, "/secrets/") {
						secretRequests.Add(1)
					}
					return rt.RoundTrip(req)
				})
			}

			mgr, err := ctrl.NewManager(managerCfg, options)
			require.NoError(t, err, "the manager must build with these options, as in main")
			require.NoError(t, ctrl.NewControllerManagedBy(mgr).
				Named("secret-watch").
				For(&corev1.Secret{}).
				Complete(reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
					return reconcile.Result{}, nil
				})))
			informerCache := mgr.GetCache()

			cacheCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			stopped := make(chan struct{})
			var startErr error
			go func() {
				startErr = mgr.Start(cacheCtx)
				close(stopped)
			}()
			defer func() {
				cancel()
				<-stopped
				assert.NoError(t, startErr, "the manager must stop cleanly")
			}()

			_, err = informerCache.GetInformer(cacheCtx, &corev1.Secret{})
			require.NoError(t, err)
			if !informerCache.WaitForCacheSync(cacheCtx) {
				select {
				case <-stopped:
					t.Fatalf("cache stopped before syncing: %v", startErr)
				default:
					t.Fatal("Secret informer did not sync")
				}
			}
			secretRequests.Store(0)

			secrets := &corev1.SecretList{}
			require.NoError(t, informerCache.List(cacheCtx, secrets, client.InNamespace(namespace)))
			var names []string
			for _, secret := range secrets.Items {
				names = append(names, secret.Name)
			}
			assert.ElementsMatch(t, tt.expectedSecrets, names)

			err = informerCache.Get(cacheCtx, client.ObjectKeyFromObject(helmReleaseSecret), &corev1.Secret{})
			if tt.helmCached {
				assert.NoError(t, err)
			} else {
				require.Error(t, err, "Helm release Secret must not be cached")
				assert.False(t, k8serrors.IsNotFound(err), "an excluded Secret must not look missing")
			}

			err = informerCache.Get(cacheCtx, client.ObjectKeyFromObject(unwatchedSecret), &corev1.Secret{})
			if tt.unwatchedCached {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err, "a Secret outside WATCH_NAMESPACE must not be cached")
			}

			operatorClient := mgr.GetClient()
			err = operatorClient.Get(cacheCtx, client.ObjectKeyFromObject(userSecret), &corev1.Secret{})
			if tt.userSecretExcluded {
				require.Error(t, err)
				assert.False(t, k8serrors.IsNotFound(err), "an excluded referenced Secret must not look missing")
				assert.Contains(t, err.Error(), "PXCO_SECRET_CACHE_EXCLUDED_TYPES")
			} else {
				assert.NoError(t, err)
			}

			for _, missing := range []client.ObjectKey{
				{Namespace: namespace, Name: "missing"},
				{Namespace: namespace},
			} {
				err = operatorClient.Get(cacheCtx, missing, &corev1.Secret{})
				assert.True(t, k8serrors.IsNotFound(err), "a missing Secret %q must still be NotFound, got %v", missing, err)
			}
			assert.Zero(t, secretRequests.Load(), "after the sync, Secret reads must be served from the cache: no new list, get or watch")
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
