package pxcbackup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/features"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	storagemock "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/mock"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestValidateLogicalBackup(t *testing.T) {
	tests := map[string]struct {
		crVersion   string
		storageName string
		storageType pxcv1.BackupStorageType
		expectedErr string
	}{
		"s3 storage on a supported crVersion": {
			crVersion:   "1.21.0",
			storageName: "s3-us-west",
			storageType: pxcv1.BackupStorageS3,
		},
		"s3 storage on a newer crVersion": {
			crVersion:   "1.22.0",
			storageName: "s3-us-west",
			storageType: pxcv1.BackupStorageS3,
		},
		"crVersion is too old": {
			crVersion:   "1.20.0",
			storageName: "s3-us-west",
			storageType: pxcv1.BackupStorageS3,
			expectedErr: "logical backups require crVersion 1.21.0 or newer",
		},
		"azure storage": {
			crVersion:   "1.21.0",
			storageName: "az1",
			storageType: pxcv1.BackupStorageAzure,
			expectedErr: "logical backups are supported only on s3 storage, storage az1 has type azure",
		},
		"filesystem storage": {
			crVersion:   "1.21.0",
			storageName: "fs-pvc",
			storageType: pxcv1.BackupStorageFilesystem,
			expectedErr: "logical backups are supported only on s3 storage, storage fs-pvc has type filesystem",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cluster := &pxcv1.PerconaXtraDBCluster{
				Spec: pxcv1.PerconaXtraDBClusterSpec{CRVersion: tt.crVersion},
			}
			storageSpec := &pxcv1.BackupStorageSpec{Type: tt.storageType}

			err := validateLogicalBackup(cluster, tt.storageName, storageSpec)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestLogicalBackupSize(t *testing.T) {
	const destination = pxcv1.PXCBackupDestination("s3://bucket/prefix/cluster1-2026-11-10-10:00:00-logical")
	const expectedListPrefix = "cluster1-2026-11-10-10:00:00-logical/"

	tests := map[string]struct {
		objects      []storage.ObjectInfo
		listErr      error
		expectedSize int64
		expectedErr  string
	}{
		"sum of all objects": {
			objects: []storage.ObjectInfo{
				{Name: expectedListPrefix + "metadata", Size: 100},
				{Name: expectedListPrefix + "shop.orders.00000.sql", Size: 250},
				{Name: expectedListPrefix + "shop.orders.00001.sql", Size: 4096},
			},
			expectedSize: 4446,
		},
		"no objects": {
			objects:      nil,
			expectedSize: 0,
		},
		"listing fails": {
			listErr:     errors.New("access denied"),
			expectedErr: "list backup objects: access denied",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := storagemock.NewStorage(t)
			s.On("ListObjectInfos", mock.Anything, expectedListPrefix).Return(tt.objects, tt.listErr)

			size, err := logicalBackupSize(context.Background(), s, destination)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSize, size)
		})
	}
}

const (
	testNamespace   = "test-ns"
	testClusterName = "cluster1"
)

// newLogicalTestCluster returns a ready cluster with one storage of each type.
func newLogicalTestCluster(t *testing.T, crVersion string) *pxcv1.PerconaXtraDBCluster {
	t.Helper()

	cluster, err := readDefaultCR(testClusterName, testNamespace)
	require.NoError(t, err)

	cluster.Spec.CRVersion = crVersion
	cluster.Spec.Backup.Storages = map[string]*pxcv1.BackupStorageSpec{
		"s3-us-west": {
			Type: pxcv1.BackupStorageS3,
			S3: &pxcv1.BackupStorageS3Spec{
				Bucket:            "operator-testing/prefix",
				Region:            "us-west-1",
				CredentialsSecret: "s3-secret",
			},
		},
		"az1": {
			Type: pxcv1.BackupStorageAzure,
			Azure: &pxcv1.BackupStorageAzureSpec{
				ContainerPath:     "operator-testing",
				CredentialsSecret: "azure-secret",
			},
		},
		"fs-pvc": {
			Type: pxcv1.BackupStorageFilesystem,
			Volume: &pxcv1.VolumeSpec{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			},
		},
	}
	cluster.Status.Status = pxcv1.AppStateReady
	cluster.Status.PXC.Ready = cluster.Spec.PXC.Size

	return cluster
}

func newLogicalTestBackup(storageName string, backupType pxcv1.PXCBackupType) *pxcv1.PerconaXtraDBClusterBackup {
	return &pxcv1.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "backup1",
			Namespace:         testNamespace,
			CreationTimestamp: metav1.NewTime(time.Date(2026, time.November, 10, 10, 0, 0, 0, time.UTC)),
		},
		Spec: pxcv1.PXCBackupSpec{
			PXCCluster:  testClusterName,
			StorageName: storageName,
			Type:        backupType,
		},
	}
}

// buildLogicalTestClient returns a fake client that, unlike buildFakeClient,
// serves the status subresource of backups, so that status updates made by
// the reconciler are stored.
func buildLogicalTestClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(buildFakeClient().Scheme()).
		WithObjects(objs...).
		WithStatusSubresource(&pxcv1.PerconaXtraDBClusterBackup{}).
		Build()
}

func newLogicalTestReconciler(cl client.Client, newStorage storage.NewClientFunc) *ReconcilePerconaXtraDBClusterBackup {
	r := reconciler(cl)
	r.serverVersion = &version.ServerVersion{
		Platform: version.PlatformKubernetes,
		Info:     k8sversion.Info{},
	}
	if newStorage != nil {
		r.newStorageClientFunc = newStorage
	}
	return r
}

func TestReconcileBackupType(t *testing.T) {
	tests := map[string]struct {
		crVersion      string
		storageName    string
		backupType     pxcv1.PXCBackupType
		sidecarEnabled bool

		// expectations for a backup that is rejected
		expectedErr string

		// expectations for a backup that gets a job
		expectedCommand []string
		// expectedDestinationSuffix is what follows the creation time in the
		// destination. The time itself is formatted in the local time zone,
		// so it is computed in the test body.
		expectedDestinationSuffix string
		expectedType              pxcv1.PXCBackupType
	}{
		"full backup keeps the job and destination it had before": {
			crVersion:                 version.Version(),
			storageName:               "s3-us-west",
			backupType:                pxcv1.BackupTypeFull,
			expectedCommand:           []string{"bash", "/opt/percona/backup/backup.sh"},
			expectedDestinationSuffix: "-full",
			expectedType:              pxcv1.BackupTypeFull,
		},
		"backup without a type is a full backup": {
			crVersion:                 version.Version(),
			storageName:               "s3-us-west",
			backupType:                "",
			expectedCommand:           []string{"bash", "/opt/percona/backup/backup.sh"},
			expectedDestinationSuffix: "-full",
			expectedType:              pxcv1.BackupTypeFull,
		},
		"logical backup on s3": {
			crVersion:                 version.Version(),
			storageName:               "s3-us-west",
			backupType:                pxcv1.BackupTypeLogical,
			expectedCommand:           []string{"bash", "/opt/percona/backup/logical-backup.sh"},
			expectedDestinationSuffix: "-logical",
			expectedType:              pxcv1.BackupTypeLogical,
		},
		"logical backup ignores the xtrabackup sidecar feature gate": {
			crVersion:                 version.Version(),
			storageName:               "s3-us-west",
			backupType:                pxcv1.BackupTypeLogical,
			sidecarEnabled:            true,
			expectedCommand:           []string{"bash", "/opt/percona/backup/logical-backup.sh"},
			expectedDestinationSuffix: "-logical",
			expectedType:              pxcv1.BackupTypeLogical,
		},
		"logical backup on azure is rejected": {
			crVersion:   version.Version(),
			storageName: "az1",
			backupType:  pxcv1.BackupTypeLogical,
			expectedErr: "logical backups are supported only on s3 storage, storage az1 has type azure",
		},
		"logical backup on a pvc is rejected": {
			crVersion:   version.Version(),
			storageName: "fs-pvc",
			backupType:  pxcv1.BackupTypeLogical,
			expectedErr: "logical backups are supported only on s3 storage, storage fs-pvc has type filesystem",
		},
		"logical backup on an old crVersion is rejected": {
			crVersion:   "1.20.0",
			storageName: "s3-us-west",
			backupType:  pxcv1.BackupTypeLogical,
			expectedErr: "logical backups require crVersion 1.21.0 or newer",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if tt.sidecarEnabled {
				gate := features.NewGate()
				require.NoError(t, gate.Set(string(features.XtrabackupSidecar)+"=true"))
				ctx = features.NewContextWithGate(ctx, gate)
			}

			cluster := newLogicalTestCluster(t, tt.crVersion)
			cr := newLogicalTestBackup(tt.storageName, tt.backupType)
			cl := buildLogicalTestClient(cluster, cr)
			r := newLogicalTestReconciler(cl, nil)

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

			reconciled := new(pxcv1.PerconaXtraDBClusterBackup)
			require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(cr), reconciled))

			jobs := new(batchv1.JobList)
			require.NoError(t, cl.List(ctx, jobs, client.InNamespace(testNamespace)))

			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				assert.Equal(t, pxcv1.BackupFailed, reconciled.Status.State)
				assert.Equal(t, tt.expectedErr, reconciled.Status.Error)
				assert.Empty(t, jobs.Items, "a rejected backup must not create a job")
				return
			}

			require.NoError(t, err)
			require.Len(t, jobs.Items, 1)
			job := jobs.Items[0]
			assert.Equal(t, naming.BackupJobName(cr.Name), job.Name)
			require.Len(t, job.Spec.Template.Spec.Containers, 1)
			container := job.Spec.Template.Spec.Containers[0]
			assert.Equal(t, tt.expectedCommand, container.Command)
			assert.Contains(t, container.Env, corev1.EnvVar{Name: "S3_BUCKET", Value: "operator-testing"})
			assert.Contains(t, container.Env, corev1.EnvVar{
				Name:  "S3_BUCKET_PATH",
				Value: reconciled.Status.Destination.PathWithoutBucket(),
			})
			for _, env := range container.Env {
				assert.NotEqual(t, "BACKUP_DEST", env.Name, "BACKUP_DEST is only for the xtrabackup sidecar")
			}

			expectedDestination := pxcv1.PXCBackupDestination("s3://operator-testing/prefix/" + testClusterName + "-" +
				reconciled.CreationTimestamp.Time.Format("2006-01-02-15:04:05") + tt.expectedDestinationSuffix)
			assert.Equal(t, expectedDestination, reconciled.Status.Destination)
			assert.Equal(t, tt.expectedType, reconciled.Status.Type)
			assert.Zero(t, reconciled.Status.Size)
		})
	}
}

func TestUpdateJobStatusLogical(t *testing.T) {
	const destination = pxcv1.PXCBackupDestination("s3://operator-testing/prefix/cluster1-2026-11-10-10:00:00-logical")

	objects := []storage.ObjectInfo{
		{Name: "cluster1-2026-11-10-10:00:00-logical/metadata", Size: 100},
		{Name: "cluster1-2026-11-10-10:00:00-logical/shop.orders.00000.sql", Size: 250},
		{Name: "cluster1-2026-11-10-10:00:00-logical/shop.orders.00001.sql", Size: 4096},
	}

	completeJob := batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
	}
	failedJob := batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}},
	}

	tests := map[string]struct {
		backupType pxcv1.PXCBackupType
		jobStatus  batchv1.JobStatus
		// newStorage is the storage client constructor the reconciler uses.
		// nil means the storage must not be used at all.
		newStorage func(t *testing.T) storage.NewClientFunc

		expectedState pxcv1.PXCBackupState
		expectedType  pxcv1.PXCBackupType
		expectedSize  int64
	}{
		"logical backup succeeded: size is the sum of its objects": {
			backupType: pxcv1.BackupTypeLogical,
			jobStatus:  completeJob,
			newStorage: func(t *testing.T) storage.NewClientFunc {
				s := storagemock.NewStorage(t)
				s.On("ListObjectInfos", mock.Anything, "cluster1-2026-11-10-10:00:00-logical/").Return(objects, nil)
				return func(context.Context, storage.Options) (storage.Storage, error) { return s, nil }
			},
			expectedState: pxcv1.BackupSucceeded,
			expectedType:  pxcv1.BackupTypeLogical,
			expectedSize:  4446,
		},
		"logical backup succeeded but the bucket can't be listed: still succeeded, size unknown": {
			backupType: pxcv1.BackupTypeLogical,
			jobStatus:  completeJob,
			newStorage: func(t *testing.T) storage.NewClientFunc {
				s := storagemock.NewStorage(t)
				s.On("ListObjectInfos", mock.Anything, mock.Anything).Return(nil, errors.New("access denied"))
				return func(context.Context, storage.Options) (storage.Storage, error) { return s, nil }
			},
			expectedState: pxcv1.BackupSucceeded,
			expectedType:  pxcv1.BackupTypeLogical,
			expectedSize:  0,
		},
		"logical backup succeeded but no storage client can be created: still succeeded, size unknown": {
			backupType: pxcv1.BackupTypeLogical,
			jobStatus:  completeJob,
			newStorage: func(t *testing.T) storage.NewClientFunc {
				return func(context.Context, storage.Options) (storage.Storage, error) {
					return nil, errors.New("no credentials")
				}
			},
			expectedState: pxcv1.BackupSucceeded,
			expectedType:  pxcv1.BackupTypeLogical,
			expectedSize:  0,
		},
		"logical backup failed: storage is not used": {
			backupType:    pxcv1.BackupTypeLogical,
			jobStatus:     failedJob,
			expectedState: pxcv1.BackupFailed,
			expectedType:  pxcv1.BackupTypeLogical,
			expectedSize:  0,
		},
		"logical backup still starting: type is kept, storage is not used": {
			backupType:    pxcv1.BackupTypeLogical,
			jobStatus:     batchv1.JobStatus{},
			expectedState: pxcv1.BackupStarting,
			expectedType:  pxcv1.BackupTypeLogical,
			expectedSize:  0,
		},
		"full backup succeeded: type is kept, storage is not used": {
			backupType:    pxcv1.BackupTypeFull,
			jobStatus:     completeJob,
			expectedState: pxcv1.BackupSucceeded,
			expectedType:  pxcv1.BackupTypeFull,
			expectedSize:  0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			cluster := newLogicalTestCluster(t, version.Version())
			storageSpec := cluster.Spec.Backup.Storages["s3-us-west"]

			cr := newLogicalTestBackup("s3-us-west", tt.backupType)
			cr.Status.Type = tt.backupType
			cr.Status.Destination = destination
			cr.Status.S3 = storageSpec.S3
			cr.Status.StorageType = storageSpec.Type

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      naming.BackupJobName(cr.Name),
					Namespace: testNamespace,
				},
				Status: tt.jobStatus,
			}

			newStorage := func(context.Context, storage.Options) (storage.Storage, error) {
				t.Error("storage must not be used")
				return nil, errors.New("storage must not be used")
			}
			if tt.newStorage != nil {
				newStorage = tt.newStorage(t)
			}

			cl := buildLogicalTestClient(cluster, cr, job)
			r := newLogicalTestReconciler(cl, newStorage)

			err := r.updateJobStatus(ctx, cr, job, cr.Spec.StorageName, storageSpec, cluster)
			require.NoError(t, err)

			updated := new(pxcv1.PerconaXtraDBClusterBackup)
			require.NoError(t, cl.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, updated))

			assert.Equal(t, tt.expectedState, updated.Status.State)
			assert.Equal(t, tt.expectedType, updated.Status.Type)
			assert.Equal(t, tt.expectedSize, updated.Status.Size)
			assert.Equal(t, destination, updated.Status.Destination)
		})
	}
}

// TestUpdateJobStatusPITRBookkeeping checks which succeeded backups reset the
// binlog collector's gap tracking. The cluster has PITR enabled and no binlog
// collector pod, so a backup that takes part in the bookkeeping fails when it
// looks the pod up, and one that is skipped does not.
func TestUpdateJobStatusPITRBookkeeping(t *testing.T) {
	tests := map[string]struct {
		pitrEnabled bool
		backupType  pxcv1.PXCBackupType
		expectedErr string
	}{
		"full backup with pitr enabled resets gap tracking": {
			pitrEnabled: true,
			backupType:  pxcv1.BackupTypeFull,
			expectedErr: "get binlog collector pod: no binlog collector pods",
		},
		"backup without a type with pitr enabled resets gap tracking": {
			pitrEnabled: true,
			backupType:  "",
			expectedErr: "get binlog collector pod: no binlog collector pods",
		},
		"logical backup with pitr enabled leaves gap tracking alone": {
			pitrEnabled: true,
			backupType:  pxcv1.BackupTypeLogical,
		},
		"full backup with pitr disabled leaves gap tracking alone": {
			pitrEnabled: false,
			backupType:  pxcv1.BackupTypeFull,
		},
		"logical backup with pitr disabled leaves gap tracking alone": {
			pitrEnabled: false,
			backupType:  pxcv1.BackupTypeLogical,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			cluster := newLogicalTestCluster(t, version.Version())
			cluster.Spec.Backup.PITR.Enabled = tt.pitrEnabled
			storageSpec := cluster.Spec.Backup.Storages["s3-us-west"]

			cr := newLogicalTestBackup("s3-us-west", tt.backupType)
			cr.Status.Type = tt.backupType
			cr.Status.Destination = "s3://operator-testing/prefix/backup1"
			cr.Status.S3 = storageSpec.S3
			cr.Status.StorageType = storageSpec.Type

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      naming.BackupJobName(cr.Name),
					Namespace: testNamespace,
				},
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
				},
			}

			cl := buildLogicalTestClient(cluster, cr, job)
			r := newLogicalTestReconciler(cl, nil)

			err := r.updateJobStatus(ctx, cr, job, cr.Spec.StorageName, storageSpec, cluster)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)

			updated := new(pxcv1.PerconaXtraDBClusterBackup)
			require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(cr), updated))
			assert.Equal(t, pxcv1.BackupSucceeded, updated.Status.State)
		})
	}
}
