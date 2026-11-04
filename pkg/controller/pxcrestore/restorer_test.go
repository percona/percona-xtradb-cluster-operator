package pxcrestore

import (
	"context"
	"path"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	mockstorage "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage/mock"
)

func TestValidatePITRTarget(t *testing.T) {
	mockBackup := &api.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-backup",
		},
		Spec: api.PXCBackupSpec{
			PXCCluster: "test-cluster",
		},
		Status: api.PXCBackupStatus{
			State: api.BackupSucceeded,
		},
	}

	mockRestore := &api.PerconaXtraDBClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-restore",
		},
		Spec: api.PerconaXtraDBClusterRestoreSpec{},
	}

	testCases := []struct {
		desc        string
		backup      *api.PerconaXtraDBClusterBackup
		restore     *api.PerconaXtraDBClusterRestore
		expectedErr error
	}{
		{
			desc:        "PITR disabled",
			backup:      mockBackup,
			restore:     mockRestore,
			expectedErr: nil,
		},
		{
			desc: "LatestRestorableTime not known",
			backup: func() *api.PerconaXtraDBClusterBackup {
				backup := mockBackup.DeepCopy()
				backup.Status.LatestRestorableTime = nil
				return backup
			}(),
			restore: func() *api.PerconaXtraDBClusterRestore {
				restore := mockRestore.DeepCopy()
				restore.Spec.PITR = &api.PITR{
					Type: api.PITRTypeDate,
					Date: "2021-01-01 00:00:00",
				}
				return restore
			}(),
			expectedErr: errors.New("latest restorable time is not known"),
		},
		{
			desc: "Target datetime is after the latest restorable time",
			backup: func() *api.PerconaXtraDBClusterBackup {
				backup := mockBackup.DeepCopy()
				latestRestorableTime, _ := time.Parse(time.DateTime, "2025-07-16 10:30:00")
				backup.Status.LatestRestorableTime = &metav1.Time{Time: latestRestorableTime}
				return backup
			}(),

			restore: func() *api.PerconaXtraDBClusterRestore {
				restore := mockRestore.DeepCopy()
				restore.Spec.PITR = &api.PITR{
					Type: api.PITRTypeDate,
					Date: "2025-07-16 11:30:00",
				}
				return restore
			}(),
			expectedErr: errors.New("target datetime is after the latest restorable time"),
		},
		{
			desc: "Target datetime is before the latest restorable time",
			backup: func() *api.PerconaXtraDBClusterBackup {
				backup := mockBackup.DeepCopy()
				latestRestorableTime, _ := time.Parse(time.DateTime, "2025-07-16 10:30:00")
				backup.Status.LatestRestorableTime = &metav1.Time{Time: latestRestorableTime}
				return backup
			}(),

			restore: func() *api.PerconaXtraDBClusterRestore {
				restore := mockRestore.DeepCopy()
				restore.Spec.PITR = &api.PITR{
					Type: api.PITRTypeDate,
					Date: "2025-07-16 09:30:00",
				}
				return restore
			}(),
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := validatePITRTarget(tc.backup, tc.restore)
			if tc.expectedErr != nil {
				assert.Error(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// incrObject describes one incremental backup directory as it appears in object
// storage. xtrabackup_info is uploaded last, so a backup that died mid-upload
// has its data chunks but no marker file.
type incrObject struct {
	name     string
	complete bool
}

// chainListing renders what storage.Storage.ListObjects returns for a chain
// directory. Both the S3 and the Azure client strip their own prefix from the
// keys they return, so every key here starts with the list prefix. xbcloud
// chunks every file, hence the ".00000000000000000000" suffixes.
func chainListing(listPrefix string, objs ...incrObject) []string {
	out := make([]string, 0, len(objs)*3)
	for _, o := range objs {
		out = append(out,
			listPrefix+o.name+"/backup-my.cnf.00000000000000000000",
			listPrefix+o.name+"/ibdata1.00000000000000000000",
		)
		if o.complete {
			out = append(out, listPrefix+o.name+"/xtrabackup_info.00000000000000000000")
		}
	}
	return out
}

func TestResolveIncrementalChain(t *testing.T) {
	const (
		namespace = "namespace"
		baseName  = "c1-2026-11-03-10:00:00-full"
		incr1     = "c1-2026-11-03-11:00:00-incr"
		incr2     = "c1-2026-11-03-12:00:00-incr"
		incr3     = "c1-2026-11-03-13:00:00-incr"
	)

	// s3Backup builds an incremental backup CR whose destination points at leaf
	// inside baseName's chain directory, under bucket "my-bucket" and the given
	// prefix. Status.S3.Bucket is deliberately left empty so storage options fall
	// back to the destination's own prefix - which for an incremental is the
	// ".incr/" directory. That is the exact trap resolveIncrementalChain's
	// SetPrefix reset exists for.
	s3Backup := func(prefix, leaf string) *api.PerconaXtraDBClusterBackup {
		return &api.PerconaXtraDBClusterBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "target-backup", Namespace: namespace},
			Spec:       api.PXCBackupSpec{PXCCluster: "c1", StorageName: "s3-us-west"},
			Status: api.PXCBackupStatus{
				State:       api.BackupSucceeded,
				Type:        api.BackupTypeIncremental,
				Destination: api.PXCBackupDestination("s3://my-bucket/" + path.Join(prefix, baseName+".incr", leaf)),
				S3:          &api.BackupStorageS3Spec{CredentialsSecret: "s3-secret"},
			},
		}
	}
	azureBackup := func(prefix, leaf string) *api.PerconaXtraDBClusterBackup {
		bcp := s3Backup(prefix, leaf)
		bcp.Spec.StorageName = "azure-blob"
		bcp.Status.S3 = nil
		bcp.Status.Azure = &api.BackupStorageAzureSpec{CredentialsSecret: "azure-secret"}
		bcp.Status.Destination = api.PXCBackupDestination("azure://my-container/" + path.Join(prefix, baseName+".incr", leaf))
		return bcp
	}

	azureSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "azure-secret", Namespace: namespace}}

	// Paths as they come back from resolveIncrementalChain: bucket-relative,
	// which is what the restore script hands to "xbcloud get".
	chainPath := func(prefix, leaf string) string {
		return path.Join(prefix, baseName+".incr", leaf)
	}

	tests := []struct {
		name             string
		bcp              *api.PerconaXtraDBClusterBackup
		objects          []string
		listErr          error
		newClientErr     error
		objs             []runtime.Object
		wantBase         string
		wantIncrementals []string
		wantErr          string
		wantSetPrefix    []string
		wantListPrefix   []string
	}{
		{
			name:             "single incremental",
			bcp:              s3Backup("pfx", incr1),
			objects:          chainListing(baseName+".incr/", incrObject{incr1, true}),
			wantBase:         "s3://my-bucket/pfx/" + baseName,
			wantIncrementals: []string{chainPath("pfx", incr1)},
			wantSetPrefix:    []string{"pfx/"},
			wantListPrefix:   []string{baseName + ".incr/"},
		},
		{
			name: "three incrementals returned in chain order",
			bcp:  s3Backup("pfx", incr3),
			// Deliberately out of order: the listing order of an object store is
			// not guaranteed, so the chain must be sorted before use.
			objects: chainListing(baseName+".incr/",
				incrObject{incr2, true}, incrObject{incr3, true}, incrObject{incr1, true}),
			wantBase: "s3://my-bucket/pfx/" + baseName,
			wantIncrementals: []string{
				chainPath("pfx", incr1), chainPath("pfx", incr2), chainPath("pfx", incr3),
			},
			wantSetPrefix:  []string{"pfx/"},
			wantListPrefix: []string{baseName + ".incr/"},
		},
		{
			name: "directory without xtrabackup_info is excluded",
			bcp:  s3Backup("pfx", incr3),
			// incr2 died mid-upload: its data chunks are in the bucket but the
			// marker file is not. Admitting it would splice a partial backup into
			// the chain and the prepare would fail on inconsistent LSN ranges.
			objects: chainListing(baseName+".incr/",
				incrObject{incr1, true}, incrObject{incr2, false}, incrObject{incr3, true}),
			wantBase:         "s3://my-bucket/pfx/" + baseName,
			wantIncrementals: []string{chainPath("pfx", incr1), chainPath("pfx", incr3)},
			wantSetPrefix:    []string{"pfx/"},
			wantListPrefix:   []string{baseName + ".incr/"},
		},
		{
			name: "restoring the middle of a chain drops later incrementals",
			bcp:  s3Backup("pfx", incr2),
			objects: chainListing(baseName+".incr/",
				incrObject{incr1, true}, incrObject{incr2, true}, incrObject{incr3, true}),
			wantBase:         "s3://my-bucket/pfx/" + baseName,
			wantIncrementals: []string{chainPath("pfx", incr1), chainPath("pfx", incr2)},
			wantSetPrefix:    []string{"pfx/"},
			wantListPrefix:   []string{baseName + ".incr/"},
		},
		{
			name:             "bucket without a prefix",
			bcp:              s3Backup("", incr1),
			objects:          chainListing(baseName+".incr/", incrObject{incr1, true}),
			wantBase:         "s3://my-bucket/" + baseName,
			wantIncrementals: []string{chainPath("", incr1)},
			wantSetPrefix:    []string{""},
			wantListPrefix:   []string{baseName + ".incr/"},
		},
		{
			name:             "azure destination",
			bcp:              azureBackup("pfx", incr1),
			objs:             []runtime.Object{azureSecret},
			objects:          chainListing(baseName+".incr/", incrObject{incr1, true}),
			wantBase:         "azure://my-container/pfx/" + baseName,
			wantIncrementals: []string{chainPath("pfx", incr1)},
			wantSetPrefix:    []string{"pfx/"},
			wantListPrefix:   []string{baseName + ".incr/"},
		},
		{
			name: "objects belonging to a different base are excluded",
			bcp:  s3Backup("pfx", incr1),
			objects: append(
				chainListing(baseName+".incr/", incrObject{incr1, true}),
				chainListing("c1-2026-11-01-10:00:00-full.incr/", incrObject{incr2, true})...,
			),
			wantBase:         "s3://my-bucket/pfx/" + baseName,
			wantIncrementals: []string{chainPath("pfx", incr1)},
			wantSetPrefix:    []string{"pfx/"},
			wantListPrefix:   []string{baseName + ".incr/"},
		},
		{
			name: "directories that are not incremental leaves are excluded",
			bcp:  s3Backup("pfx", incr1),
			objects: append(
				chainListing(baseName+".incr/", incrObject{incr1, true}),
				baseName+".incr/some-other-dir/xtrabackup_info.00000000000000000000",
				baseName+".incr/"+baseName+"/xtrabackup_info.00000000000000000000",
			),
			wantBase:         "s3://my-bucket/pfx/" + baseName,
			wantIncrementals: []string{chainPath("pfx", incr1)},
			wantSetPrefix:    []string{"pfx/"},
			wantListPrefix:   []string{baseName + ".incr/"},
		},
		{
			name: "xtrabackup_info without the chunk suffix is not a marker",
			bcp:  s3Backup("pfx", incr1),
			// xbcloud always chunks, so a bare "xtrabackup_info" is not something
			// it produced and must not be treated as the completeness marker.
			objects:        []string{baseName + ".incr/" + incr1 + "/xtrabackup_info"},
			wantErr:        "no incremental backups found in chain",
			wantSetPrefix:  []string{"pfx/"},
			wantListPrefix: []string{baseName + ".incr/"},
		},
		{
			name:           "empty chain is an error",
			bcp:            s3Backup("pfx", incr1),
			objects:        nil,
			wantErr:        "no incremental backups found in chain",
			wantSetPrefix:  []string{"pfx/"},
			wantListPrefix: []string{baseName + ".incr/"},
		},
		{
			name: "chain holding only later incrementals is an error",
			bcp:  s3Backup("pfx", incr1),
			objects: chainListing(baseName+".incr/",
				incrObject{incr2, true}, incrObject{incr3, true}),
			wantErr:        "no incremental backups found in chain",
			wantSetPrefix:  []string{"pfx/"},
			wantListPrefix: []string{baseName + ".incr/"},
		},
		{
			name: "destination that does not parse as incremental is an error",
			bcp: func() *api.PerconaXtraDBClusterBackup {
				bcp := s3Backup("pfx", incr1)
				bcp.Status.Destination = "s3://my-bucket/pfx/" + baseName
				return bcp
			}(),
			wantErr: "could not parse incremental destination path",
		},
		{
			name:           "storage client error propagates",
			bcp:            s3Backup("pfx", incr1),
			newClientErr:   errors.New("boom"),
			wantErr:        "failed to create storage client: boom",
			wantSetPrefix:  nil,
			wantListPrefix: nil,
		},
		{
			name:           "list error propagates",
			bcp:            s3Backup("pfx", incr1),
			listErr:        errors.New("failListObjects"),
			wantErr:        "failed to list incremental backups: failListObjects",
			wantSetPrefix:  []string{"pfx/"},
			wantListPrefix: []string{baseName + ".incr/"},
		},
		{
			name: "storage options error propagates",
			bcp: func() *api.PerconaXtraDBClusterBackup {
				bcp := s3Backup("pfx", incr1)
				bcp.Status.S3 = nil
				return bcp
			}(),
			wantErr: "failed to get storage options",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

			var setPrefixCalls, listPrefixCalls []string
			stg := new(mockstorage.Storage)
			stg.On("SetPrefix", mock.Anything).Run(func(args mock.Arguments) {
				setPrefixCalls = append(setPrefixCalls, args.String(0))
			}).Return()
			stg.On("ListObjects", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				listPrefixCalls = append(listPrefixCalls, args.String(1))
			}).Return(tc.objects, tc.listErr)

			opts := &restorerOptions{
				bcp:       tc.bcp,
				k8sClient: buildFakeClient(append(tc.objs, tc.bcp)...),
				newStorageClient: func(context.Context, storage.Options) (storage.Storage, error) {
					if tc.newClientErr != nil {
						return nil, tc.newClientErr
					}
					return stg, nil
				},
			}

			base, incrementals, err := opts.resolveIncrementalChain(ctx)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Empty(t, base.String(), "no base may be returned alongside an error")
				assert.Nil(t, incrementals, "no chain may be returned alongside an error")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantBase, base.String())
				assert.Equal(t, tc.wantIncrementals, incrementals)
			}

			// The storage client is constructed from the INCREMENTAL destination,
			// so its prefix points inside the ".incr/" directory. It must be reset
			// to the BASE prefix before listing: listing with the incremental's
			// prefix still in place finds nothing, and listing with "" scans the
			// whole bucket.
			assert.Equal(t, tc.wantSetPrefix, setPrefixCalls, "prefix the storage client was scoped to")
			assert.Equal(t, tc.wantListPrefix, listPrefixCalls, "prefix passed to ListObjects")
		})
	}
}

func TestRestorerValidateIncremental(t *testing.T) {
	const namespace = "namespace"
	const baseName = "c1-2026-11-03-10:00:00-full"
	const incrName = "c1-2026-11-03-11:00:00-incr"

	newBackup := func(dest string, bcpType api.PXCBackupType) *api.PerconaXtraDBClusterBackup {
		return &api.PerconaXtraDBClusterBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "target-backup", Namespace: namespace},
			Spec:       api.PXCBackupSpec{PXCCluster: "c1", StorageName: "s3-us-west"},
			Status: api.PXCBackupStatus{
				State:       api.BackupSucceeded,
				Type:        bcpType,
				Destination: api.PXCBackupDestination(dest),
				S3:          &api.BackupStorageS3Spec{CredentialsSecret: "s3-secret"},
			},
		}
	}

	tests := []struct {
		name string
		bcp  *api.PerconaXtraDBClusterBackup
		// azure drives the azure restorer instead of the s3 one, to prove both
		// carry the same guard.
		azure          bool
		wantErr        string
		wantSetPrefix  []string
		wantListPrefix []string
		// wantFullPath is clientPrefix + listPrefix: the object-storage path the
		// existence check actually looks at, whichever way the prefix was split.
		wantFullPath string
	}{
		{
			name: "full backup is unaffected",
			bcp:  newBackup("s3://my-bucket/pfx/"+baseName, api.BackupTypeFull),
			// Legacy behaviour: SetPrefix is never called, and the list path comes
			// straight from backupListPath.
			wantSetPrefix:  nil,
			wantListPrefix: []string{"pfx/" + baseName + "/"},
			wantFullPath:   "pfx/" + baseName,
		},
		{
			name:           "full backup with an empty status type is unaffected",
			bcp:            newBackup("s3://my-bucket/pfx/"+baseName, ""),
			wantSetPrefix:  nil,
			wantListPrefix: []string{"pfx/" + baseName + "/"},
			wantFullPath:   "pfx/" + baseName,
		},
		{
			name: "incremental on s3 is accepted and scoped to its own directory",
			bcp: newBackup(
				"s3://my-bucket/pfx/"+baseName+".incr/"+incrName, api.BackupTypeIncremental),
			// Mirror image of the chain listing: validation looks INSIDE the
			// incremental, so the client is pinned to the incremental's prefix.
			wantSetPrefix:  []string{"pfx/" + baseName + ".incr/"},
			wantListPrefix: []string{incrName + "/"},
			wantFullPath:   "pfx/" + baseName + ".incr/" + incrName,
		},
		{
			name: "incremental on azure is accepted",
			bcp: newBackup(
				"azure://my-container/pfx/"+baseName+".incr/"+incrName, api.BackupTypeIncremental),
			azure:          true,
			wantSetPrefix:  []string{"pfx/" + baseName + ".incr/"},
			wantListPrefix: []string{incrName + "/"},
			wantFullPath:   "pfx/" + baseName + ".incr/" + incrName,
		},
		{
			name: "incremental on gcs is rejected",
			bcp: newBackup(
				"gs://my-bucket/pfx/"+baseName+".incr/"+incrName, api.BackupTypeIncremental),
			wantErr: "incremental backups are only supported on s3 and azure storages",
		},
		{
			name:    "incremental on a pvc destination is rejected",
			bcp:     newBackup("pvc/"+baseName, api.BackupTypeIncremental),
			wantErr: "incremental backups are only supported on s3 and azure storages",
		},
		{
			name:    "incremental with a non-incremental destination is rejected",
			bcp:     newBackup("s3://my-bucket/pfx/"+baseName, api.BackupTypeIncremental),
			wantErr: "is not an incremental path",
		},
		{
			name: "incremental nested under a prefix that merely ends in .incr is rejected",
			bcp: newBackup(
				"s3://my-bucket/nightly.incr/"+baseName, api.BackupTypeIncremental),
			wantErr: "is not an incremental path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

			if tc.azure {
				tc.bcp.Spec.StorageName = "azure-blob"
				tc.bcp.Status.S3 = nil
				tc.bcp.Status.Azure = &api.BackupStorageAzureSpec{CredentialsSecret: "azure-secret"}
			}

			var listPrefixCalls, setPrefixCalls []string
			// Model the real clients: SetPrefix is what GetPrefix reports back,
			// and ListObjects resolves its argument against it.
			clientPrefix := ""
			stg := new(mockstorage.Storage)
			stg.On("SetPrefix", mock.Anything).Run(func(args mock.Arguments) {
				clientPrefix = args.String(0)
				setPrefixCalls = append(setPrefixCalls, clientPrefix)
			}).Return()
			stg.On("GetPrefix").Return(func() string { return clientPrefix })
			stg.On("ListObjects", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				listPrefixCalls = append(listPrefixCalls, args.String(1))
			}).Return([]string{"some-object"}, nil)

			objs := []runtime.Object{
				tc.bcp,
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "azure-secret", Namespace: namespace}},
			}
			opts := &restorerOptions{
				cr:        &api.PerconaXtraDBClusterRestore{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: namespace}},
				bcp:       tc.bcp,
				k8sClient: buildFakeClient(objs...),
				newStorageClient: func(context.Context, storage.Options) (storage.Storage, error) {
					return stg, nil
				},
			}

			var err error
			if tc.azure {
				err = (&azure{opts}).Validate(ctx)
			} else {
				err = (&s3{opts}).Validate(ctx)
			}

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Empty(t, listPrefixCalls, "storage must not be listed once the guard rejects the restore")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantSetPrefix, setPrefixCalls, "prefix the storage client was scoped to")
			assert.Equal(t, tc.wantListPrefix, listPrefixCalls, "prefix passed to ListObjects")
			// Whatever the two combine to must still address the backup itself.
			assert.Equal(t, tc.wantFullPath, path.Join(clientPrefix, listPrefixCalls[0]))
		})
	}
}
