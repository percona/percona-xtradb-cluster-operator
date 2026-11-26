package pxcrestore

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/testutil"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	fakestorage "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/fake"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

// TestOperatorRestart checks that the operator can catch up with the restore process after a restart.
// This test is run for each restore state. It runs reconcile twice for each state. Each reconcile operator should change the restore state.
// This test helps to eliminate errors such as creating an existing Pod without handling the AlreadyExists error.
func TestOperatorRestart(t *testing.T) {
	ctx := context.Background()

	const clusterName = "test-cluster"
	const namespace = "namespace"
	const backupName = clusterName + "-backup"
	const restoreName = clusterName + "-restore"
	const s3SecretName = "my-cluster-name-backup-s3"
	const azureSecretName = "my-cluster-name-backup-azure"

	states := []api.RestoreState{
		api.RestoreNew,
		api.RestoreStopCluster,
		api.RestoreRestore,
		api.RestoreStartCluster,
		api.RestorePITR,
	}

	bcp := testutil.ReadDefaultBackup(t, backupName, namespace)
	crSecret := testutil.ReadDefaultCRSecret(t, clusterName+"-secrets", namespace)
	cluster := testutil.ReadDefaultCR(t, clusterName, namespace)
	if err := cluster.CheckNSetDefaults(new(version.ServerVersion), logf.FromContext(ctx)); err != nil {
		t.Fatal(err)
	}
	cluster.Status.PXC.Status = api.AppStateReady
	cr := testutil.ReadDefaultRestore(t, restoreName, namespace)
	cr.Spec.BackupName = backupName
	cr.Spec.PXCCluster = clusterName

	tests := []struct {
		name    string
		bcp     *api.PerconaXtraDBClusterBackup
		objects []runtime.Object
	}{
		{
			name: "s3",
			bcp: testutil.UpdateResource(bcp, func(bcp *api.PerconaXtraDBClusterBackup) {
				bcp.Status.State = api.BackupSucceeded
				bcp.Status.Destination.SetS3Destination("some-dest", "dest")
				bcp.Spec.StorageName = "s3-us-west"
				bcp.Status.S3 = &api.BackupStorageS3Spec{
					Bucket:            "some-bucket",
					CredentialsSecret: s3SecretName,
				}
			}),
			objects: []runtime.Object{testutil.ReadDefaultS3Secret(t, s3SecretName, namespace)},
		},
		{
			name: "azure",
			bcp: testutil.UpdateResource(bcp, func(bcp *api.PerconaXtraDBClusterBackup) {
				bcp.Status.State = api.BackupSucceeded
				bcp.Status.Destination.SetAzureDestination("some-dest", "dest")
				bcp.Spec.StorageName = "azure-blob"
				bcp.Status.Azure = &api.BackupStorageAzureSpec{
					ContainerPath:     "some-bucket",
					CredentialsSecret: azureSecretName,
				}
			}),
			objects: []runtime.Object{
				testutil.UpdateResource(testutil.ReadDefaultS3Secret(t, azureSecretName, namespace), func(secret *corev1.Secret) {
					secret.Data = map[string][]byte{
						"AZURE_STORAGE_ACCOUNT_NAME": []byte("some-account"),
						"AZURE_STORAGE_ACCOUNT_KEY":  []byte("some-key"),
					}
				}),
			},
		},
		{
			name: "pvc",
			bcp: testutil.UpdateResource(bcp, func(bcp *api.PerconaXtraDBClusterBackup) {
				bcp.Status.State = api.BackupSucceeded
				bcp.Status.Destination.SetPVCDestination("some-dest")
				bcp.Status.StorageType = api.BackupStorageFilesystem
			}),
			objects: []runtime.Object{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster,
						Namespace: cr.Namespace,
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster + "-verify",
						Namespace: cr.Namespace,
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodSucceeded,
					},
				},
				backup.PVCRestoreService(cr, cluster),
			},
		},
	}

	for _, tt := range tests {
		for _, state := range states {
			if tt.bcp.Status.StorageType == api.BackupStorageFilesystem && state == api.RestorePITR {
				continue
			}
			t.Run(tt.name+" state "+string(state), func(t *testing.T) {
				cr := cr.DeepCopy()
				cluster := cluster.DeepCopy()
				if state == api.RestorePITR {
					cr.Spec.PITR = &api.PITR{
						BackupSource: &api.PXCBackupStatus{
							StorageName: tt.bcp.Spec.StorageName,
						},
					}
				}
				cr.Status.State = state
				objects := append(tt.objects, tt.bcp, cr, cluster, crSecret, &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc1",
						Namespace: namespace,
						Labels:    statefulset.NewNode(cluster).Labels(),
					},
				})
				cl := testutil.BuildFakeClient(objects...)

				r := reconciler(cl)
				r.newStorageClientFunc = func(ctx context.Context, opts storage.Options) (storage.Storage, error) {
					defaultFakeClient, err := fakestorage.NewStorage(ctx, opts)
					if err != nil {
						return nil, err
					}
					return &testutil.FakeStorageClient{Storage: defaultFakeClient}, nil
				}

				if state == api.RestoreRestore || state == api.RestorePITR {
					// the job the restore is waiting for in this state
					destination := tt.bcp.Status.Destination
					if tt.bcp.Status.StorageType == api.BackupStorageFilesystem {
						destination = ""
					}
					job, err := backup.RestoreJob(ctx, cr, tt.bcp, cluster, cluster.Spec.InitImage, cl.Scheme(), destination, state == api.RestorePITR)
					if err != nil {
						t.Fatal(err)
					}
					job.Status.Conditions = []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					}
					if err := r.client.Create(ctx, job); err != nil {
						t.Fatal(err)
					}
				}

				nn := types.NamespacedName{
					Name:      cr.Name,
					Namespace: cr.Namespace,
				}
				req := reconcile.Request{
					NamespacedName: nn,
				}

				_, err := r.Reconcile(ctx, req)
				if err != nil {
					t.Fatal(err)
				}

				restore := new(api.PerconaXtraDBClusterRestore)
				if err := r.client.Get(ctx, nn, restore); err != nil {
					t.Fatal(err)
				}
				if restore.Status.State == state {
					t.Fatal("state not changed")
				}

				// Assuming that the operator restarted just before the status update
				restore.Status.State = state
				if err := r.client.Status().Update(ctx, restore); err != nil {
					t.Fatal(err)
				}
				_, err = r.Reconcile(ctx, req)
				if err != nil {
					t.Fatal(err)
				}

				if err := r.client.Get(ctx, nn, restore); err != nil {
					t.Fatal(err)
				}
				if restore.Status.State == state {
					t.Fatal("state not changed")
				}
			})
		}
	}
}

func TestReconcileLogicalRestore(t *testing.T) {
	completeJob := batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
	}
	failedJob := batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "BackoffLimitExceeded"}},
	}

	// restoreJob returns the restore job as it exists while a restore is running.
	restoreJob := func(t *testing.T, status batchv1.JobStatus) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      naming.RestoreJobName(newLogicalRestoreTestRestore(t), false),
				Namespace: logicalTestNamespace,
			},
			Status: status,
		}
	}

	tests := map[string]struct {
		// what exists before the reconcile
		cr           func(cr *api.PerconaXtraDBClusterRestore)
		cluster      func(cluster *api.PerconaXtraDBCluster)
		backupType   api.PXCBackupType
		backupSize   int64
		storage      *logicalFakeStorage
		extraObjects func(t *testing.T) []runtime.Object
		noS3Secret   bool

		// what is expected after one reconcile
		expectedState      api.RestoreState
		expectedComments   string
		expectedBackupType api.PXCBackupType
		// expectedVolumeSize is the size of the job's scratch volume.
		// Empty means no job must have been created by this reconcile.
		expectedVolumeSize string
	}{
		"new restore creates the job, sized from the backup's recorded size": {
			backupType:         api.BackupTypeLogical,
			backupSize:         10 * gi,
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(10 * gi)},
			expectedState:      api.RestoreRestore,
			expectedBackupType: api.BackupTypeLogical,
			expectedVolumeSize: "11Gi",
		},
		"new restore of a backup with no recorded size is sized from the bucket": {
			backupType:         api.BackupTypeLogical,
			backupSize:         0,
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(20 * gi)},
			expectedState:      api.RestoreRestore,
			expectedBackupType: api.BackupTypeLogical,
			expectedVolumeSize: "22Gi",
		},
		"new restore with a volume size override uses it": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.Job = &api.RestoreJobSpec{
					Volume: &api.RestoreJobVolume{Size: new(resource.MustParse("5Gi"))},
				}
			},
			backupType:         api.BackupTypeLogical,
			backupSize:         10 * gi,
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(10 * gi)},
			expectedState:      api.RestoreRestore,
			expectedBackupType: api.BackupTypeLogical,
			expectedVolumeSize: "5Gi",
		},
		"new restore from a backupSource without a type that points at a logical dump": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.BackupName = ""
				cr.Spec.BackupSource = &api.PXCBackupStatus{
					Destination: logicalTestDestination,
					StorageType: api.BackupStorageS3,
					S3: &api.BackupStorageS3Spec{
						Bucket:            "some-bucket",
						CredentialsSecret: logicalTestS3Secret,
					},
				}
			},
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(2 * gi)},
			expectedState:      api.RestoreRestore,
			expectedBackupType: api.BackupTypeLogical,
			expectedVolumeSize: "2362232013", // 2Gi plus 10%, rounded up
		},
		"new restore from a backupSource without a type waits while the bucket can't be listed": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.BackupName = ""
				cr.Spec.BackupSource = &api.PXCBackupStatus{
					Destination: logicalTestDestination,
					StorageType: api.BackupStorageS3,
					S3: &api.BackupStorageS3Spec{
						Bucket:            "some-bucket",
						CredentialsSecret: logicalTestS3Secret,
					},
				}
			},
			storage:            &logicalFakeStorage{listErr: errors.New("connection refused")},
			expectedState:      api.RestoreStarting,
			expectedBackupType: "",
		},
		"point-in-time recovery is rejected": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.PITR = &api.PITR{Type: "latest", BackupSource: &api.PXCBackupStatus{StorageName: "s3-us-west"}}
			},
			backupType:         api.BackupTypeLogical,
			backupSize:         gi,
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(gi)},
			expectedState:      api.RestoreFailed,
			expectedComments:   "point-in-time recovery is not supported for logical backups",
			expectedBackupType: api.BackupTypeLogical,
		},
		"cluster that is not ready is rejected": {
			cluster: func(cluster *api.PerconaXtraDBCluster) {
				cluster.Status.Status = api.AppStateInit
			},
			backupType:         api.BackupTypeLogical,
			backupSize:         gi,
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(gi)},
			expectedState:      api.RestoreFailed,
			expectedComments:   "cluster must be ready for a logical restore, current status: initializing",
			expectedBackupType: api.BackupTypeLogical,
		},
		"old crVersion is rejected": {
			cluster: func(cluster *api.PerconaXtraDBCluster) {
				cluster.Spec.CRVersion = "1.20.0"
			},
			backupType:         api.BackupTypeLogical,
			backupSize:         gi,
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(gi)},
			expectedState:      api.RestoreFailed,
			expectedComments:   "logical restores require crVersion 1.21.0 or newer",
			expectedBackupType: api.BackupTypeLogical,
		},
		"backup without a metadata object is rejected": {
			backupType: api.BackupTypeLogical,
			backupSize: gi,
			storage: &logicalFakeStorage{objects: []storage.ObjectInfo{
				{Name: logicalTestListPath + "shop.orders.00000.sql", Size: gi},
			}},
			expectedState:      api.RestoreFailed,
			expectedComments:   "backup not found or incomplete: no metadata object in " + string(logicalTestDestination),
			expectedBackupType: api.BackupTypeLogical,
		},
		"backup that is gone from the bucket is rejected": {
			backupType:         api.BackupTypeLogical,
			backupSize:         gi,
			storage:            &logicalFakeStorage{},
			expectedState:      api.RestoreFailed,
			expectedComments:   "backup not found or incomplete: no metadata object in " + string(logicalTestDestination),
			expectedBackupType: api.BackupTypeLogical,
		},
		"bucket that can't be listed is rejected": {
			backupType:         api.BackupTypeLogical,
			backupSize:         gi,
			storage:            &logicalFakeStorage{listErr: errors.New("access denied")},
			expectedState:      api.RestoreFailed,
			expectedComments:   "failed to validate backup existence: list backup objects: access denied",
			expectedBackupType: api.BackupTypeLogical,
		},
		"missing credentials secret is rejected": {
			backupType:         api.BackupTypeLogical,
			backupSize:         gi,
			storage:            &logicalFakeStorage{objects: logicalDumpObjects(gi)},
			noS3Secret:         true,
			expectedState:      api.RestoreFailed,
			expectedComments:   "failed to validate job: secrets my-cluster-name-backup-s3 not found",
			expectedBackupType: api.BackupTypeLogical,
		},
		"physical restore with job.volume is rejected": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.Job = &api.RestoreJobSpec{
					Volume: &api.RestoreJobVolume{Size: new(resource.MustParse("5Gi"))},
				}
			},
			backupType:         api.BackupTypeFull,
			storage:            &logicalFakeStorage{},
			expectedState:      api.RestoreFailed,
			expectedComments:   "job.volume is only supported for logical restores",
			expectedBackupType: api.BackupTypeFull,
		},
		"backup taken before the type existed is restored as a physical backup": {
			backupType: "",
			storage: &logicalFakeStorage{objects: []storage.ObjectInfo{
				{Name: logicalTestListPath + "xtrabackup.stream.00000000000000000000", Size: gi},
			}},
			expectedState:      api.RestoreStopCluster,
			expectedBackupType: api.BackupTypeFull,
		},
		"running restore keeps waiting for its job": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Status.State = api.RestoreRestore
				cr.Status.BackupType = api.BackupTypeLogical
			},
			backupType: api.BackupTypeLogical,
			storage:    &logicalFakeStorage{},
			extraObjects: func(t *testing.T) []runtime.Object {
				return []runtime.Object{restoreJob(t, batchv1.JobStatus{})}
			},
			expectedState:      api.RestoreRestore,
			expectedBackupType: api.BackupTypeLogical,
		},
		"running restore succeeds when its job completes": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Status.State = api.RestoreRestore
				cr.Status.BackupType = api.BackupTypeLogical
			},
			backupType: api.BackupTypeLogical,
			storage:    &logicalFakeStorage{},
			extraObjects: func(t *testing.T) []runtime.Object {
				return []runtime.Object{restoreJob(t, completeJob)}
			},
			expectedState:      api.RestoreSucceeded,
			expectedBackupType: api.BackupTypeLogical,
		},
		"running restore fails when its job fails": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Status.State = api.RestoreRestore
				cr.Status.BackupType = api.BackupTypeLogical
			},
			backupType: api.BackupTypeLogical,
			storage:    &logicalFakeStorage{},
			extraObjects: func(t *testing.T) []runtime.Object {
				return []runtime.Object{restoreJob(t, failedJob)}
			},
			expectedState:      api.RestoreFailed,
			expectedComments:   "job restore-job-test-restore-test-cluster failed: BackoffLimitExceeded",
			expectedBackupType: api.BackupTypeLogical,
		},
		"running logical restore stays logical when the bucket no longer has a metadata object": {
			cr: func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.BackupName = ""
				cr.Spec.BackupSource = &api.PXCBackupStatus{
					Destination: logicalTestDestination,
					StorageType: api.BackupStorageS3,
					S3: &api.BackupStorageS3Spec{
						Bucket:            "some-bucket",
						CredentialsSecret: logicalTestS3Secret,
					},
				}
				cr.Status.State = api.RestoreRestore
				cr.Status.BackupType = api.BackupTypeLogical
			},
			storage: &logicalFakeStorage{},
			extraObjects: func(t *testing.T) []runtime.Object {
				return []runtime.Object{restoreJob(t, completeJob)}
			},
			expectedState:      api.RestoreSucceeded,
			expectedBackupType: api.BackupTypeLogical,
		},
		"second restore on the same cluster is rejected while a logical one is running": {
			backupType: api.BackupTypeLogical,
			backupSize: gi,
			storage:    &logicalFakeStorage{objects: logicalDumpObjects(gi)},
			extraObjects: func(t *testing.T) []runtime.Object {
				other := newLogicalRestoreTestRestore(t)
				other.Name = "other-restore"
				other.Status.State = api.RestoreRestore
				other.Status.BackupType = api.BackupTypeLogical
				return []runtime.Object{other}
			},
			expectedState:    api.RestoreFailed,
			expectedComments: "unable to continue, concurrent restore job other-restore running now",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			cr := newLogicalRestoreTestRestore(t)
			if tt.cr != nil {
				tt.cr(cr)
			}
			cluster := newLogicalRestoreTestCluster(t)
			if tt.cluster != nil {
				tt.cluster(cluster)
			}
			clusterBefore := cluster.DeepCopy()

			objects := []runtime.Object{
				cr,
				cluster,
				testutil.ReadDefaultCRSecret(t, logicalTestCluster+"-secrets", logicalTestNamespace),
			}
			if !tt.noS3Secret {
				objects = append(objects, testutil.ReadDefaultS3Secret(t, logicalTestS3Secret, logicalTestNamespace))
			}
			if cr.Spec.BackupSource == nil {
				objects = append(objects, newLogicalRestoreTestBackup(t, tt.backupType, tt.backupSize))
			}
			if tt.extraObjects != nil {
				objects = append(objects, tt.extraObjects(t)...)
			}
			hadJob := false
			for _, object := range objects {
				if _, ok := object.(*batchv1.Job); ok {
					hadJob = true
				}
			}

			cl := testutil.BuildFakeClient(objects...)
			r := reconciler(cl)
			r.newStorageClientFunc = newLogicalFakeStorageFunc(tt.storage)

			// the reconciler reports failures through the restore status, so its error is not checked here
			_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cr)})

			reconciled := new(api.PerconaXtraDBClusterRestore)
			require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(cr), reconciled))
			assert.Equal(t, tt.expectedState, reconciled.Status.State)
			assert.Equal(t, tt.expectedComments, reconciled.Status.Comments)
			assert.Equal(t, tt.expectedBackupType, reconciled.Status.BackupType)

			// a logical restore must never touch the cluster
			if tt.expectedState != api.RestoreStopCluster {
				clusterAfter := new(api.PerconaXtraDBCluster)
				require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(cluster), clusterAfter))
				assert.Equal(t, clusterBefore.Spec, clusterAfter.Spec)
				assert.False(t, clusterAfter.Spec.Pause)
			}

			jobs := new(batchv1.JobList)
			require.NoError(t, cl.List(ctx, jobs, client.InNamespace(logicalTestNamespace)))
			if tt.expectedVolumeSize == "" {
				if !hadJob {
					assert.Empty(t, jobs.Items, "no job must be created")
				}
				return
			}

			require.Len(t, jobs.Items, 1)
			job := jobs.Items[0]
			assert.Equal(t, naming.RestoreJobName(cr, false), job.Name)
			assert.Equal(t, []string{"bash", "/opt/percona/backup/logical-restore.sh"}, job.Spec.Template.Spec.Containers[0].Command)

			var requested *resource.Quantity
			for _, volume := range job.Spec.Template.Spec.Volumes {
				if volume.Ephemeral != nil {
					requested = new(volume.Ephemeral.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage])
				}
			}
			require.NotNil(t, requested, "the job has no scratch volume")
			expected := resource.MustParse(tt.expectedVolumeSize)
			assert.Zero(t, expected.Cmp(*requested), "expected %s, got %s", expected.String(), requested.String())
		})
	}
}
