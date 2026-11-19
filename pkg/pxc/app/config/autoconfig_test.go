package config

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func quantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func validRequest() AutoConfigRequest {
	return AutoConfigRequest{
		Version:  "8.4.7-7.1",
		CPU:      quantity("2"),
		Memory:   quantity("4Gi"),
		LoadType: api.AutoConfigLoadTypeSomeWrites,
	}
}

func TestAutoConfigParams(t *testing.T) {
	tests := map[string]struct {
		modify     func(req *AutoConfigRequest)
		check      func(t *testing.T, got MySQLdConfig)
		wantErr    error
		wantErrMsg string
	}{
		"calculates innodb and galera options": {
			check: func(t *testing.T, got MySQLdConfig) {
				for _, name := range []string{
					"innodb_buffer_pool_size", "innodb_redo_log_capacity", "max_connections",
					"wsrep_provider_options", "wsrep_slave_threads", "wsrep_sync_wait",
				} {
					assert.Contains(t, got, name)
				}
				assert.Contains(t, got["wsrep_provider_options"], "gcache.size=")
			},
		},
		"skips calculated groups that are not mysqld options": {
			check: func(t *testing.T, got MySQLdConfig) {
				for _, name := range []string{"cpu", "memory", "timeoutSeconds"} {
					assert.NotContains(t, got, name)
				}
			},
		},
		"leaves options the user configured": {
			modify: func(req *AutoConfigRequest) {
				req.User = MySQLdConfig{"max_connections": "10", "loose_wsrep_provider_options": "pc.weight=10"}
			},
			check: func(t *testing.T, got MySQLdConfig) {
				assert.NotContains(t, got, "max_connections")
				assert.NotContains(t, got, "wsrep_provider_options")
				assert.Contains(t, got, "innodb_buffer_pool_size")
			},
		},
		"load type changes the result": {
			modify: func(req *AutoConfigRequest) { req.LoadType = api.AutoConfigLoadTypeMostlyReads },
			check: func(t *testing.T, got MySQLdConfig) {
				assert.Equal(t, "0", got["wsrep_sync_wait"])
			},
		},
		"volume large enough": {
			modify: func(req *AutoConfigRequest) { req.Storage = 100 << 30 },
			check: func(t *testing.T, got MySQLdConfig) {
				assert.Contains(t, got, "innodb_redo_log_capacity")
			},
		},
		"volume too small": {
			modify:  func(req *AutoConfigRequest) { req.Storage = 1 << 30 },
			wantErr: ErrInsufficientStorage,
		},
		"no cpu": {
			modify:     func(req *AutoConfigRequest) { req.CPU = nil },
			wantErrMsg: "cpu is required for autoconfig",
		},
		"zero cpu": {
			modify:     func(req *AutoConfigRequest) { req.CPU = quantity("0") },
			wantErrMsg: "cpu is required for autoconfig",
		},
		"no memory": {
			modify:     func(req *AutoConfigRequest) { req.Memory = nil },
			wantErrMsg: "memory is required for autoconfig",
		},
		"version below the supported range": {
			modify:  func(req *AutoConfigRequest) { req.Version = "8.0.32-24.2" },
			wantErr: ErrVersionUnsupported,
		},
		"version above the supported range": {
			modify:  func(req *AutoConfigRequest) { req.Version = "12.0.0" },
			wantErr: ErrVersionUnsupported,
		},
		"no version": {
			modify:     func(req *AutoConfigRequest) { req.Version = "" },
			wantErrMsg: `parse mysql version ""`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req := validRequest()
			if tc.modify != nil {
				tc.modify(&req)
			}

			got, err := AutoConfigParams(req)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, got)
				return
			case tc.wantErrMsg != "":
				require.ErrorContains(t, err, tc.wantErrMsg)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestAutoConfigParamsIsStable(t *testing.T) {
	a, err := AutoConfigParams(validRequest())
	require.NoError(t, err)
	b, err := AutoConfigParams(validRequest())
	require.NoError(t, err)
	assert.Equal(t, a.String(), b.String())
}

func TestCheckPreallocatedFits(t *testing.T) {
	tests := map[string]struct {
		storage    int64
		params     MySQLdConfig
		wantErr    error
		wantErrMsg string
	}{
		"no volume bound": {
			params: MySQLdConfig{"innodb_redo_log_capacity": strconv.Itoa(1 << 30)},
		},
		"redo log capacity fits": {
			storage: 4 << 30,
			params:  MySQLdConfig{"innodb_redo_log_capacity": strconv.Itoa(1 << 30)},
		},
		"redo log and gcache overfill": {
			storage: 4 << 30,
			params: MySQLdConfig{
				"innodb_redo_log_capacity": strconv.Itoa(1 << 30),
				"wsrep_provider_options":   "gcache.size=1;",
			},
			wantErr: ErrInsufficientStorage,
		},
		"pre-8.0.30 log files": {
			storage: 4 << 30,
			params: MySQLdConfig{
				"innodb_log_file_size":      strconv.Itoa(512 << 20),
				"innodb_log_files_in_group": "3",
			},
			wantErr: ErrInsufficientStorage,
		},
		"single log file by default": {
			storage: 4 << 30,
			params:  MySQLdConfig{"innodb_log_file_size": strconv.Itoa(1 << 30)},
		},
		"malformed redo log capacity": {
			storage:    4 << 30,
			params:     MySQLdConfig{"innodb_redo_log_capacity": "1G"},
			wantErrMsg: `parse innodb_redo_log_capacity value "1G"`,
		},
		"malformed log files": {
			storage:    4 << 30,
			params:     MySQLdConfig{"innodb_log_file_size": "1", "innodb_log_files_in_group": "two"},
			wantErrMsg: `parse innodb_log_files_in_group value "two"`,
		},
		"malformed gcache size": {
			storage:    4 << 30,
			params:     MySQLdConfig{"wsrep_provider_options": "gcache.size=1G"},
			wantErrMsg: `parse gcache.size value "1G"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkPreallocatedFits(tc.storage, tc.params)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantErrMsg != "":
				require.ErrorContains(t, err, tc.wantErrMsg)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestAutoConfigVersion(t *testing.T) {
	tests := map[string]struct {
		cr   *api.PerconaXtraDBCluster
		want string
	}{
		"image tag wins over the recorded version": {
			cr: &api.PerconaXtraDBCluster{
				Spec:   api.PerconaXtraDBClusterSpec{PXC: &api.PXCSpec{PodSpec: &api.PodSpec{Image: "percona/percona-xtradb-cluster:8.4.7-7.1"}}},
				Status: api.PerconaXtraDBClusterStatus{PXC: api.AppStatus{ComponentStatus: api.ComponentStatus{Version: "8.4.6-6.1"}}},
			},
			want: "8.4.7",
		},
		"recorded version for a tag without one": {
			cr: &api.PerconaXtraDBCluster{
				Spec:   api.PerconaXtraDBClusterSpec{PXC: &api.PXCSpec{PodSpec: &api.PodSpec{Image: "perconalab/percona-xtradb-cluster-operator:main-pxc8.4"}}},
				Status: api.PerconaXtraDBClusterStatus{PXC: api.AppStatus{ComponentStatus: api.ComponentStatus{Version: "8.4.7-7.1"}}},
			},
			want: "8.4.7-7.1",
		},
		"no version anywhere": {
			cr: &api.PerconaXtraDBCluster{
				Spec: api.PerconaXtraDBClusterSpec{PXC: &api.PXCSpec{PodSpec: &api.PodSpec{Image: "perconalab/percona-xtradb-cluster-operator:main-pxc8.4"}}},
			},
		},
		"no pxc spec": {
			cr: &api.PerconaXtraDBCluster{
				Status: api.PerconaXtraDBClusterStatus{PXC: api.AppStatus{ComponentStatus: api.ComponentStatus{Version: "8.4.7-7.1"}}},
			},
			want: "8.4.7-7.1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, AutoConfigVersion(tc.cr))
		})
	}
}

func TestEffectiveResource(t *testing.T) {
	res := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi"), corev1.ResourceCPU: resource.MustParse("500m")},
	}

	assert.Equal(t, "2Gi", EffectiveResource(res, corev1.ResourceMemory).String())
	assert.Equal(t, "500m", EffectiveResource(res, corev1.ResourceCPU).String())
	assert.Nil(t, EffectiveResource(res, corev1.ResourceStorage))
}

func TestCalculatorLoadType(t *testing.T) {
	seen := make(map[int]api.AutoConfigLoadType)
	for _, lt := range []api.AutoConfigLoadType{
		api.AutoConfigLoadTypeMostlyReads, api.AutoConfigLoadTypeSomeWrites,
		api.AutoConfigLoadTypeEqualReadsWrites, api.AutoConfigLoadTypeHeavyWrites,
	} {
		id := calculatorLoadType(lt)
		assert.NotContains(t, seen, id, "%s maps to the same calculator load as %s", lt, seen[id])
		seen[id] = lt
	}
	assert.Equal(t, calculatorLoadType(api.AutoConfigLoadTypeSomeWrites), calculatorLoadType(""))
}
