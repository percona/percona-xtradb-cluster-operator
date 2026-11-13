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

// TestCurrentMySQLVersion covers the version resolution helper used to gate the
// password_history / password_reuse_interval / password_require_current
// overrides, as well as monitor-user privilege grants.
func TestCurrentMySQLVersion(t *testing.T) {
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

			ver, err := r.currentMySQLVersion(context.Background(), cr)

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

func TestMysqlSupportsPasswordPolicyExemptions(t *testing.T) {
	tests := []struct {
		name      string
		statusVer string
		want      bool
	}{
		{name: "mysql 8.0.0 supports password policy exemptions", statusVer: "8.0.0", want: true},
		{name: "mysql 8.4.0 supports password policy exemptions", statusVer: "8.4.0", want: true},
		{name: "mysql 5.7 does not support password policy exemptions", statusVer: "5.7.33", want: false},
		{name: "unknown version does not support password policy exemptions", statusVer: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newCR("cr-mock", "pxc")
			cr.Status.PXC.Version = tt.statusVer

			r := buildFakeClient([]runtime.Object{cr})

			got, err := r.mysqlSupportsPasswordPolicyExemptions(context.Background(), cr)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExemptUserFromPasswordPolicies_SkipPaths covers the cases where the
// override must be skipped without ever reaching for a user manager / DB
// connection: the annotation already applied (for both the MySQL 8.0+ full
// exemption and the legacy MySQL 5.7 expire-only exemption, which are tracked
// under distinct annotation keys), and the PXC version not yet known/ready.
func TestExemptUserFromPasswordPolicies_SkipPaths(t *testing.T) {
	user := &users.SysUser{Name: "operator", Hosts: []string{"%"}}

	t.Run("mysql 8.0+ full-exemption annotation already done: no-op", func(t *testing.T) {
		cr := newCR("cr-mock", "pxc")
		cr.Status.PXC.Version = "8.0.20"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "internal-" + cr.Name,
				Namespace: cr.Namespace,
				Annotations: map[string]string{
					"password-policy-for-1.21.0-user-operator": "done",
				},
			},
		}

		r := buildFakeClient([]runtime.Object{cr, secret})

		err := r.exemptUserFromPasswordPolicies(context.Background(), cr, secret, user)
		require.NoError(t, err)
	})

	t.Run("mysql 5.7 legacy expire-only annotation already done: no-op", func(t *testing.T) {
		cr := newCR("cr-mock", "pxc")
		cr.Status.PXC.Version = "5.7.33"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "internal-" + cr.Name,
				Namespace: cr.Namespace,
				Annotations: map[string]string{
					"pass-expire-policy-for-1.13.0-user-operator": "done",
				},
			},
		}

		r := buildFakeClient([]runtime.Object{cr, secret})

		err := r.exemptUserFromPasswordPolicies(context.Background(), cr, secret, user)
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

		err := r.exemptUserFromPasswordPolicies(context.Background(), cr, secret, user)
		require.NoError(t, err)
		assert.Empty(t, secret.Annotations)
	})
}

// TestExemptUserFromPasswordPolicies_UpgradeFromMySQL57To80 is a regression
// test for a bug where the legacy MySQL 5.7 expire-only annotation and the
// MySQL 8.0+ full-exemption annotation shared a single key. A cluster
// upgraded from 5.7 (where only PASSWORD EXPIRE NEVER was applied and
// recorded as "done") would then have the 8.0+ password_history/reuse_interval/
// require_current exemptions silently skipped forever, since the shared
// annotation was already marked "done" under 5.7.
//
// With distinct annotation keys per MySQL capability, only the legacy
// annotation is "done" here, so the function must NOT short-circuit: it must
// proceed past the skip check and attempt to (re-)apply the full exemption,
// which surfaces as a connection error in this test (there's no real MySQL
// server behind the configured address) rather than a silent no-op.
func TestExemptUserFromPasswordPolicies_UpgradeFromMySQL57To80(t *testing.T) {
	user := &users.SysUser{Name: "operator", Hosts: []string{"%"}}

	cr := newCR("cr-mock", "pxc")
	cr.Status.PXC.Version = "8.0.20" // cluster has since been upgraded to 8.0+
	cr.Spec.PXC.ReadinessProbes.TimeoutSeconds = 1
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal-" + cr.Name,
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				// only the legacy 5.7 annotation is marked done; the 8.0+
				// annotation ("password-policy-for-1.21.0-user-operator") is not.
				"pass-expire-policy-for-1.13.0-user-operator": "done",
			},
		},
		Data: map[string][]byte{
			users.Root: []byte("rootpass"),
		},
	}

	r := buildFakeClient([]runtime.Object{cr, secret})

	err := r.exemptUserFromPasswordPolicies(context.Background(), cr, secret, user)
	// The legacy annotation must not gate the 8.0+ exemption: the function
	// should attempt the real exemption (and fail to connect, since there's
	// no live MySQL server here), instead of silently returning nil.
	require.Error(t, err)
	assert.NotEqual(t, "done", secret.Annotations["password-policy-for-1.21.0-user-operator"])
}

// TestUpdateMySQLInitFile_PasswordPolicyExemptClause verifies that the generated
// mysql-init secret only appends the PASSWORD HISTORY / REUSE INTERVAL / REQUIRE
// CURRENT exemption clause when the cluster's MySQL version supports it (8.0+),
// since the init file is re-applied on every mysqld startup and must not trip
// password policy checks on older servers that don't understand the clause.
func TestUpdateMySQLInitFile_PasswordPolicyExemptClause(t *testing.T) {
	tests := []struct {
		name             string
		statusVer        string
		wantExemptClause bool
	}{
		{name: "mysql 8.0+: exempt clause appended", statusVer: "8.0.20", wantExemptClause: true},
		{name: "mysql 5.7: exempt clause omitted", statusVer: "5.7.33", wantExemptClause: false},
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

			if tt.wantExemptClause {
				assert.Contains(t, initSQL, "PASSWORD HISTORY 0 PASSWORD REUSE INTERVAL 0 DAY PASSWORD REQUIRE CURRENT OPTIONAL")
			} else {
				assert.NotContains(t, initSQL, "PASSWORD HISTORY")
				assert.NotContains(t, initSQL, "PASSWORD REQUIRE CURRENT")
			}
		})
	}
}
