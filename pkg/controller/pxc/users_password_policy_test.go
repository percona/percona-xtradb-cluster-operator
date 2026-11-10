package pxc

import (
	"context"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

// TestMysqlVersionForPasswordPolicies covers the version resolution helper used
// to gate the password_history / password_reuse_interval / password_require_current
// overrides.
func TestMysqlVersionForPasswordPolicies(t *testing.T) {
	tests := []struct {
		name        string
		statusVer   string
		want        *version.Version
		expectError bool
	}{
		{
			name:      "uses status version when already known",
			statusVer: "8.0.14",
			want:      version.Must(version.NewVersion("8.0.14")),
		},
		{
			name:      "no status version and PXC not ready yet: no error, nil version",
			statusVer: "",
			want:      nil,
		},
		{
			name:        "invalid status version is an error",
			statusVer:   "not-a-version",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newCR("cr-mock", "pxc")
			cr.Status.PXC.Version = tt.statusVer
			// cr.Status.PXC.Ready stays 0, so if the status version is empty,
			// mysqlVersion() returns versionNotReadyErr without any cluster access.

			r := buildFakeClient([]runtime.Object{cr})

			ver, err := r.mysqlVersionForPasswordPolicies(context.Background(), cr)

			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, ver)
				return
			}
			require.NotNil(t, ver)
			assert.True(t, ver.Equal(tt.want), "got %s, want %s", ver, tt.want)
		})
	}
}

func TestMysqlSupportsPasswordHistoryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		statusVer string
		want      bool
	}{
		{name: "mysql 8.0.0 supports password history policy", statusVer: "8.0.0", want: true},
		{name: "mysql 8.4.0 supports password history policy", statusVer: "8.4.0", want: true},
		{name: "mysql 5.7 does not support password history policy", statusVer: "5.7.33", want: false},
		{name: "unknown version does not support password history policy", statusVer: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newCR("cr-mock", "pxc")
			cr.Status.PXC.Version = tt.statusVer

			r := buildFakeClient([]runtime.Object{cr})

			got, err := r.mysqlSupportsPasswordHistoryPolicy(context.Background(), cr)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDisableUserPasswordPolicies_SkipPaths covers the cases where the
// override must be skipped without ever reaching for a user manager / DB
// connection: the annotation already applied, and the PXC version not yet
// known/ready.
func TestDisableUserPasswordPolicies_SkipPaths(t *testing.T) {
	user := &users.SysUser{Name: "operator", Hosts: []string{"%"}}

	t.Run("annotation already done: no-op", func(t *testing.T) {
		cr := newCR("cr-mock", "pxc")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "internal-" + cr.Name,
				Namespace: cr.Namespace,
				Annotations: map[string]string{
					"password-policy-for-1.20.0-user-operator": "done",
				},
			},
		}

		r := buildFakeClient([]runtime.Object{cr, secret})

		err := r.disableUserPasswordPolicies(context.Background(), cr, secret, user)
		require.NoError(t, err)
	})

	t.Run("version not ready: no-op, no secret mutation", func(t *testing.T) {
		cr := newCR("cr-mock", "pxc")
		// cr.Status.PXC.Ready == 0 and cr.Status.PXC.Version == "" => version
		// resolution returns (nil, nil) and the function must bail out before
		// ever constructing a user manager / DB connection.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "internal-" + cr.Name,
				Namespace: cr.Namespace,
			},
		}

		r := buildFakeClient([]runtime.Object{cr, secret})

		err := r.disableUserPasswordPolicies(context.Background(), cr, secret, user)
		require.NoError(t, err)
		assert.Empty(t, secret.Annotations)
	})
}

// TestUpdateMySQLInitFile_PasswordHistoryClause verifies that the generated
// mysql-init secret only appends the PASSWORD HISTORY / REUSE INTERVAL clause
// when the cluster's MySQL version supports it (8.0+), since the init file is
// re-applied on every mysqld startup and must not trip password reuse checks
// on older servers that don't understand the clause.
func TestUpdateMySQLInitFile_PasswordHistoryClause(t *testing.T) {
	tests := []struct {
		name        string
		statusVer   string
		wantHistory bool
	}{
		{name: "mysql 8.0+: history clause appended", statusVer: "8.0.20", wantHistory: true},
		{name: "mysql 5.7: history clause omitted", statusVer: "5.7.33", wantHistory: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newCR("cr-mock", "pxc")
			cr.Status.PXC.Version = tt.statusVer

			r := buildFakeClient([]runtime.Object{cr})

			user := &users.SysUser{Name: "operator", Pass: "secret-pass", Hosts: []string{"%"}}
			err := r.updateMySQLInitFile(context.Background(), cr, nil, user)
			require.NoError(t, err)

			secret := &corev1.Secret{}
			err = r.client.Get(context.Background(), client.ObjectKey{
				Name:      cr.Name + "-mysql-init",
				Namespace: cr.Namespace,
			}, secret)
			require.NoError(t, err)

			initSQL := string(secret.Data["init.sql"])
			assert.Contains(t, initSQL, "ALTER USER 'operator'@'%' IDENTIFIED BY 'secret-pass'")

			if tt.wantHistory {
				assert.Contains(t, initSQL, "PASSWORD HISTORY 0 PASSWORD REUSE INTERVAL 0 DAY")
			} else {
				assert.NotContains(t, initSQL, "PASSWORD HISTORY")
			}
		})
	}
}
