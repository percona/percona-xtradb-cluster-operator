package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	metricsServer "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestConfigureGroupKindConcurrency(t *testing.T) {
	tests := map[string]struct {
		envValue      string
		expectedError string
		expectedVal   map[string]int
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
		"invalid non-integer value": {
			envValue:      "invalid",
			expectedError: "valid integer",
		},
		"zero value rejected": {
			envValue:      "0",
			expectedError: "positive number",
		},
		"negative value rejected": {
			envValue:      "-1",
			expectedError: "positive number",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("MAX_CONCURRENT_RECONCILES", tt.envValue)
			}

			options := ctrl.Options{
				Scheme: scheme,
				Metrics: metricsServer.Options{
					BindAddress: "bind-address",
				},
				HealthProbeBindAddress: "probe-address",
				LeaderElection:         true,
				LeaderElectionID:       "election-id",
			}

			err := configureGroupKindConcurrency(&options)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVal, options.Controller.GroupKindConcurrency)

				// ensure that the original options are not affected
				assert.Equal(t, scheme, options.Scheme)
				assert.Equal(t, metricsServer.Options{
					BindAddress: "bind-address",
				}, options.Metrics)
				assert.Equal(t, "probe-address", options.HealthProbeBindAddress)
				assert.Equal(t, "election-id", options.LeaderElectionID)
				assert.True(t, options.LeaderElection)
			}
		})
	}
}
