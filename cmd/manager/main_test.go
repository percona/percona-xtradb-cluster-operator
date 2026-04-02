package main

import (
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"

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
