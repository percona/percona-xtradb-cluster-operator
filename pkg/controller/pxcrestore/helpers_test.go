package pxcrestore

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/pxcrestore/testutil"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	fakestorage "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/fake"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func reconciler(cl client.Client) *ReconcilePerconaXtraDBClusterRestore {
	return &ReconcilePerconaXtraDBClusterRestore{
		client:               cl,
		scheme:               cl.Scheme(),
		newStorageClientFunc: fakestorage.NewStorage,
		serverVersion: &version.ServerVersion{
			Platform: version.PlatformKubernetes,
		},
	}
}

const (
	logicalTestCluster   = "test-cluster"
	logicalTestNamespace = "namespace"
	logicalTestBackup    = "test-backup"
	logicalTestRestore   = "test-restore"
	logicalTestS3Secret  = "my-cluster-name-backup-s3"

	logicalTestDestination = api.PXCBackupDestination("s3://some-bucket/test-cluster-2026-11-10-10:00:00-logical")
	// logicalTestListPath is the path the storage is listed with for logicalTestDestination.
	logicalTestListPath = "test-cluster-2026-11-10-10:00:00-logical/"

	gi = int64(1) << 30
)

// logicalFakeStorage is a storage that holds a fixed list of objects.
type logicalFakeStorage struct {
	storage.Storage

	objects []storage.ObjectInfo
	listErr error
	// listCalls counts the listings, when set
	listCalls *int
}

func (s *logicalFakeStorage) ListObjectInfos(_ context.Context, prefix string) ([]storage.ObjectInfo, error) {
	if s.listCalls != nil {
		*s.listCalls++
	}
	if s.listErr != nil {
		return nil, s.listErr
	}
	if prefix != logicalTestListPath {
		return nil, errors.Errorf("unexpected list path: got %s want %s", prefix, logicalTestListPath)
	}
	return s.objects, nil
}

func (s *logicalFakeStorage) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	objects, err := s.ListObjectInfos(ctx, prefix)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(objects))
	for _, object := range objects {
		names = append(names, object.Name)
	}
	return names, nil
}

func (s *logicalFakeStorage) GetPrefix() string { return "" }

func newLogicalFakeStorageFunc(s *logicalFakeStorage) storage.NewClientFunc {
	return func(context.Context, storage.Options) (storage.Storage, error) {
		return s, nil
	}
}

// logicalDumpObjects returns the objects of a complete logical backup of the given size.
func logicalDumpObjects(size int64) []storage.ObjectInfo {
	return []storage.ObjectInfo{
		{Name: logicalTestListPath + "metadata", Size: 1024},
		{Name: logicalTestListPath + "shop.orders.00000.sql", Size: size - 1024},
	}
}

func newLogicalRestoreTestCluster(t *testing.T) *api.PerconaXtraDBCluster {
	t.Helper()

	cluster := testutil.ReadDefaultCR(t, logicalTestCluster, logicalTestNamespace)
	require.NoError(t, cluster.CheckNSetDefaults(new(version.ServerVersion), logf.FromContext(t.Context())))
	cluster.Status.Status = api.AppStateReady
	cluster.Status.PXC.Status = api.AppStateReady

	return cluster
}

func newLogicalRestoreTestBackup(t *testing.T, backupType api.PXCBackupType, size int64) *api.PerconaXtraDBClusterBackup {
	t.Helper()

	bcp := testutil.ReadDefaultBackup(t, logicalTestBackup, logicalTestNamespace)
	bcp.Spec.PXCCluster = logicalTestCluster
	bcp.Spec.StorageName = "s3-us-west"
	bcp.Spec.Type = backupType
	bcp.Status.Type = backupType
	bcp.Status.Size = size
	bcp.Status.State = api.BackupSucceeded
	bcp.Status.StorageName = "s3-us-west"
	bcp.Status.StorageType = api.BackupStorageS3
	bcp.Status.Destination = logicalTestDestination
	bcp.Status.S3 = &api.BackupStorageS3Spec{
		Bucket:            "some-bucket",
		CredentialsSecret: logicalTestS3Secret,
	}

	return bcp
}

func newLogicalRestoreTestRestore(t *testing.T) *api.PerconaXtraDBClusterRestore {
	t.Helper()

	cr := testutil.ReadDefaultRestore(t, logicalTestRestore, logicalTestNamespace)
	cr.Spec.PXCCluster = logicalTestCluster
	cr.Spec.BackupName = logicalTestBackup

	return cr
}
