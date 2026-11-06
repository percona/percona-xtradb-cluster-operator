package pxcrestore

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

// unavailableSecretError stands in for the Secret metadata cache failing
// to read a Secret from the API server.
type unavailableSecretError struct{ error }

func (unavailableSecretError) SecretUnavailable() bool { return true }

func TestFailUnlessRetryable(t *testing.T) {
	unavailable := errors.Wrap(unavailableSecretError{errors.New("get Secret pxc/s3: unavailable")}, "get storage")
	for name, tc := range map[string]struct {
		err         error
		created     time.Time
		want        api.RestoreState
		wantComment string
	}{
		"Secret unavailable":                  {err: unavailable, created: time.Now(), want: api.RestoreStarting, wantComment: "retrying: failed to validate restore job: " + unavailable.Error()},
		"Secret unavailable past the timeout": {err: unavailable, created: time.Now().Add(-secretUnavailableRetryTimeout), want: api.RestoreFailed, wantComment: "failed to validate restore job: " + unavailable.Error()},
		"other error":                         {err: errors.New("backup not found"), created: time.Now(), want: api.RestoreFailed, wantComment: "failed to validate restore job: backup not found"},
	} {
		t.Run(name, func(t *testing.T) {
			cr := &api.PerconaXtraDBClusterRestore{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(tc.created)},
				Status:     api.PerconaXtraDBClusterRestoreStatus{State: api.RestoreStarting},
			}

			failUnlessRetryable(cr, "failed to validate restore job: "+tc.err.Error(), tc.err)

			assert.Equal(t, tc.want, cr.Status.State)
			assert.Equal(t, tc.wantComment, cr.Status.Comments)
		})
	}
}

func TestReconcileRetriesUnavailableSecrets(t *testing.T) {
	const clusterName = "test-cluster"
	const namespace = "namespace"
	const s3SecretName = "my-cluster-name-backup-s3"

	cluster := readDefaultCR(t, clusterName, namespace)
	require.NoError(t, cluster.CheckNSetDefaults(new(version.ServerVersion), logf.FromContext(t.Context())))
	cluster.Status.PXC.Status = api.AppStateReady
	bcp := updateResource(readDefaultBackup(t, clusterName+"-backup", namespace), func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.Status.State = api.BackupSucceeded
		bcp.Status.Destination.SetS3Destination("some-dest", "dest")
		bcp.Spec.StorageName = "s3-us-west"
		bcp.Status.S3 = &api.BackupStorageS3Spec{Bucket: "some-bucket", CredentialsSecret: s3SecretName}
	})
	cr := readDefaultRestore(t, clusterName+"-restore", namespace)
	cr.Spec.BackupName = bcp.Name
	cr.Spec.PXCCluster = clusterName
	cr.CreationTimestamp = metav1.Now()

	for name, tc := range map[string]struct {
		err     error
		want    api.RestoreState
		wantErr bool
	}{
		"Secret unavailable": {err: unavailableSecretError{errors.New("get Secret: unavailable")}, want: api.RestoreStarting},
		"other error":        {err: errors.New("get Secret: broken"), want: api.RestoreFailed, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			cl := interceptor.NewClient(buildFakeClient(cr.DeepCopy(), bcp, cluster,
				readDefaultCRSecret(t, clusterName+"-secrets", namespace),
				readDefaultS3Secret(t, s3SecretName, namespace)).(client.WithWatch),
				interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*corev1.Secret); ok {
						return errors.Wrap(tc.err, "read credentials")
					}
					return c.Get(ctx, key, obj, opts...)
				}})
			r := reconciler(cl)

			res, err := r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

			assert.Equal(t, tc.wantErr, err != nil, "error: %v", err)
			if !tc.wantErr {
				assert.Positive(t, res.RequeueAfter, "a retry must be requeued, not left to the error backoff")
			}
			restore := new(api.PerconaXtraDBClusterRestore)
			require.NoError(t, cl.Get(t.Context(), client.ObjectKeyFromObject(cr), restore))
			assert.Equal(t, tc.want, restore.Status.State, restore.Status.Comments)
			assert.Contains(t, restore.Status.Comments, "failed to validate restore job")
		})
	}
}

func TestReconcileRetriesBinlogCacheInvalidation(t *testing.T) {
	const clusterName = "test-cluster"
	const namespace = "namespace"
	const s3SecretName = "my-cluster-name-backup-s3"

	cluster := readDefaultCR(t, clusterName, namespace)
	cluster.Spec.Backup.PITR.Enabled = true
	cluster.Spec.Backup.PITR.StorageName = "s3-us-west"
	require.NoError(t, cluster.CheckNSetDefaults(new(version.ServerVersion), logf.FromContext(t.Context())))
	bcp := updateResource(readDefaultBackup(t, clusterName+"-backup", namespace), func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.Status.State = api.BackupSucceeded
		bcp.Status.Destination.SetS3Destination("some-dest", "dest")
		bcp.Spec.StorageName = "s3-us-west"
		bcp.Status.S3 = &api.BackupStorageS3Spec{Bucket: "some-bucket", CredentialsSecret: s3SecretName}
	})
	cr := readDefaultRestore(t, clusterName+"-restore", namespace)
	cr.Spec.BackupName = bcp.Name
	cr.Spec.PXCCluster = clusterName
	cr.Status.State = api.RestoreRestore

	for name, tc := range map[string]struct {
		err       error
		completed time.Time
		want      api.RestoreState
	}{
		"Secret unavailable":                  {err: unavailableSecretError{errors.New("get Secret: unavailable")}, completed: time.Now(), want: api.RestoreRestore},
		"Secret unavailable past the timeout": {err: unavailableSecretError{errors.New("get Secret: unavailable")}, completed: time.Now().Add(-secretUnavailableRetryTimeout), want: api.RestorePrepareCluster},
		"other error":                         {err: errors.New("get Secret: broken"), completed: time.Now(), want: api.RestorePrepareCluster},
	} {
		t.Run(name, func(t *testing.T) {
			var failSecrets atomic.Bool
			cl := interceptor.NewClient(buildFakeClient(cr.DeepCopy(), bcp, cluster,
				readDefaultCRSecret(t, clusterName+"-secrets", namespace),
				readDefaultS3Secret(t, s3SecretName, namespace)).(client.WithWatch),
				interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*corev1.Secret); ok && failSecrets.Load() {
						return errors.Wrap(tc.err, "read credentials")
					}
					return c.Get(ctx, key, obj, opts...)
				}})
			r := reconciler(cl)
			restorer, err := r.getRestorer(t.Context(), cr, bcp, cluster)
			require.NoError(t, err)
			job, err := restorer.Job(t.Context())
			require.NoError(t, err)
			job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
			job.Status.CompletionTime = ptr.To(metav1.NewTime(tc.completed))
			require.NoError(t, cl.Create(t.Context(), job))
			failSecrets.Store(true)

			_, err = r.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

			require.NoError(t, err, "a retry must be requeued, not left to the error backoff")
			restore := new(api.PerconaXtraDBClusterRestore)
			require.NoError(t, cl.Get(t.Context(), client.ObjectKeyFromObject(cr), restore))
			assert.Equal(t, tc.want, restore.Status.State, restore.Status.Comments)
			if tc.want == api.RestoreRestore {
				assert.Contains(t, restore.Status.Comments, "retrying binlog collector cache invalidation")
			}
		})
	}
}
