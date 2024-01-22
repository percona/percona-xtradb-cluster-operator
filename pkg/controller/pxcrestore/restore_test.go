package pxcrestore

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	fakestorage "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/fake"
	"github.com/percona/percona-xtradb-cluster-operator/version"
	"github.com/pkg/errors"
)

func TestValidate(t *testing.T) {
	ctx := context.Background()

	const clusterName = "test-cluster"
	const namespace = "namespace"
	const backupName = clusterName + "-backup"
	const restoreName = clusterName + "-restore"
	const s3SecretName = "my-cluster-name-backup-s3"
	const azureSecretName = "azure-secret"

	cluster := readDefaultCR(t, clusterName, namespace)
	s3Bcp := readDefaultBackup(t, backupName, namespace)
	s3Bcp.Spec.StorageName = "s3-us-west"
	s3Bcp.Status.Destination.SetS3Destination("some-dest", "dest")
	s3Bcp.Status.S3 = &api.BackupStorageS3Spec{
		Bucket:            "some-bucket",
		CredentialsSecret: s3SecretName,
	}
	s3Bcp.Status.State = api.BackupSucceeded
	azureBcp := readDefaultBackup(t, backupName, namespace)
	azureBcp.Spec.StorageName = "azure-blob"
	azureBcp.Status.Destination.SetAzureDestination("some-dest", "dest")
	azureBcp.Status.Azure = &api.BackupStorageAzureSpec{
		ContainerPath:     "some-bucket",
		CredentialsSecret: azureSecretName,
	}
	azureBcp.Status.State = api.BackupSucceeded
	cr := readDefaultRestore(t, restoreName, namespace)
	cr.Spec.BackupName = backupName
	crSecret := readDefaultCRSecret(t, clusterName+"-secrets", namespace)
	s3Secret := readDefaultS3Secret(t, s3SecretName, namespace)
	azureSecret := readDefaultAzureSecret(t, azureSecretName, namespace)

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
			cluster: updateResource(cluster, func(cluster *api.PerconaXtraDBCluster) {
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
				return &fakeStorageClient{failListObjects: true}, nil
			},
		},
		{
			name: "s3 without provided bucket",
			cr: updateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
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
				return &fakeStorageClient{emptyListObjects: true}, nil
			},
		},
		{
			name: "s3 pitr",
			bcp:  s3Bcp,
			cr: updateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.PITR = &api.PITR{
					BackupSource: &api.PXCBackupStatus{
						StorageName: s3Bcp.Spec.StorageName,
					},
				}
			}),
			cluster: updateResource(cluster, func(cluster *api.PerconaXtraDBCluster) {
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
			name:        "azure without secrets",
			cr:          cr.DeepCopy(),
			cluster:     cluster.DeepCopy(),
			bcp:         azureBcp,
			expectedErr: "failed to validate job: secrets azure-secret, test-cluster-secrets not found",
		},
		{
			name: "azure pitr",
			bcp:  azureBcp,
			cr: updateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
				cr.Spec.PITR = &api.PITR{
					BackupSource: &api.PXCBackupStatus{
						StorageName: azureBcp.Spec.StorageName,
					},
				}
			}),
			cluster: updateResource(cluster, func(cluster *api.PerconaXtraDBCluster) {
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
				return &fakeStorageClient{failListObjects: true}, nil
			},
		},
		{
			name: "azure without provided bucket",
			cr: updateResource(cr, func(cr *api.PerconaXtraDBClusterRestore) {
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
				return &fakeStorageClient{emptyListObjects: true}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fakeStorageClientFunc == nil {
				tt.fakeStorageClientFunc = func(ctx context.Context, opts storage.Options) (storage.Storage, error) {
					defaultFakeClient, err := fakestorage.NewFakeClient(ctx, opts)
					if err != nil {
						return nil, err
					}
					return &fakeStorageClient{defaultFakeClient, false, false}, nil
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

			cl := buildFakeClient(tt.objects...)
			r := reconciler(cl)
			r.newStorageClientFunc = tt.fakeStorageClientFunc

			bcp, err := r.getBackup(ctx, tt.cr)
			if err != nil {
				t.Fatal(err)
			}
			err = r.validate(ctx, tt.cr, bcp, tt.cluster)
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

type fakeStorageClient struct {
	storage.Storage
	failListObjects  bool
	emptyListObjects bool
}

func (c *fakeStorageClient) ListObjects(_ context.Context, _ string) ([]string, error) {
	switch {
	case c.emptyListObjects:
		return nil, nil
	case c.failListObjects:
		return nil, errors.New("failListObjects")
	}
	return []string{"some-dest/backup1", "some-dest/backup2"}, nil
}
