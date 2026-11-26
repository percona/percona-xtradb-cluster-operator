package pxc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

// reconcilerFailingSecretGets returns a reconciler whose first `failures`
// Secret reads fail with a server error, as they can when Secrets are read
// from the API server.
func reconcilerFailingSecretGets(t *testing.T, failures int, objs ...client.Object) *ReconcilePerconaXtraDBCluster {
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apis.AddToScheme(s))
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&api.PerconaXtraDBCluster{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Secret); ok && failures > 0 {
					failures--
					return k8serrors.NewServiceUnavailable("unavailable")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
	return &ReconcilePerconaXtraDBCluster{
		client:        cl,
		scheme:        cl.Scheme(),
		lockers:       newLockStore(),
		serverVersion: &version.ServerVersion{Platform: version.PlatformKubernetes},
	}
}

func TestDeleteCertsReturnsSecretReadErrors(t *testing.T) {
	cr := &api.PerconaXtraDBCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Namespace: "pxc"}}
	tlsSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cluster1-ssl", Namespace: "pxc"}}
	r := reconcilerFailingSecretGets(t, 1, tlsSecret)

	err := r.deleteCerts(t.Context(), cr)

	require.Error(t, err, "a Secret that could not be read must not be skipped")
	assert.True(t, k8serrors.IsServiceUnavailable(err))
	err = r.client.Get(t.Context(), client.ObjectKeyFromObject(tlsSecret), &corev1.Secret{})
	assert.True(t, k8serrors.IsNotFound(err), "the Secrets that could be read must be deleted, got %v", err)
}

func TestDeleteSSLFinalizerKeptOnSecretReadErrors(t *testing.T) {
	cr, err := readDefaultCR("cluster1", "pxc")
	require.NoError(t, err)
	cr.Finalizers = []string{naming.FinalizerDeleteSSL}
	cr.DeletionTimestamp = new(metav1.Now())
	tlsSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cr.Name + "-ssl", Namespace: cr.Namespace}}
	r := reconcilerFailingSecretGets(t, 1, cr, tlsSecret)

	_, err = r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

	require.NoError(t, err)
	stored := &api.PerconaXtraDBCluster{}
	require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), stored))
	assert.Contains(t, stored.Finalizers, naming.FinalizerDeleteSSL, "the finalizer must stay until the TLS Secrets are deleted")
}

func TestDeleteSSLFinalizerRemovedAfterTimeout(t *testing.T) {
	cr, err := readDefaultCR("cluster1", "pxc")
	require.NoError(t, err)
	cr.Finalizers = []string{naming.FinalizerDeleteSSL}
	cr.UID = "cluster1-uid"
	cr.DeletionTimestamp = new(metav1.NewTime(time.Now().Add(-deleteCertsTimeout - time.Minute)))
	tlsSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cr.Name + "-ssl", Namespace: cr.Namespace}}
	r := reconcilerFailingSecretGets(t, 10, cr, tlsSecret)
	recorder := record.NewFakeRecorder(1)
	r.recorder = recorder

	_, err = r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

	require.NoError(t, err)
	stored := &api.PerconaXtraDBCluster{}
	require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), stored))
	assert.Contains(t, stored.Finalizers, naming.FinalizerDeleteSSL, "the wait must count from the first failure, not from the deletion")
	assert.Empty(t, recorder.Events)

	since, ok := r.tlsSecretsUnreadableSince.Load(cr.UID)
	require.True(t, ok, "the wait must be kept while the Secrets stay unreadable")
	r.tlsSecretsUnreadableSince.Store(cr.UID, since.(time.Time).Add(-deleteCertsTimeout))

	_, err = r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

	require.NoError(t, err)
	err = r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), &api.PerconaXtraDBCluster{})
	assert.True(t, k8serrors.IsNotFound(err), "the cluster must be deleted once the timeout passed, got %v", err)
	require.Len(t, recorder.Events, 1)
	assert.Contains(t, <-recorder.Events, naming.EventTLSSecretsNotDeleted)
}

func TestDeleteSSLFinalizerKeptAfterTimeoutOnDeleteErrors(t *testing.T) {
	cr, err := readDefaultCR("cluster1", "pxc")
	require.NoError(t, err)
	cr.Finalizers = []string{naming.FinalizerDeleteSSL}
	cr.UID = "cluster1-uid"
	cr.DeletionTimestamp = new(metav1.NewTime(time.Now().Add(-deleteCertsTimeout - time.Minute)))
	tlsSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cr.Name + "-ssl", Namespace: cr.Namespace}}
	r := reconcilerFailingSecretGets(t, 0, cr, tlsSecret)
	r.recorder = record.NewFakeRecorder(1)
	r.tlsSecretsUnreadableSince.Store(cr.UID, time.Now().Add(-deleteCertsTimeout-time.Minute))
	r.client = interceptor.NewClient(r.client.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Secret); ok && key.Name == naming.CACertificateName(cr) {
				return k8serrors.NewServiceUnavailable("unavailable")
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*corev1.Secret); ok {
				return k8serrors.NewForbidden(corev1.Resource("secrets"), obj.GetName(), nil)
			}
			return c.Delete(ctx, obj, opts...)
		},
	})

	_, err = r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

	require.NoError(t, err)
	stored := &api.PerconaXtraDBCluster{}
	require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), stored))
	assert.Contains(t, stored.Finalizers, naming.FinalizerDeleteSSL, "only Secrets that cannot be read may be left behind")
}

func TestDeleteSSLFinalizerWaitKeptUntilFinalizerRemoved(t *testing.T) {
	cr, err := readDefaultCR("cluster1", "pxc")
	require.NoError(t, err)
	cr.Finalizers = []string{naming.FinalizerDeleteSSL}
	cr.UID = "cluster1-uid"
	cr.DeletionTimestamp = new(metav1.NewTime(time.Now()))
	tlsSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cr.Name + "-ssl", Namespace: cr.Namespace}}
	r := reconcilerFailingSecretGets(t, 100, cr, tlsSecret)
	r.recorder = record.NewFakeRecorder(2)
	since := time.Now().Add(-deleteCertsTimeout - time.Minute)
	r.tlsSecretsUnreadableSince.Store(cr.UID, since)
	updateFails := true
	r.client = interceptor.NewClient(r.client.(client.WithWatch), interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*api.PerconaXtraDBCluster); ok && updateFails {
				return k8serrors.NewServiceUnavailable("unavailable")
			}
			return c.Update(ctx, obj, opts...)
		},
	})

	_, err = r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

	require.Error(t, err)
	stored, ok := r.tlsSecretsUnreadableSince.Load(cr.UID)
	require.True(t, ok, "the wait must be kept while the finalizer is still set")
	assert.Equal(t, since, stored)

	updateFails = false
	_, err = r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

	require.NoError(t, err)
	_, ok = r.tlsSecretsUnreadableSince.Load(cr.UID)
	assert.False(t, ok, "the wait must be cleared once the finalizer is removed")
}

func TestDeleteSSLFinalizerWaitRestartsAfterOtherFailures(t *testing.T) {
	cr, err := readDefaultCR("cluster1", "pxc")
	require.NoError(t, err)
	cr.Finalizers = []string{naming.FinalizerDeleteSSL}
	cr.UID = "cluster1-uid"
	cr.DeletionTimestamp = new(metav1.NewTime(time.Now()))
	tlsSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cr.Name + "-ssl", Namespace: cr.Namespace}}
	r := reconcilerFailingSecretGets(t, 0, cr, tlsSecret)
	r.recorder = record.NewFakeRecorder(1)
	readsFail, deletesFail := true, false
	r.client = interceptor.NewClient(r.client.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Secret); ok && readsFail {
				return k8serrors.NewServiceUnavailable("unavailable")
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if _, ok := obj.(*corev1.Secret); ok && deletesFail {
				return k8serrors.NewConflict(corev1.Resource("secrets"), obj.GetName(), nil)
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	reconcileAndExpectFinalizer := func(msg string) {
		t.Helper()
		_, err := r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})
		require.NoError(t, err)
		stored := &api.PerconaXtraDBCluster{}
		require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), stored))
		require.Contains(t, stored.Finalizers, naming.FinalizerDeleteSSL, msg)
	}

	reconcileAndExpectFinalizer("the first read failure must not remove the finalizer")
	since, ok := r.tlsSecretsUnreadableSince.Load(cr.UID)
	require.True(t, ok)
	r.tlsSecretsUnreadableSince.Store(cr.UID, since.(time.Time).Add(-deleteCertsTimeout-time.Minute))

	readsFail, deletesFail = false, true
	reconcileAndExpectFinalizer("a failed delete must keep the finalizer")

	readsFail, deletesFail = true, false
	reconcileAndExpectFinalizer("the wait must start over after the Secrets could be read")
}

func TestPasswordChangeChecksMySQLInitSecretFirst(t *testing.T) {
	cr, err := readDefaultCR("cluster1", "pxc")
	require.NoError(t, err)
	cr.Status.Status = api.AppStateReady
	secrets := &corev1.Secret{Data: map[string][]byte{users.Root: []byte("new")}}
	internalSecrets := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"pass-expire-policy-for-1.13.0-user-root": "done"}},
		Data:       map[string][]byte{users.Root: []byte("old")},
	}
	r := reconcilerFailingSecretGets(t, 1, mysqlInitSecretWith("ALTER USER 'monitor'@'%' IDENTIFIED BY 'p';\n"))

	err = r.handleRootUserWithoutDP(t.Context(), cr, secrets, internalSecrets)

	require.Error(t, err)
	assert.True(t, k8serrors.IsServiceUnavailable(err), "the init Secret must be checked before the password is changed, got %v", err)
}

func TestCheckMySQLInitSecretReturnsSecretReadErrors(t *testing.T) {
	cr := &api.PerconaXtraDBCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Namespace: "pxc"}}
	r := reconcilerFailingSecretGets(t, 1, mysqlInitSecretWith("ALTER USER 'monitor'@'%' IDENTIFIED BY 'p';\n"))

	err := r.checkMySQLInitSecret(t.Context(), cr)

	require.Error(t, err, "the password must not be changed while the init file cannot be updated")
	assert.True(t, k8serrors.IsServiceUnavailable(err))
	assert.NoError(t, r.checkMySQLInitSecret(t.Context(), &api.PerconaXtraDBCluster{ObjectMeta: metav1.ObjectMeta{Name: "missing", Namespace: "pxc"}}))
}

func TestUpdateMySQLInitFile(t *testing.T) {
	cr := &api.PerconaXtraDBCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Namespace: "pxc"}}
	root := &users.SysUser{Name: "root", Pass: "p", Hosts: []string{"%"}}

	t.Run("appends to the Secret", func(t *testing.T) {
		initSecret := mysqlInitSecretWith("ALTER USER 'monitor'@'%' IDENTIFIED BY 'p';\n")
		r := reconcilerFailingSecretGets(t, 0, initSecret)

		require.NoError(t, r.checkMySQLInitSecret(t.Context(), cr))
		require.NoError(t, r.updateMySQLInitFile(t.Context(), cr, root))

		stored := &corev1.Secret{}
		require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(initSecret), stored))
		assert.Equal(t, "ALTER USER 'monitor'@'%' IDENTIFIED BY 'p';\nALTER USER 'root'@'%' IDENTIFIED BY 'p';\n", string(stored.Data["init.sql"]))
	})

	t.Run("starts over when a backup deleted the Secret", func(t *testing.T) {
		initSecret := mysqlInitSecretWith("ALTER USER 'monitor'@'%' IDENTIFIED BY 'old';\n")
		r := reconcilerFailingSecretGets(t, 0, initSecret)

		require.NoError(t, r.checkMySQLInitSecret(t.Context(), cr))
		require.NoError(t, r.client.Delete(t.Context(), initSecret))
		require.NoError(t, r.updateMySQLInitFile(t.Context(), cr, root))

		stored := &corev1.Secret{}
		require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(initSecret), stored))
		assert.Equal(t, "SET SESSION wsrep_on=OFF;\nSET SESSION sql_log_bin=0;\nALTER USER 'root'@'%' IDENTIFIED BY 'p';\n", string(stored.Data["init.sql"]),
			"statements a backup discarded must not come back")
	})

	t.Run("reads the Secret once", func(t *testing.T) {
		r := reconcilerFailingSecretGets(t, 0, mysqlInitSecretWith("ALTER USER 'monitor'@'%' IDENTIFIED BY 'p';\n"))
		reads := 0
		r.client = interceptor.NewClient(r.client.(client.WithWatch), interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Secret); ok {
					reads++
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})

		require.NoError(t, r.updateMySQLInitFile(t.Context(), cr, root))

		assert.Equal(t, 1, reads, "a backup could delete the Secret between two reads, and the write would bring back what it discarded")
	})
}

func mysqlInitSecretWith(initSQL string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1-mysql-init", Namespace: "pxc"},
		Data:       map[string][]byte{"init.sql": []byte(initSQL)},
	}
}
