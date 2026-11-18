package physical

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/common"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/testutil"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	fakestorage "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/fake"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestValidate(t *testing.T) {
	const clusterName = "test-cluster"
	const namespace = "namespace"
	const backupName = clusterName + "-backup"
	const restoreName = clusterName + "-restore"
	const s3SecretName = "my-cluster-name-backup-s3"
	const azureSecretName = "azure-secret"

	cluster := testutil.ReadDefaultCR(t, clusterName, namespace)
	s3Bcp := testutil.ReadDefaultBackup(t, backupName, namespace)
	s3Bcp.Spec.StorageName = "s3-us-west"
	s3Bcp.Status.Destination.SetS3Destination("some-dest", "dest")
	s3Bcp.Status.S3 = &api.BackupStorageS3Spec{
		Bucket:            "some-bucket",
		CredentialsSecret: s3SecretName,
	}
	s3Bcp.Status.State = api.BackupSucceeded
	azureBcp := testutil.ReadDefaultBackup(t, backupName, namespace)
	azureBcp.Spec.StorageName = "azure-blob"
	azureBcp.Status.Destination.SetAzureDestination("some-dest", "dest")
	azureBcp.Status.Azure = &api.BackupStorageAzureSpec{
		ContainerPath:     "some-bucket",
		CredentialsSecret: azureSecretName,
	}
	azureBcp.Status.State = api.BackupSucceeded
	cr := testutil.ReadDefaultRestore(t, restoreName, namespace)
	cr.Spec.BackupName = backupName
	crSecret := testutil.ReadDefaultCRSecret(t, clusterName+"-secrets", namespace)
	s3Secret := testutil.ReadDefaultS3Secret(t, s3SecretName, namespace)
	azureSecret := testutil.ReadDefaultAzureSecret(t, azureSecretName, namespace)

	tests := []struct {
		name                  string
		cr                    *api.PerconaXtraDBClusterRestore
		bcp                   *api.PerconaXtraDBClusterBackup
		cluster               *api.PerconaXtraDBCluster
		objects               []runtime.Object
		expectedErr           string
		fakeStorageClientFunc storage.NewClientFunc
	}{
		{
			name:    "s3",
			cr:      cr.DeepCopy(),
			cluster: cluster.DeepCopy(),
			bcp:     s3Bcp,
			objects: []runtime.Object{
				crSecret,
				s3Secret,
			},
		},
		{
			name:        "s3 without secrets",
			cr:          cr.DeepCopy(),
			cluster:     cluster.DeepCopy(),
			bcp:         s3Bcp,
			expectedErr: "failed to validate job: secrets my-cluster-name-backup-s3, test-cluster-secrets not found",
		},
		{
			name: "s3 without credentialsSecret",
			cr:   cr.DeepCopy(),
			bcp:  s3Bcp,
			objects: []runtime.Object{
				crSecret,
				s3Secret,
			},
			cluster: testutil.UpdateResource(cluster, func(cluster *api.PerconaXtraDBCluster) {
				cluster.Spec.Backup.Storages["s3-us-west"].S3.CredentialsSecret = ""
			}),
			expectedErr: "",
		},
		{
			name:        "s3 with failing storage client",
			cr:          cr.DeepCopy(),
			cluster:     cluster.DeepCopy(),
			bcp:         s3Bcp,
			expectedErr: "failed to validate backup existence: failed to list objects: failListObjects",
			objects: []runtime.Object{
				crSecret,
				s3Secret,
			},
			fakeStorageClientFunc: func(_ context.Context, opts storage.Options) (storage.Storage, error) {
				return &testutil.FakeStorageClient{FailListObjects: true}, nil
			},
		},
		{
			name:    "s3 with prefixed destination",
			cr:      cr.DeepCopy(),
			cluster: cluster.DeepCopy(),
			bcp: testutil.UpdateResource(s3Bcp.DeepCopy(), func(bcp *api.PerconaXtraDBClusterBackup) {
				bcp.Status.Destination.SetS3Destination("some-dest/prefix", "dest")
			}),
			objects: []runtime.Object{
				crSecret,
				s3Secret,
			},
			fakeStorageClientFunc: func(_ context.Context, opts storage.Options) (storage.Storage, error) {
				return &testutil.FakeStorageClient{
					ExpectedListObjectsPath: "prefix/dest/",
					ListObjectsResponse:     []string{"some-dest/prefix/dest/xtrabackup.stream"},
				}, nil
			},
		},
		{
			name: "s3 without provided bucket",
			cr: testutil.UpdateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.BackupName = ""
				cr.Spec.BackupSource = &api.PXCBackupStatus{
					Destination: s3Bcp.Status.Destination,
					StorageType: api.BackupStorageS3,
					S3:          s3Bcp.Status.S3,
				}
				cr.Spec.BackupSource.S3.Bucket = ""
			},
			),
			cluster: cluster.DeepCopy(),
			objects: []runtime.Object{
				crSecret,
				s3Secret,
			},
		},
		{
			name:        "s3 with empty bucket",
			cr:          cr.DeepCopy(),
			cluster:     cluster.DeepCopy(),
			bcp:         s3Bcp,
			expectedErr: "failed to validate backup existence: backup not found",
			objects: []runtime.Object{
				crSecret,
				s3Secret,
			},
			fakeStorageClientFunc: func(_ context.Context, opts storage.Options) (storage.Storage, error) {
				return &testutil.FakeStorageClient{EmptyListObjects: true}, nil
			},
		},
		{
			name: "s3 pitr",
			bcp:  s3Bcp,
			cr: testutil.UpdateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.PITR = &api.PITR{
					BackupSource: &api.PXCBackupStatus{
						StorageName: s3Bcp.Spec.StorageName,
					},
				}
			}),
			cluster: testutil.UpdateResource(cluster, func(cluster *api.PerconaXtraDBCluster) {
				cluster.Spec.Backup.PITR = api.PITRSpec{
					Enabled:     true,
					StorageName: s3Bcp.Spec.StorageName,
				}
			}),
			objects: []runtime.Object{
				crSecret,
				s3Secret,
			},
		},
		{
			name:    "azure",
			bcp:     azureBcp,
			cr:      cr.DeepCopy(),
			cluster: cluster.DeepCopy(),
			objects: []runtime.Object{
				crSecret,
				azureSecret,
			},
		},
		{
			name: "azure with prefixed destination",
			bcp: testutil.UpdateResource(azureBcp.DeepCopy(), func(bcp *api.PerconaXtraDBClusterBackup) {
				bcp.Status.Destination.SetAzureDestination("some-dest/prefix", "dest")
			}),
			cr:      cr.DeepCopy(),
			cluster: cluster.DeepCopy(),
			objects: []runtime.Object{
				crSecret,
				azureSecret,
			},
			fakeStorageClientFunc: func(_ context.Context, opts storage.Options) (storage.Storage, error) {
				return &testutil.FakeStorageClient{
					ExpectedListObjectsPath: "prefix/dest/",
					ListObjectsResponse:     []string{"some-dest/prefix/dest/xtrabackup.stream"},
				}, nil
			},
		},
		{
			name:        "azure without secrets",
			cr:          cr.DeepCopy(),
			cluster:     cluster.DeepCopy(),
			bcp:         azureBcp,
			expectedErr: "failed to validate job: secrets azure-secret, test-cluster-secrets not found",
		},
		{
			name: "azure pitr",
			bcp:  azureBcp,
			cr: testutil.UpdateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.PITR = &api.PITR{
					BackupSource: &api.PXCBackupStatus{
						StorageName: azureBcp.Spec.StorageName,
					},
				}
			}),
			cluster: testutil.UpdateResource(cluster, func(cluster *api.PerconaXtraDBCluster) {
				cluster.Spec.Backup.PITR = api.PITRSpec{
					Enabled:     true,
					StorageName: azureBcp.Spec.StorageName,
				}
			}),
			objects: []runtime.Object{
				crSecret,
				azureSecret,
			},
		},
		{
			name:        "azure with failing storage client",
			cr:          cr.DeepCopy(),
			cluster:     cluster.DeepCopy(),
			bcp:         azureBcp,
			expectedErr: "failed to validate backup existence: list blobs: failListObjects",
			objects: []runtime.Object{
				crSecret,
				azureSecret,
			},
			fakeStorageClientFunc: func(_ context.Context, opts storage.Options) (storage.Storage, error) {
				return &testutil.FakeStorageClient{FailListObjects: true}, nil
			},
		},
		{
			name: "azure without provided bucket",
			cr: testutil.UpdateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.BackupName = ""
				cr.Spec.BackupSource = &api.PXCBackupStatus{
					Destination: azureBcp.Status.Destination,
					StorageType: api.BackupStorageAzure,
					Azure:       azureBcp.Status.Azure,
				}
				cr.Spec.BackupSource.Azure.ContainerPath = ""
			},
			),
			cluster: cluster.DeepCopy(),
			objects: []runtime.Object{
				crSecret,
				azureSecret,
			},
		},
		{
			name:        "azure with empty bucket",
			cr:          cr.DeepCopy(),
			cluster:     cluster.DeepCopy(),
			bcp:         azureBcp,
			expectedErr: "failed to validate backup existence: no backups found",
			objects: []runtime.Object{
				crSecret,
				azureSecret,
			},
			fakeStorageClientFunc: func(_ context.Context, opts storage.Options) (storage.Storage, error) {
				return &testutil.FakeStorageClient{EmptyListObjects: true}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.fakeStorageClientFunc == nil {
				tt.fakeStorageClientFunc = func(ctx context.Context, opts storage.Options) (storage.Storage, error) {
					defaultFakeClient, err := fakestorage.NewStorage(ctx, opts)
					if err != nil {
						return nil, err
					}
					return &testutil.FakeStorageClient{Storage: defaultFakeClient}, nil
				}
			}

			if err := tt.cr.CheckNsetDefaults(); err != nil {
				t.Fatal(err)
			}
			if err := tt.cluster.CheckNSetDefaults(new(version.ServerVersion), logf.FromContext(ctx)); err != nil {
				t.Fatal(err)
			}
			if tt.bcp != nil {
				tt.objects = append(tt.objects, tt.bcp)
			}
			tt.objects = append(tt.objects, tt.cr, tt.cluster)

			cl := testutil.BuildFakeClient(tt.objects...)

			bcp := tt.bcp
			if bcp == nil {
				// a restore from a backupSource: the backup is built from the restore
				bcp = &api.PerconaXtraDBClusterBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tt.cr.Name,
						Namespace: tt.cr.Namespace,
					},
					Spec: api.PXCBackupSpec{
						PXCCluster:  tt.cr.Spec.PXCCluster,
						StorageName: tt.cr.Spec.BackupSource.StorageName,
					},
					Status: *tt.cr.Spec.BackupSource.DeepCopy(),
				}
				bcp.Status.State = api.BackupSucceeded
			}

			restorer, err := getRestorer(ctx, common.Options{
				Client:           cl,
				Scheme:           cl.Scheme(),
				NewStorageClient: tt.fakeStorageClientFunc,
				CR:               tt.cr,
				Cluster:          tt.cluster,
				Backup:           bcp,
			})
			if err != nil {
				t.Fatal(err)
			}
			err = validate(ctx, restorer, tt.cr)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if errStr != tt.expectedErr {
				t.Fatal("expected err:", tt.expectedErr, "; got:", errStr)
			}
		})
	}
}
