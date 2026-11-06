package pxcbackup

import (
	"context"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/features"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

const (
	incrNamespace     = "pxc"
	incrClusterName   = "cluster1"
	incrStorageName   = "s3-us-west"
	incrBaseLeaf      = "cluster1-2026-11-03-10:00:00-full"
	incrBaseDest      = "s3://bucket/prefix/" + incrBaseLeaf
	incrBaseSourcePod = "cluster1-pxc-1"
)

var incrStartedAt = time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

func incrS3Storage() *api.BackupStorageSpec {
	return &api.BackupStorageSpec{
		Type: api.BackupStorageS3,
		S3:   &api.BackupStorageS3Spec{Bucket: "bucket/prefix", Region: "us-west-1"},
	}
}

func incrCluster(crVersion string, allowParallel bool) *api.PerconaXtraDBCluster {
	return &api.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{Name: incrClusterName, Namespace: incrNamespace},
		Spec: api.PerconaXtraDBClusterSpec{
			CRVersion: crVersion,
			Backup: &api.BackupSpec{
				AllowParallel: ptr.To(allowParallel),
				Storages:      map[string]*api.BackupStorageSpec{incrStorageName: incrS3Storage()},
			},
		},
	}
}

type backupOption func(*api.PerconaXtraDBClusterBackup)

func withBackupType(backupType api.PXCBackupType) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) { bcp.Status.Type = backupType }
}

func withState(state api.PXCBackupState) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) { bcp.Status.State = state }
}

func withDestination(dest string) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.Status.Destination = api.PXCBackupDestination(dest)
	}
}

func withBaseAnnotation(base string) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.SetAnnotations(map[string]string{naming.AnnotationBaseBackupName: base})
	}
}

func withCompletedAt(minutes int) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		t := metav1.NewTime(time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC).Add(time.Duration(minutes) * time.Minute))
		bcp.Status.CompletedAt = &t
	}
}

// withoutCompletedAt clears the completion time, which is the real shape of a
// backup that has not succeeded: the controller only ever records one when the
// job completes.
func withoutCompletedAt() backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.Status.CompletedAt = nil
	}
}

func withCreatedAt(minutes int) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.CreationTimestamp = metav1.NewTime(time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC).Add(time.Duration(minutes) * time.Minute))
	}
}

func withDeletionTimestamp() backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		t := metav1.NewTime(time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))
		bcp.SetDeletionTimestamp(&t)
		bcp.SetFinalizers([]string{naming.FinalizerDeleteBackup})
	}
}

func withStorageSpec(stg *api.BackupStorageSpec) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.Status.StorageType = stg.Type
		bcp.Status.S3 = stg.S3
		bcp.Status.Azure = stg.Azure
	}
}

// withSourcePod records the pod a backup was taken from, as the reconciler does
// once the backup Job is built.
func withSourcePod(pod string) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) { bcp.Status.SourcePod = pod }
}

// withSpecSourcePod pins a backup to a pod the way a user does, via the spec.
func withSpecSourcePod(pod string) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) { bcp.Spec.SourcePod = pod }
}

func withExplicitBase(name string) backupOption {
	return func(bcp *api.PerconaXtraDBClusterBackup) {
		bcp.Spec.IncrementalBaseBackupName = ptr.To(name)
	}
}

func incrBackup(name string, opts ...backupOption) *api.PerconaXtraDBClusterBackup {
	bcp := &api.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         incrNamespace,
			CreationTimestamp: metav1.NewTime(time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)),
		},
		Spec: api.PXCBackupSpec{
			PXCCluster:  incrClusterName,
			StorageName: incrStorageName,
		},
	}
	// Every backup that has got as far as having a destination also has its
	// storage recorded: prepareStatus writes the two together.
	withStorageSpec(incrS3Storage())(bcp)
	for _, opt := range opts {
		opt(bcp)
	}
	return bcp
}

// fullBackup builds a succeeded full backup that an incremental can chain onto.
//
// It records a source pod because a base without one cannot be chained onto at
// all: an incremental has to run on the same node as its base, so the
// reconciler refuses a base whose node it cannot determine.
func fullBackup(name, dest string, completedAt int, opts ...backupOption) *api.PerconaXtraDBClusterBackup {
	base := []backupOption{
		withBackupType(api.BackupTypeFull),
		withState(api.BackupSucceeded),
		withDestination(dest),
		withStorageSpec(incrS3Storage()),
		withCompletedAt(completedAt),
		withSourcePod(incrBaseSourcePod),
	}
	return incrBackup(name, append(base, opts...)...)
}

// incrementalBackup builds an incremental backup already chained to incrBaseLeaf.
func incrementalBackup(name string, state api.PXCBackupState, completedAt int, opts ...backupOption) *api.PerconaXtraDBClusterBackup {
	base := []backupOption{
		withBackupType(api.BackupTypeIncremental),
		withState(state),
		withBaseAnnotation(incrBaseLeaf),
		withDestination(incrBaseDest + ".incr/cluster1-2026-11-03-1" + string(rune('0'+completedAt%10)) + ":00:00-incr"),
		withStorageSpec(incrS3Storage()),
		withCompletedAt(completedAt),
	}
	return incrBackup(name, append(base, opts...)...)
}

// incrReconciler builds a reconciler over a fake client that carries the
// cluster-name field index every chain helper lists through, mirroring the
// manager-side registration in pkg/controller/pxc/controller.go.
func incrReconciler(t *testing.T, objs ...client.Object) *ReconcilePerconaXtraDBClusterBackup {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, api.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&api.PerconaXtraDBClusterBackup{}).
		WithIndex(&api.PerconaXtraDBClusterBackup{}, backup.PXCClusterBackupField, func(obj client.Object) []string {
			bcp, ok := obj.(*api.PerconaXtraDBClusterBackup)
			if !ok {
				return nil
			}
			return []string{bcp.Spec.PXCCluster}
		}).
		Build()

	return &ReconcilePerconaXtraDBClusterBackup{client: cl, scheme: scheme}
}

func contextWithSidecarGate(t *testing.T, enabled bool) context.Context {
	t.Helper()

	gate := features.NewGate()
	if enabled {
		require.NoError(t, gate.Set(string(features.XtrabackupSidecar)+"=true"))
	}
	return features.NewContextWithGate(context.Background(), gate)
}

// ---------------------------------------------------------------------------
// validation gates
// ---------------------------------------------------------------------------

func TestValidateIncrementalBackup(t *testing.T) {
	pvcStorage := &api.BackupStorageSpec{
		Type:   api.BackupStorageFilesystem,
		Volume: &api.VolumeSpec{PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{}},
	}
	gcsStorage := &api.BackupStorageSpec{Type: api.BackupStorageType("gcs")}
	azureStorage := &api.BackupStorageSpec{
		Type:  api.BackupStorageAzure,
		Azure: &api.BackupStorageAzureSpec{ContainerPath: "container/prefix"},
	}

	tests := map[string]struct {
		backupType    api.PXCBackupType
		crVersion     string
		sidecarGate   bool
		storage       *api.BackupStorageSpec
		allowParallel bool
		expectedErr   string
	}{
		"incremental backup with everything in place": {
			backupType:  api.BackupTypeIncremental,
			sidecarGate: true,
			storage:     incrS3Storage(),
		},
		"incremental backup on azure storage": {
			backupType:  api.BackupTypeIncremental,
			sidecarGate: true,
			storage:     azureStorage,
		},
		"incremental backup with the sidecar gate off": {
			backupType:  api.BackupTypeIncremental,
			sidecarGate: false,
			storage:     incrS3Storage(),
			expectedErr: "require the 'XtrabackupSidecar' feature gate",
		},
		"incremental backup on pvc storage": {
			backupType:  api.BackupTypeIncremental,
			sidecarGate: true,
			storage:     pvcStorage,
			expectedErr: `not supported on storage "s3-us-west" of type "filesystem"`,
		},
		"incremental backup on gcs storage": {
			backupType:  api.BackupTypeIncremental,
			sidecarGate: true,
			storage:     gcsStorage,
			expectedErr: `of type "gcs"`,
		},
		"incremental backup with allowParallel": {
			backupType:    api.BackupTypeIncremental,
			sidecarGate:   true,
			storage:       incrS3Storage(),
			allowParallel: true,
		},
		"incremental backup on a cluster below 1.21.0": {
			backupType:  api.BackupTypeIncremental,
			crVersion:   "1.20.0",
			sidecarGate: true,
			storage:     incrS3Storage(),
			expectedErr: "require crVersion 1.21.0 or newer",
		},
		"full backup with the sidecar gate off": {
			backupType:  api.BackupTypeFull,
			sidecarGate: false,
			storage:     incrS3Storage(),
		},
		"full backup on pvc storage": {
			backupType:  api.BackupTypeFull,
			sidecarGate: true,
			storage:     pvcStorage,
		},
		"full backup on gcs storage": {
			backupType:  api.BackupTypeFull,
			sidecarGate: true,
			storage:     gcsStorage,
		},
		"full backup with allowParallel": {
			backupType:    api.BackupTypeFull,
			sidecarGate:   true,
			storage:       incrS3Storage(),
			allowParallel: true,
		},
		"backup with an empty status type is a full backup": {
			backupType:    "",
			sidecarGate:   false,
			storage:       pvcStorage,
			allowParallel: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			crVersion := tt.crVersion
			if crVersion == "" {
				crVersion = "1.21.0"
			}

			ctx := contextWithSidecarGate(t, tt.sidecarGate)
			cr := incrBackup("backup1", withBackupType(tt.backupType))

			err := validateIncrementalBackup(ctx, cr, incrCluster(crVersion, tt.allowParallel), tt.storage)
			if tt.expectedErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestBackupNeedsLease(t *testing.T) {
	tests := map[string]struct {
		backupType    api.PXCBackupType
		allowParallel bool
		want          bool
	}{
		"full backup, parallel disallowed": {
			backupType: api.BackupTypeFull,
			want:       true,
		},
		"full backup, parallel allowed": {
			backupType:    api.BackupTypeFull,
			allowParallel: true,
			want:          false,
		},
		"incremental backup, parallel disallowed": {
			backupType: api.BackupTypeIncremental,
			want:       true,
		},
		"incremental backup ignores allowParallel": {
			backupType:    api.BackupTypeIncremental,
			allowParallel: true,
			want:          true,
		},
		"a legacy backup with no recorded type counts as full": {
			backupType:    "",
			allowParallel: true,
			want:          false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := incrBackup("backup1", withBackupType(tt.backupType))

			assert.Equal(t, tt.want, backupNeedsLease(cr, incrCluster("1.21.0", tt.allowParallel)))
		})
	}
}

// TestBackupLeaseHolderStale covers when the cluster's backup lock may be taken
// away from whoever currently holds it.
func TestBackupLeaseHolderStale(t *testing.T) {
	const holderUID = "11111111-1111-1111-1111-111111111111"

	tests := map[string]struct {
		holderState    api.PXCBackupState
		holderDeleting bool
		holderMissing  bool
		// identityOverride replaces the identity asked about; nil asks about
		// the holder backup itself.
		identityOverride *string
		want             bool
	}{
		"a running holder keeps the lease": {
			holderState: api.BackupRunning,
			want:        false,
		},
		"a starting holder keeps the lease": {
			holderState: api.BackupStarting,
			want:        false,
		},
		"a suspended holder keeps the lease": {
			holderState: api.BackupSuspended,
			want:        false,
		},
		"a succeeded holder is stale": {
			holderState: api.BackupSucceeded,
			want:        true,
		},
		"a failed holder is stale": {
			holderState: api.BackupFailed,
			want:        true,
		},
		"a holder being deleted is stale": {
			holderState:    api.BackupRunning,
			holderDeleting: true,
			want:           true,
		},
		"a holder that no longer exists is stale": {
			holderState:   api.BackupRunning,
			holderMissing: true,
			want:          true,
		},
		"an empty holder identity is stale": {
			holderState:      api.BackupRunning,
			identityOverride: ptr.To(""),
			want:             true,
		},
		// A backup recreated under an old name gets a fresh UID, so an
		// identity carrying the old one names an object that is gone.
		"an identity naming a different UID is stale": {
			holderState:      api.BackupRunning,
			identityOverride: ptr.To("holder-99999999-9999-9999-9999-999999999999"),
			want:             true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			opts := []backupOption{withState(tt.holderState)}
			if tt.holderDeleting {
				opts = append(opts, withDeletionTimestamp())
			}
			bcp := incrBackup("holder", opts...)
			bcp.UID = holderUID

			var objs []client.Object
			if !tt.holderMissing {
				objs = append(objs, bcp)
			}
			r := incrReconciler(t, objs...)

			identity := naming.BackupHolderId(bcp)
			if tt.identityOverride != nil {
				identity = *tt.identityOverride
			}

			stale, err := r.backupLeaseHolderStale(incrNamespace)(t.Context(), identity)
			require.NoError(t, err)
			assert.Equal(t, tt.want, stale)
		})
	}
}

// TestEnsureFinalizersFollowsTheLease checks the release-lock finalizer is
// added exactly when the lease is taken. Acquiring without the finalizer would
// orphan the lease when a backup is deleted mid-flight, blocking every later
// backup on the cluster; adding it without acquiring is dead weight on the
// object.
func TestEnsureFinalizersFollowsTheLease(t *testing.T) {
	tests := map[string]struct {
		backupType    api.PXCBackupType
		allowParallel bool
		wantFinalizer bool
	}{
		"full backup, parallel disallowed": {
			backupType:    api.BackupTypeFull,
			wantFinalizer: true,
		},
		"full backup, parallel allowed": {
			backupType:    api.BackupTypeFull,
			allowParallel: true,
			wantFinalizer: false,
		},
		"incremental backup, parallel allowed": {
			backupType:    api.BackupTypeIncremental,
			allowParallel: true,
			wantFinalizer: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := incrBackup("backup1", withBackupType(tt.backupType))
			cluster := incrCluster("1.21.0", tt.allowParallel)
			r := incrReconciler(t, cr, cluster)

			require.NoError(t, r.ensureFinalizers(t.Context(), cluster, cr))

			if tt.wantFinalizer {
				assert.Contains(t, cr.GetFinalizers(), naming.FinalizerReleaseLock)
				return
			}
			assert.NotContains(t, cr.GetFinalizers(), naming.FinalizerReleaseLock)
		})
	}
}

func TestSetIncrementalBase(t *testing.T) {
	newerBase := "s3://bucket/prefix/cluster1-2026-11-03-11:00:00-full"

	tests := map[string]struct {
		cr           *api.PerconaXtraDBClusterBackup
		existing     []client.Object
		expectedBase string
		// expectedPin is the node the backup must end up tied to; empty skips
		// the check.
		expectedPin string
		expectedErr string
	}{
		"resolves the newest succeeded full backup in the storage": {
			cr: incrBackup("incr-1", withBackupType(api.BackupTypeIncremental)),
			existing: []client.Object{
				fullBackup("full-old", "s3://bucket/prefix/cluster1-2026-11-03-08:00:00-full", 1),
				fullBackup("full-new", incrBaseDest, 2),
			},
			expectedBase: incrBaseLeaf,
		},
		"honours an explicitly named base": {
			cr: incrBackup("incr-1", withBackupType(api.BackupTypeIncremental), withExplicitBase("full-old")),
			existing: []client.Object{
				fullBackup("full-old", incrBaseDest, 1),
				fullBackup("full-new", newerBase, 2),
			},
			expectedBase: incrBaseLeaf,
		},
		"never recomputes an annotation that is already set": {
			cr: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
			),
			existing: []client.Object{
				fullBackup("full-base", incrBaseDest, 1),
				fullBackup("full-new", newerBase, 5, withSourcePod("cluster1-pxc-2")),
			},
			expectedBase: incrBaseLeaf,
			expectedPin:  incrBaseSourcePod,
		},
		"full backups are left alone": {
			cr: incrBackup("full-1", withBackupType(api.BackupTypeFull)),
			existing: []client.Object{
				fullBackup("full-new", incrBaseDest, 2),
			},
			expectedBase: "",
		},
		"fails when the cluster has no full backup": {
			cr:          incrBackup("incr-1", withBackupType(api.BackupTypeIncremental)),
			expectedErr: "no full backup found in storage",
		},
		"fails when the only full backup did not succeed": {
			cr: incrBackup("incr-1", withBackupType(api.BackupTypeIncremental)),
			existing: []client.Object{
				incrBackup("full-running",
					withBackupType(api.BackupTypeFull),
					withState(api.BackupRunning),
					withDestination(incrBaseDest),
					withStorageSpec(incrS3Storage()),
				),
			},
			expectedErr: "no full backup found in storage",
		},
		"fails when the named base is itself an incremental backup": {
			cr: incrBackup("incr-2", withBackupType(api.BackupTypeIncremental), withExplicitBase("incr-1")),
			existing: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 2),
			},
			expectedErr: "only a full backup can be the base of a chain",
		},
		"fails when the base is in a different storage": {
			cr: incrBackup("incr-1", withBackupType(api.BackupTypeIncremental), withExplicitBase("full-elsewhere")),
			existing: []client.Object{
				incrBackup("full-elsewhere",
					withBackupType(api.BackupTypeFull),
					withState(api.BackupSucceeded),
					withDestination("s3://other-bucket/cluster1-2026-11-03-10:00:00-full"),
					withStorageSpec(&api.BackupStorageSpec{
						Type: api.BackupStorageS3,
						S3:   &api.BackupStorageS3Spec{Bucket: "other-bucket"},
					}),
					withCompletedAt(1),
				),
			},
			expectedErr: "which is not the storage of incremental backup",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			objs := append([]client.Object{tt.cr}, tt.existing...)
			r := incrReconciler(t, objs...)

			err := r.setIncrementalBase(t.Context(), tt.cr, incrCluster("1.21.0", false), incrS3Storage())
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}
			require.NoError(t, err)

			// The in-memory CR must carry the annotation: the destination is
			// derived from it a few lines later in the same reconcile.
			assert.Equal(t, tt.expectedBase, tt.cr.GetAnnotations()[naming.AnnotationBaseBackupName])

			persisted := new(api.PerconaXtraDBClusterBackup)
			require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(tt.cr), persisted))
			assert.Equal(t, tt.expectedBase, persisted.GetAnnotations()[naming.AnnotationBaseBackupName])

			if tt.expectedPin != "" {
				assert.Equal(t, tt.expectedPin, tt.cr.Spec.SourcePod,
					"the node the backup is tied to must come from the base the annotation names")
			}
		})
	}
}

func TestSetIncrementalBaseAnnotationPinsSourcePod(t *testing.T) {
	tests := map[string]struct {
		incremental *api.PerconaXtraDBClusterBackup
		base        *api.PerconaXtraDBClusterBackup
		wantPin     string
		wantErr     string
	}{
		"incremental inherits the pod its base was taken from": {
			incremental: incrBackup("incr-1", withBackupType(api.BackupTypeIncremental)),
			base:        fullBackup("full-1", incrBaseDest, 1),
			wantPin:     incrBaseSourcePod,
		},
		"a base whose pod was never recorded cannot be chained onto": {
			incremental: incrBackup("incr-1", withBackupType(api.BackupTypeIncremental)),
			base:        fullBackup("full-1", incrBaseDest, 1, withSourcePod("")),
			wantErr:     "source cannot be known",
		},
		"an explicit pin is overridden by the pod the base was taken from": {
			incremental: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withSpecSourcePod("cluster1-pxc-2"),
			),
			base:    fullBackup("full-1", incrBaseDest, 1),
			wantPin: incrBaseSourcePod,
		},
		"an explicit pin is honoured when the base has no recorded pod": {
			incremental: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withSpecSourcePod("cluster1-pxc-2"),
			),
			base:    fullBackup("full-1", incrBaseDest, 1, withSourcePod("")),
			wantPin: "cluster1-pxc-2",
		},
		"a full backup is never pinned to anything": {
			incremental: incrBackup("full-2", withBackupType(api.BackupTypeFull)),
			base:        fullBackup("full-1", incrBaseDest, 1),
			wantPin:     "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := incrReconciler(t, tt.incremental, tt.base)

			err := r.setIncrementalBase(t.Context(), tt.incremental, incrCluster("1.21.0", false), incrS3Storage())

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				// A backup that cannot be pinned must not be left half-resolved.
				assert.NotContains(t, tt.incremental.GetAnnotations(), naming.AnnotationBaseBackupName)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPin, tt.incremental.Spec.SourcePod,
				"the pod the backup Job will be sent to")
		})
	}
}

func TestSourcePodReachesTheBackupJob(t *testing.T) {
	tests := map[string]struct {
		specSourcePod string
		wantPod       string
	}{
		"pinned to the base's pod":    {specSourcePod: incrBaseSourcePod, wantPod: incrBaseSourcePod},
		"pinned to a user-chosen pod": {specSourcePod: "cluster1-pxc-2", wantPod: "cluster1-pxc-2"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cr := incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withSpecSourcePod(tt.specSourcePod),
			)
			cluster := incrCluster("1.21.0", false)

			node, err := backup.GetNodeForBackup(t.Context(), incrReconciler(t, cr).client, cluster, cr)
			require.NoError(t, err)

			assert.Equal(t, tt.wantPod, node.Pod)
			// The host is the pod's FQDN, so a chain cannot silently drift to
			// another node even when a proxy would have offered one.
			assert.Contains(t, node.FQDN, tt.wantPod)
		})
	}
}

// ---------------------------------------------------------------------------
// destination
// ---------------------------------------------------------------------------

func TestSetBackupDestination(t *testing.T) {
	azureStorage := &api.BackupStorageSpec{
		Type:  api.BackupStorageAzure,
		Azure: &api.BackupStorageAzureSpec{ContainerPath: "container/prefix"},
	}

	tests := map[string]struct {
		cr          *api.PerconaXtraDBClusterBackup
		storage     *api.BackupStorageSpec
		expected    string
		expectedErr string
	}{
		"full backup on s3 keeps the legacy layout": {
			cr:       incrBackup("full-1", withBackupType(api.BackupTypeFull)),
			storage:  incrS3Storage(),
			expected: "s3://bucket/prefix/cluster1-2026-11-03-09:30:00-full",
		},
		"backup with an empty status type is a full backup": {
			cr:       incrBackup("full-1"),
			storage:  incrS3Storage(),
			expected: "s3://bucket/prefix/cluster1-2026-11-03-09:30:00-full",
		},
		"full backup on azure keeps the legacy layout": {
			cr:       incrBackup("full-1", withBackupType(api.BackupTypeFull)),
			storage:  azureStorage,
			expected: "azure://container/prefix/cluster1-2026-11-03-09:30:00-full",
		},
		"incremental backup nests under the chain directory of its base": {
			cr: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
			),
			storage:  incrS3Storage(),
			expected: "s3://bucket/prefix/" + incrBaseLeaf + ".incr/cluster1-2026-11-03-09:30:00-incr",
		},
		"incremental backup on azure nests under the chain directory too": {
			cr: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
			),
			storage:  azureStorage,
			expected: "azure://container/prefix/" + incrBaseLeaf + ".incr/cluster1-2026-11-03-09:30:00-incr",
		},
		"incremental backup without a resolved base is a programming error": {
			cr:          incrBackup("incr-1", withBackupType(api.BackupTypeIncremental)),
			storage:     incrS3Storage(),
			expectedErr: "has no percona.com/base-backup-name annotation",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := setBackupDestination(tt.cr, tt.storage, nil, incrStartedAt)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, tt.cr.Status.Destination.String())
		})
	}
}

// TestBackupDestinationIsComputedOnce covers the guard in createBackupJob: the
// destination carries a timestamp and, for an incremental backup, the base
// annotation, so recomputing it would point status at a path the running Job is
// not writing to.
func TestBackupDestinationIsComputedOnce(t *testing.T) {
	tests := map[string]struct {
		cr       *api.PerconaXtraDBClusterBackup
		expected string
	}{
		"full backup": {
			cr: incrBackup("full-1",
				withBackupType(api.BackupTypeFull),
				withDestination("s3://bucket/prefix/cluster1-2026-11-03-08:59:59-full"),
			),
			expected: "s3://bucket/prefix/cluster1-2026-11-03-08:59:59-full",
		},
		"incremental backup": {
			cr: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
				withDestination(incrBaseDest+".incr/cluster1-2026-11-03-08:59:59-incr"),
			),
			expected: incrBaseDest + ".incr/cluster1-2026-11-03-08:59:59-incr",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// This is the guard createBackupJob applies; a second pass must not
			// reach setBackupDestination at all.
			if tt.cr.Status.Destination == "" {
				require.NoError(t, setBackupDestination(tt.cr, incrS3Storage(), nil, incrStartedAt))
			}

			assert.Equal(t, tt.expected, tt.cr.Status.Destination.String())
		})
	}
}

// ---------------------------------------------------------------------------
// predecessor selection
// ---------------------------------------------------------------------------

func TestIncrementalPredecessorDestination(t *testing.T) {
	incrDest := func(hour int) string {
		return incrBaseDest + ".incr/cluster1-2026-11-03-1" + string(rune('0'+hour%10)) + ":00:00-incr"
	}

	tests := map[string]struct {
		cr          *api.PerconaXtraDBClusterBackup
		chain       []client.Object
		expected    string
		expectedErr string
	}{
		"full backup has no predecessor": {
			cr:       incrBackup("full-1", withBackupType(api.BackupTypeFull), withDestination(incrBaseDest)),
			expected: "",
		},
		"first incremental in a chain starts from the base": {
			cr: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
				withDestination(incrDest(1)),
				withState(api.BackupStarting),
			),
			expected: "prefix/" + incrBaseLeaf,
		},
		"later incremental starts from the newest succeeded member of the chain": {
			cr: incrBackup("incr-3",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
				withDestination(incrDest(3)),
				withState(api.BackupStarting),
			),
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupSucceeded, 2),
			},
			expected: "prefix/" + incrBaseLeaf + ".incr/cluster1-2026-11-03-12:00:00-incr",
		},
		"a failed tail is skipped in favour of the newest succeeded member": {
			cr: incrBackup("incr-3",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
				withDestination(incrDest(3)),
				withState(api.BackupStarting),
			),
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupFailed, 2),
			},
			expected: "prefix/" + incrBaseLeaf + ".incr/cluster1-2026-11-03-11:00:00-incr",
		},
		"a chain of only failed incrementals starts from the base": {
			cr: incrBackup("incr-2",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
				withDestination(incrDest(2)),
				withState(api.BackupStarting),
			),
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupFailed, 1),
			},
			expected: "prefix/" + incrBaseLeaf,
		},
		"incrementals of another chain are not considered": {
			cr: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
				withDestination(incrDest(1)),
				withState(api.BackupStarting),
			),
			chain: []client.Object{
				incrBackup("other-incr",
					withBackupType(api.BackupTypeIncremental),
					withState(api.BackupSucceeded),
					withBaseAnnotation("cluster1-2026-11-02-10:00:00-full"),
					withDestination("s3://bucket/prefix/cluster1-2026-11-02-10:00:00-full.incr/cluster1-2026-11-02-11:00:00-incr"),
					withCompletedAt(9),
				),
			},
			expected: "prefix/" + incrBaseLeaf,
		},
		"an incremental without a destination cannot be placed": {
			cr: incrBackup("incr-1",
				withBackupType(api.BackupTypeIncremental),
				withBaseAnnotation(incrBaseLeaf),
			),
			expectedErr: "has no destination recorded",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			objs := append([]client.Object{tt.cr}, tt.chain...)
			r := incrReconciler(t, objs...)

			got, err := r.incrementalPredecessorDestination(t.Context(), tt.cr)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// chain-safe deletion
// ---------------------------------------------------------------------------

func TestCanDeleteIncrementalBackup(t *testing.T) {
	tests := map[string]struct {
		target      string
		chain       []client.Object
		expected    bool
		expectedErr string
	}{
		"a full backup is always deletable": {
			target: "full-1",
			chain: []client.Object{
				fullBackup("full-1", incrBaseDest, 1),
				incrementalBackup("incr-1", api.BackupSucceeded, 2),
			},
			expected: true,
		},
		"the tail of a chain is deletable": {
			target: "incr-3",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupSucceeded, 2),
				incrementalBackup("incr-3", api.BackupSucceeded, 3),
			},
			expected: true,
		},
		"a middle member of a chain is refused": {
			target: "incr-2",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupSucceeded, 2),
				incrementalBackup("incr-3", api.BackupSucceeded, 3),
			},
			expected: false,
		},
		"an unfinished tail still blocks the member underneath it": {
			target: "incr-1",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupRunning, 2, withoutCompletedAt(), withCreatedAt(2)),
			},
			expected: false,
		},
		// The running backup was asked for before the one underneath it
		// finished, which is what happens whenever a second backup waits on the
		// backup lease. Judged by creation time it looks like the older of the
		// two, and the member it is about to be built on looks like the tail.
		"a backup waiting on the lease blocks the member it will be built on": {
			target: "incr-1",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 50, withCreatedAt(10)),
				incrementalBackup("incr-2", api.BackupRunning, 0, withoutCompletedAt(), withCreatedAt(20)),
			},
			expected: false,
		},
		"a backup still waiting on the lease blocks it too": {
			target: "incr-1",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 50, withCreatedAt(10)),
				incrementalBackup("incr-2", api.BackupNew, 0, withoutCompletedAt(), withCreatedAt(20)),
			},
			expected: false,
		},
		// A failed backup is terminal: it will never be appended to the chain,
		// nothing reads an LSN from it, and restore skips it for want of a
		// completion marker. Letting it block would strand the real tail, since
		// nothing reaps failed backups on its own.
		"a failed member does not block the tail": {
			target: "incr-1",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupFailed, 0, withoutCompletedAt(), withCreatedAt(2)),
			},
			expected: true,
		},
		"a failed backup is itself deletable, wherever it sits": {
			target: "incr-2",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupFailed, 0, withoutCompletedAt(), withCreatedAt(2)),
				incrementalBackup("incr-3", api.BackupSucceeded, 3),
			},
			expected: true,
		},
		"a running backup is itself deletable: nothing is chained onto it yet": {
			target: "incr-2",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupRunning, 0, withoutCompletedAt(), withCreatedAt(2)),
			},
			expected: true,
		},
		"the only member of a chain is deletable": {
			target: "incr-1",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
			},
			expected: true,
		},
		"a succeeded member with no completion time is an error": {
			target: "incr-1",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupSucceeded, 0, withoutCompletedAt()),
			},
			expectedErr: "is succeeded but completedAt is not set",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := incrReconciler(t, tt.chain...)

			target := new(api.PerconaXtraDBClusterBackup)
			require.NoError(t, r.client.Get(t.Context(), client.ObjectKey{Namespace: incrNamespace, Name: tt.target}, target))

			can, err := r.canDeleteIncrementalBackup(t.Context(), target)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, can)
		})
	}
}

func TestDeleteDependentIncrementalBackups(t *testing.T) {
	tests := map[string]struct {
		target string
		chain  []client.Object
		// expectedDone reports whether the base's own objects may now be deleted.
		expectedDone bool
		// expectedDeleted is the dependent that must have been deleted by this
		// call: exactly one, and always one that canDeleteIncrementalBackup
		// will let through, or the cascade waits on a deletion its own
		// finalizer refuses to carry out.
		expectedDeleted string
		expectedRemain  []string
	}{
		"a full backup with no dependents is done immediately": {
			target:       "full-1",
			chain:        []client.Object{fullBackup("full-1", incrBaseDest, 1)},
			expectedDone: true,
		},
		"an incremental backup has no dependents to cascade to": {
			target: "incr-1",
			chain: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-2", api.BackupSucceeded, 2),
			},
			expectedDone:   true,
			expectedRemain: []string{"incr-1", "incr-2"},
		},
		"the newest dependent is deleted first, one per call": {
			target: "full-1",
			chain: []client.Object{
				fullBackup("full-1", incrBaseDest, 1),
				incrementalBackup("incr-1", api.BackupSucceeded, 2),
				incrementalBackup("incr-2", api.BackupSucceeded, 3),
				incrementalBackup("incr-3", api.BackupSucceeded, 4),
			},
			expectedDone:    false,
			expectedDeleted: "incr-3",
			expectedRemain:  []string{"full-1", "incr-1", "incr-2"},
		},
		"unfinished dependents are cascaded to as well": {
			target: "full-1",
			chain: []client.Object{
				fullBackup("full-1", incrBaseDest, 1),
				incrementalBackup("incr-1", api.BackupSucceeded, 2),
				incrementalBackup("incr-2", api.BackupFailed, 0, withoutCompletedAt(), withCreatedAt(3)),
			},
			expectedDone:    false,
			expectedDeleted: "incr-2",
			expectedRemain:  []string{"full-1", "incr-1"},
		},
		// Nothing is ever chained onto a backup that did not succeed, so it can
		// always go, while the succeeded tail underneath it cannot while the
		// running one is still there to be appended after it. Picking by
		// completion time alone would reach for the succeeded one first and then
		// wait on a deletion its finalizer refuses.
		"a running dependent is taken before the succeeded tail": {
			target: "full-1",
			chain: []client.Object{
				fullBackup("full-1", incrBaseDest, 1),
				incrementalBackup("incr-1", api.BackupSucceeded, 50, withCreatedAt(10)),
				incrementalBackup("incr-2", api.BackupRunning, 0, withoutCompletedAt(), withCreatedAt(20)),
			},
			expectedDone:    false,
			expectedDeleted: "incr-2",
			expectedRemain:  []string{"full-1", "incr-1"},
		},
		"a dependent that is already terminating is waited for": {
			target: "full-1",
			chain: []client.Object{
				fullBackup("full-1", incrBaseDest, 1),
				incrementalBackup("incr-1", api.BackupSucceeded, 2),
				incrementalBackup("incr-2", api.BackupSucceeded, 3, withDeletionTimestamp()),
			},
			expectedDone:   false,
			expectedRemain: []string{"full-1", "incr-1", "incr-2"},
		},
		"incrementals of another chain are not cascaded to": {
			target: "full-1",
			chain: []client.Object{
				fullBackup("full-1", incrBaseDest, 1),
				incrBackup("other-incr",
					withBackupType(api.BackupTypeIncremental),
					withState(api.BackupSucceeded),
					withBaseAnnotation("cluster1-2026-11-02-10:00:00-full"),
					withCompletedAt(9),
				),
			},
			expectedDone:   true,
			expectedRemain: []string{"full-1", "other-incr"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := incrReconciler(t, tt.chain...)

			target := new(api.PerconaXtraDBClusterBackup)
			require.NoError(t, r.client.Get(t.Context(), client.ObjectKey{Namespace: incrNamespace, Name: tt.target}, target))

			done, err := r.deleteDependentIncrementalBackups(t.Context(), target)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedDone, done)

			list := new(api.PerconaXtraDBClusterBackupList)
			require.NoError(t, r.client.List(t.Context(), list, client.InNamespace(incrNamespace)))

			remaining := make([]string, 0, len(list.Items))
			for i := range list.Items {
				remaining = append(remaining, list.Items[i].Name)
			}
			sort.Strings(remaining)

			if tt.expectedRemain != nil {
				expected := append([]string(nil), tt.expectedRemain...)
				sort.Strings(expected)
				assert.Equal(t, expected, remaining)
			}
			if tt.expectedDeleted != "" {
				assert.NotContains(t, remaining, tt.expectedDeleted)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// object storage cleanup
// ---------------------------------------------------------------------------

// fakeStorage is a storage.Storage that serves a fixed set of object keys with
// the same raw prefix matching the real S3 and Azure clients use, and records
// every prefix it was asked to list.
type fakeStorage struct {
	prefix  string
	objects []string

	listed  []string
	deleted []string
}

func (s *fakeStorage) ListObjects(_ context.Context, prefix string) ([]string, error) {
	s.listed = append(s.listed, prefix)

	found := []string{}
	for _, object := range s.objects {
		if strings.HasPrefix(object, prefix) {
			found = append(found, object)
		}
	}
	return found, nil
}

func (s *fakeStorage) DeleteObject(_ context.Context, name string) error {
	s.deleted = append(s.deleted, name)
	return nil
}

func (s *fakeStorage) GetObject(context.Context, string) (io.ReadCloser, error) { return nil, nil }

func (s *fakeStorage) PutObject(context.Context, string, io.Reader, int64) error { return nil }

func (s *fakeStorage) SetPrefix(prefix string) { s.prefix = prefix }

func (s *fakeStorage) GetPrefix() string { return s.prefix }

var _ storage.Storage = new(fakeStorage)

// TestRemoveBackupObjects pins the anchored directory listing: deleting a full
// backup must not reach into the sibling "<full>.incr/" chain directory, or the
// base takes its whole chain with it before the cascade ever runs.
func TestRemoveBackupObjects(t *testing.T) {
	const full = "cluster1-2026-11-03-10:00:00-full"

	chainObjects := []string{
		full + ".incr/cluster1-2026-11-03-11:00:00-incr/xtrabackup_info.00000000000000000000",
		full + ".incr/cluster1-2026-11-03-11:00:00-incr/ibdata1.00000000000000000000",
		full + ".incr/cluster1-2026-11-03-11:00:00-incr.md5",
	}

	tests := map[string]struct {
		destination     string
		objects         []string
		expectedDeleted []string
		forbidden       []string
	}{
		"a full backup deletes its own directory and siblings only": {
			destination: full,
			objects: append([]string{
				full + "/xtrabackup_info.00000000000000000000",
				full + "/ibdata1.00000000000000000000",
				full + ".md5",
				full + ".meta.json",
				full + ".sst_info/sst_info.00000000000000000000",
				full + ".sst_info.md5",
			}, chainObjects...),
			expectedDeleted: []string{
				full + "/xtrabackup_info.00000000000000000000",
				full + "/ibdata1.00000000000000000000",
				full + ".md5",
				full + ".meta.json",
				full + "." + naming.PITRNotReady,
				full + ".sst_info/sst_info.00000000000000000000",
				full + ".sst_info.md5",
			},
			forbidden: chainObjects,
		},
		"an incremental backup deletes its own directory inside the chain": {
			destination: full + ".incr/cluster1-2026-11-03-11:00:00-incr",
			objects:     chainObjects,
			expectedDeleted: []string{
				full + ".incr/cluster1-2026-11-03-11:00:00-incr/xtrabackup_info.00000000000000000000",
				full + ".incr/cluster1-2026-11-03-11:00:00-incr/ibdata1.00000000000000000000",
				full + ".incr/cluster1-2026-11-03-11:00:00-incr.md5",
				full + ".incr/cluster1-2026-11-03-11:00:00-incr.meta.json",
				full + ".incr/cluster1-2026-11-03-11:00:00-incr." + naming.PITRNotReady,
				full + ".incr/cluster1-2026-11-03-11:00:00-incr.sst_info.md5",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := &fakeStorage{objects: tt.objects}

			require.NoError(t, removeBackupObjects(t.Context(), s, tt.destination)())

			assert.ElementsMatch(t, tt.expectedDeleted, s.deleted)

			for _, object := range tt.forbidden {
				assert.NotContains(t, s.deleted, object, "object of another backup was deleted")
			}

			// Every listing must be anchored to a directory boundary, otherwise
			// the chain directory is swept in with the backup directory.
			for _, prefix := range s.listed {
				assert.True(t, strings.HasSuffix(prefix, "/"), "listing %q is not anchored to a directory", prefix)
				for _, object := range tt.forbidden {
					assert.False(t, strings.HasPrefix(object, prefix), "listing %q reaches into another backup", prefix)
				}
			}
		})
	}
}

func TestBackupObjectPrefix(t *testing.T) {
	tests := map[string]struct {
		destination  string
		clientPrefix string
		expected     string
	}{
		"full backup under a prefix": {
			destination:  incrBaseDest,
			clientPrefix: "prefix/",
			expected:     incrBaseLeaf,
		},
		"full backup without a prefix": {
			destination:  "s3://bucket/" + incrBaseLeaf,
			clientPrefix: "",
			expected:     incrBaseLeaf,
		},
		"incremental backup keeps its chain directory": {
			destination:  incrBaseDest + ".incr/cluster1-2026-11-03-11:00:00-incr",
			clientPrefix: "prefix/",
			expected:     incrBaseLeaf + ".incr/cluster1-2026-11-03-11:00:00-incr",
		},
		"incremental backup when the client is scoped to the chain directory": {
			destination:  incrBaseDest + ".incr/cluster1-2026-11-03-11:00:00-incr",
			clientPrefix: "prefix/" + incrBaseLeaf + ".incr/",
			expected:     "cluster1-2026-11-03-11:00:00-incr",
		},
		"incremental backup without a prefix": {
			destination:  "s3://bucket/" + incrBaseLeaf + ".incr/cluster1-2026-11-03-11:00:00-incr",
			clientPrefix: "",
			expected:     incrBaseLeaf + ".incr/cluster1-2026-11-03-11:00:00-incr",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dest := api.PXCBackupDestination(tt.destination)
			assert.Equal(t, tt.expected, backupObjectPrefix(&dest, tt.clientPrefix))
		})
	}
}

// pvcBackup builds a backup stored on a PVC. The chain checks in
// runBackupFinalizers are storage-independent, and a PVC-backed backup lets the
// test run the whole finalizer, deletion step included, without an object
// storage client.
func pvcBackup(name string, opts ...backupOption) (*api.PerconaXtraDBClusterBackup, *corev1.PersistentVolumeClaim) {
	pvcName := name + "-pvc"

	base := []backupOption{
		withState(api.BackupSucceeded),
		withDestination(api.PVCStoragePrefix + pvcName),
		func(bcp *api.PerconaXtraDBClusterBackup) {
			bcp.Status.StorageType = api.BackupStorageFilesystem
			bcp.Status.PVC = &corev1.PersistentVolumeClaimSpec{}
		},
		withDeletionTimestamp(),
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: incrNamespace},
	}

	return incrBackup(name, append(base, opts...)...), pvc
}

// TestRunBackupFinalizersChainOrdering covers the order the delete-backup
// finalizer runs in: refuse a mid-chain incremental, then cascade a full
// backup's dependents, and only then remove the backup's own data. Refusing
// means re-appending the finalizer - this runs detached in a goroutine and
// rebuilds the finalizer list, so a finalizer that is not re-appended is gone.
func TestRunBackupFinalizersChainOrdering(t *testing.T) {
	tailIncr, tailPVC := pvcBackup("incr-tail",
		withBackupType(api.BackupTypeIncremental),
		withBaseAnnotation(incrBaseLeaf),
		withCompletedAt(3),
	)
	lonelyFull, lonelyPVC := pvcBackup("full-lonely",
		withBackupType(api.BackupTypeFull),
		withCompletedAt(1),
	)

	tests := map[string]struct {
		target             *api.PerconaXtraDBClusterBackup
		objects            []client.Object
		expectedFinalizers []string
		// expectedGone is a dependent the cascade must have deleted.
		expectedGone string
	}{
		"a mid-chain incremental is refused and keeps its finalizer": {
			target: incrementalBackup("incr-2", api.BackupSucceeded, 2, withDeletionTimestamp()),
			objects: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 1),
				incrementalBackup("incr-3", api.BackupSucceeded, 3),
			},
			expectedFinalizers: []string{naming.FinalizerDeleteBackup},
		},
		"a full backup with dependents deletes the newest one and keeps its finalizer": {
			target: incrBackup("full-1",
				withBackupType(api.BackupTypeFull),
				withState(api.BackupSucceeded),
				withDestination(incrBaseDest),
				withStorageSpec(incrS3Storage()),
				withCompletedAt(1),
				withDeletionTimestamp(),
			),
			objects: []client.Object{
				incrementalBackup("incr-1", api.BackupSucceeded, 2),
				incrementalBackup("incr-2", api.BackupSucceeded, 3),
			},
			expectedFinalizers: []string{naming.FinalizerDeleteBackup},
			expectedGone:       "incr-2",
		},
		"the tail of a chain is deleted and drops its finalizer": {
			target:             tailIncr,
			objects:            []client.Object{tailPVC, incrementalBackup("incr-1", api.BackupSucceeded, 1)},
			expectedFinalizers: nil,
		},
		"a full backup with no dependents is deleted and drops its finalizer": {
			target:             lonelyFull,
			objects:            []client.Object{lonelyPVC},
			expectedFinalizers: nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := incrReconciler(t, append([]client.Object{tt.target}, tt.objects...)...)
			r.chLimit = make(chan struct{}, 1)
			r.bcpDeleteInProgress = new(sync.Map)
			r.chLimit <- struct{}{}

			cr := new(api.PerconaXtraDBClusterBackup)
			require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(tt.target), cr))

			r.runBackupFinalizers(t.Context(), cr)

			assert.Equal(t, tt.expectedFinalizers, cr.GetFinalizers())

			if tt.expectedGone != "" {
				gone := new(api.PerconaXtraDBClusterBackup)
				err := r.client.Get(t.Context(), client.ObjectKey{Namespace: incrNamespace, Name: tt.expectedGone}, gone)
				assert.True(t, k8sErrors.IsNotFound(err), "the newest dependent must have been deleted, got %v", err)
			}
		})
	}
}

// TestWriteStatusRefusesStaleWrites covers the guard that keeps a reconcile
// working from a stale informer read from overwriting a newer status.
//
// r.client reads through the cache, so the CR a reconcile starts from can be
// behind the API. RetryOnConflict alone makes that worse: it refreshes the
// object and then writes back values derived from the stale read. That is how a
// backup ends up recorded at a destination its Job is not writing to.
func TestWriteStatusRefusesStaleWrites(t *testing.T) {
	const (
		storedDest   = "s3://bucket/prefix/cluster1-2026-11-03-10:00:00-full"
		computedDest = "s3://bucket/prefix/cluster1-2026-11-03-11:00:00-full"
	)

	tests := map[string]struct {
		// storedState is what the API holds.
		storedState api.PXCBackupState
		// observedState is what this reconcile believes it read.
		observedState api.PXCBackupState
		computedDest  string
		wantErr       error
		wantStored    string
	}{
		"writes when the API still holds what we observed": {
			storedState:   api.BackupStarting,
			observedState: api.BackupStarting,
			computedDest:  computedDest,
			wantStored:    computedDest,
		},
		"refuses when the API moved on": {
			storedState:   api.BackupRunning,
			observedState: api.BackupStarting,
			computedDest:  computedDest,
			wantErr:       errStaleStatus,
			wantStored:    storedDest,
		},
		"writes nothing when this reconcile changed nothing": {
			storedState:   api.BackupStarting,
			observedState: api.BackupStarting,
			computedDest:  storedDest,
			wantStored:    storedDest,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			stored := incrBackup("backup-1",
				withBackupType(api.BackupTypeFull),
				withState(tt.storedState),
				withDestination(storedDest),
			)
			r := incrReconciler(t, stored)

			// What this reconcile thinks it read, and what it computed from it.
			// Built through the same helper as the stored backup so the two
			// cannot drift apart on a field the case says nothing about.
			observed := &incrBackup("backup-1",
				withBackupType(api.BackupTypeFull),
				withState(tt.observedState),
				withDestination(storedDest),
			).Status
			cr := incrBackup("backup-1",
				withBackupType(api.BackupTypeFull),
				withState(tt.observedState),
				withDestination(tt.computedDest),
			)

			err := r.writeStatus(t.Context(), cr, observed)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			persisted := new(api.PerconaXtraDBClusterBackup)
			require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), persisted))
			assert.Equal(t, tt.wantStored, persisted.Status.Destination.String())
		})
	}
}

// TestWriteStatusAdvancesObserved checks a second write in the same reconcile
// compares against what the first one wrote, not against what the reconcile
// originally read - otherwise the controller would reject its own writes.
func TestWriteStatusAdvancesObserved(t *testing.T) {
	stored := incrBackup("backup-1",
		withBackupType(api.BackupTypeFull),
		withState(api.BackupNew),
	)
	r := incrReconciler(t, stored)

	observed := stored.Status.DeepCopy()
	cr := stored.DeepCopy()

	cr.Status.Destination = "s3://bucket/prefix/cluster1-2026-11-03-10:00:00-full"
	require.NoError(t, r.writeStatus(t.Context(), cr, observed))
	assert.Equal(t, cr.Status.Destination, observed.Destination, "observed advances to what was written")

	// A later write in the same reconcile must still be accepted.
	cr.Status.SourcePod = "cluster1-pxc-1"
	require.NoError(t, r.writeStatus(t.Context(), cr, observed))

	persisted := new(api.PerconaXtraDBClusterBackup)
	require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), persisted))
	assert.Equal(t, "cluster1-pxc-1", persisted.Status.SourcePod)
	assert.Equal(t, cr.Status.Destination, persisted.Status.Destination)
}
