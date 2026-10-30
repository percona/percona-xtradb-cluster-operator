package main

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
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

func TestConfigureCache(t *testing.T) {
	secretSelector := func(t *testing.T, options ctrl.Options) fields.Selector {
		t.Helper()
		for obj, byObject := range options.Cache.ByObject {
			if _, isSecret := obj.(*corev1.Secret); isSecret {
				require.NotNil(t, byObject.Field)
				return byObject.Field
			}
		}
		require.Fail(t, "Secrets must have cache options")
		return nil
	}

	t.Run("excludes Helm release Secrets", func(t *testing.T) {
		options := ctrl.Options{}
		configureCache(&options, "", "operator-ns")

		assert.Empty(t, options.Cache.DefaultNamespaces)
		selector := secretSelector(t, options)
		assert.Equal(t, "type!=helm.sh/release.v1", selector.String())
		assert.False(t, selector.Matches(fields.Set{"type": "helm.sh/release.v1"}))
		assert.True(t, selector.Matches(fields.Set{"type": string(corev1.SecretTypeOpaque)}))
		assert.True(t, selector.Matches(fields.Set{"type": string(corev1.SecretTypeTLS)}))
	})

	t.Run("watch namespaces include the operator namespace", func(t *testing.T) {
		options := ctrl.Options{}
		configureCache(&options, "ns-1,ns-2", "operator-ns")

		assert.ElementsMatch(t, []string{"ns-1", "ns-2", "operator-ns"}, slices.Collect(maps.Keys(options.Cache.DefaultNamespaces)))
		assert.Equal(t, "type!=helm.sh/release.v1", secretSelector(t, options).String())
	})
}

// TestConfigureCacheInformer starts a manager with the configured cache
// against an envtest control plane and checks what the cached client sees:
// a Helm release Secret is reported as not found, every other Secret is read.
func TestConfigureCacheInformer(t *testing.T) {
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, testEnv.Stop()) })

	testScheme := k8sruntime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(testScheme))

	options := ctrl.Options{
		Scheme:                 testScheme,
		Metrics:                metricsServer.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	}
	configureCache(&options, "", "default")
	mgr, err := ctrl.NewManager(cfg, options)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	managerStopped := make(chan error, 1)
	go func() { managerStopped <- mgr.Start(ctx) }()
	// Registered after testEnv.Stop, so it runs first: the manager must be
	// down before the control plane it talks to.
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-managerStopped)
	})

	directClient, err := client.New(cfg, client.Options{Scheme: testScheme})
	require.NoError(t, err)
	newSecret := func(name string, secretType corev1.SecretType) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Type:       secretType,
			StringData: map[string]string{"key": "value"},
		}
	}
	cachedGet := func(secret *corev1.Secret) error {
		return mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(secret), &corev1.Secret{})
	}
	waitUntilCached := func(secret *corev1.Secret) {
		t.Helper()
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.NoError(c, cachedGet(secret))
		}, 30*time.Second, 100*time.Millisecond, "the cache must serve Secrets of other types")
	}
	assertNotCached := func(secret *corev1.Secret) {
		t.Helper()
		err := cachedGet(secret)
		assert.True(t, k8serrors.IsNotFound(err), "the cache must not hold Helm release Secrets, got %v", err)
		assert.NoError(t, mgr.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(secret), &corev1.Secret{}),
			"the Helm release Secret must exist in the API server")
	}

	// Both Secrets exist before the first cached read, so the informer's
	// initial list has to filter.
	helmSecret := newSecret("sh.helm.release.v1.cluster1.v1", helmReleaseSecretType)
	usersSecret := newSecret("cluster1-secrets", corev1.SecretTypeOpaque)
	require.NoError(t, directClient.Create(ctx, helmSecret))
	require.NoError(t, directClient.Create(ctx, usersSecret))
	waitUntilCached(usersSecret)
	assertNotCached(helmSecret)

	// Secrets created while the informer runs arrive through the watch, in
	// creation order, so once the later Opaque Secret is cached the earlier
	// Helm Secret would be too, had the watch not filtered it.
	helmSecret = newSecret("sh.helm.release.v1.cluster1.v2", helmReleaseSecretType)
	usersSecret = newSecret("cluster1-ssl", corev1.SecretTypeOpaque)
	require.NoError(t, directClient.Create(ctx, helmSecret))
	require.NoError(t, directClient.Create(ctx, usersSecret))
	waitUntilCached(usersSecret)
	assertNotCached(helmSecret)
}
