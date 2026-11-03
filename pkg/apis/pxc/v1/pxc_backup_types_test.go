package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestPXCBackupDestination_IsIncremental(t *testing.T) {
	tests := []struct {
		name string
		dest *PXCBackupDestination
		want bool
	}{
		{
			name: "full backup",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full")),
			want: false,
		},
		{
			name: "incremental backup",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: true,
		},
		{
			name: "incremental backup without prefix",
			dest: new(PXCBackupDestination("s3://bucket/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: true,
		},
		{
			name: "incremental backup under a nested prefix",
			dest: new(PXCBackupDestination("s3://bucket/a/b/c/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: true,
		},
		{
			name: "pvc destination",
			dest: new(PXCBackupDestination("pvc/c1-2026-11-03-10:00:00-full")),
			want: false,
		},
		{
			name: "azure full backup",
			dest: new(PXCBackupDestination("azure://container/pfx/c1-2026-11-03-10:00:00-full")),
			want: false,
		},
		{
			name: "azure incremental backup",
			dest: new(PXCBackupDestination("azure://container/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: true,
		},
		{
			name: "bucket name contains .incr but holds a full backup",
			dest: new(PXCBackupDestination("s3://my.incr-bucket/pfx/c1-2026-11-03-10:00:00-full")),
			want: false,
		},
		{
			name: "bucket name contains .incr and holds an incremental backup",
			dest: new(PXCBackupDestination("s3://my.incr-bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: true,
		},
		{
			name: "prefix contains .incr but holds a full backup",
			dest: new(PXCBackupDestination("s3://bucket/nightly.incr/c1-2026-11-03-10:00:00-full")),
			want: false,
		},
		{
			name: "empty destination",
			dest: new(PXCBackupDestination("")),
			want: false,
		},
		{
			name: "nil destination",
			dest: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.dest.IsIncremental())
		})
	}
}

func TestPXCBackupDestination_IncrementalBaseDestination(t *testing.T) {
	tests := []struct {
		name string
		dest *PXCBackupDestination
		want PXCBackupDestination
	}{
		{
			name: "incremental resolves to its base",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "s3://bucket/pfx/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "incremental without prefix",
			dest: new(PXCBackupDestination("s3://bucket/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "s3://bucket/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "incremental under a nested prefix",
			dest: new(PXCBackupDestination("s3://bucket/a/b/c/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "s3://bucket/a/b/c/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "azure incremental",
			dest: new(PXCBackupDestination("azure://container/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "azure://container/pfx/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "bucket name contains .incr",
			dest: new(PXCBackupDestination("s3://my.incr-bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "s3://my.incr-bucket/pfx/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "full backup is returned unchanged",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full")),
			want: "s3://bucket/pfx/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "full backup under a .incr prefix is returned unchanged",
			dest: new(PXCBackupDestination("s3://bucket/nightly.incr/c1-2026-11-03-10:00:00-full")),
			want: "s3://bucket/nightly.incr/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "pvc destination is returned unchanged",
			dest: new(PXCBackupDestination("pvc/c1-2026-11-03-10:00:00-full")),
			want: "pvc/c1-2026-11-03-10:00:00-full",
		},
		{
			name: "empty destination",
			dest: new(PXCBackupDestination("")),
			want: "",
		},
		{
			name: "nil destination",
			dest: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.dest.IncrementalBaseDestination())
		})
	}
}

func TestPXCBackupDestination_IncrementalsDir(t *testing.T) {
	tests := []struct {
		name string
		dest *PXCBackupDestination
		want string
	}{
		{
			name: "incremental with prefix",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "pfx/c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name: "incremental without prefix",
			dest: new(PXCBackupDestination("s3://bucket/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name: "incremental under a nested prefix",
			dest: new(PXCBackupDestination("s3://bucket/a/b/c/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "a/b/c/c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name: "azure incremental",
			dest: new(PXCBackupDestination("azure://container/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "pfx/c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name: "bucket name contains .incr",
			dest: new(PXCBackupDestination("s3://my.incr-bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "pfx/c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name: "full backup has no incrementals dir",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full")),
			want: "",
		},
		{
			name: "full backup under a .incr prefix has no incrementals dir",
			dest: new(PXCBackupDestination("s3://bucket/nightly.incr/c1-2026-11-03-10:00:00-full")),
			want: "",
		},
		{
			name: "pvc destination has no incrementals dir",
			dest: new(PXCBackupDestination("pvc/c1-2026-11-03-10:00:00-full")),
			want: "",
		},
		{
			name: "nil destination",
			dest: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.dest.IncrementalsDir())
		})
	}
}

func TestPXCBackupDestination_BackupName(t *testing.T) {
	tests := []struct {
		name string
		dest *PXCBackupDestination
		want string
	}{
		{
			name: "s3 full backup with prefix",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full")),
			want: "c1-2026-11-03-10:00:00-full",
		},
		{
			name: "s3 full backup without prefix",
			dest: new(PXCBackupDestination("s3://bucket/c1-2026-11-03-10:00:00-full")),
			want: "c1-2026-11-03-10:00:00-full",
		},
		{
			name: "s3 incremental backup returns the leaf only",
			dest: new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "c1-2026-11-03-11:00:00-incr",
		},
		{
			name: "s3 incremental backup without prefix",
			dest: new(PXCBackupDestination("s3://bucket/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "c1-2026-11-03-11:00:00-incr",
		},
		{
			name: "s3 incremental backup under a nested prefix",
			dest: new(PXCBackupDestination("s3://bucket/a/b/c/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "c1-2026-11-03-11:00:00-incr",
		},
		{
			name: "azure incremental backup",
			dest: new(PXCBackupDestination("azure://container/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			want: "c1-2026-11-03-11:00:00-incr",
		},
		{
			name: "pvc destination keeps the whole name",
			dest: new(PXCBackupDestination("pvc/c1-2026-11-03-10:00:00-full")),
			want: "c1-2026-11-03-10:00:00-full",
		},
		{
			name: "empty destination",
			dest: new(PXCBackupDestination("")),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.dest.BackupName())
		})
	}
}

func TestPXCBackupDestination_BucketAndPrefix(t *testing.T) {
	tests := []struct {
		name       string
		dest       *PXCBackupDestination
		wantBucket string
		wantPrefix string
	}{
		{
			name:       "s3 full backup with prefix",
			dest:       new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full")),
			wantBucket: "bucket",
			wantPrefix: "pfx/",
		},
		{
			name:       "s3 full backup without prefix",
			dest:       new(PXCBackupDestination("s3://bucket/c1-2026-11-03-10:00:00-full")),
			wantBucket: "bucket",
			wantPrefix: "",
		},
		{
			name:       "s3 incremental backup keeps the chain dir in the prefix",
			dest:       new(PXCBackupDestination("s3://bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			wantBucket: "bucket",
			wantPrefix: "pfx/c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name:       "s3 incremental backup without prefix",
			dest:       new(PXCBackupDestination("s3://bucket/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			wantBucket: "bucket",
			wantPrefix: "c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name:       "s3 incremental backup under a nested prefix",
			dest:       new(PXCBackupDestination("s3://bucket/a/b/c/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			wantBucket: "bucket",
			wantPrefix: "a/b/c/c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name:       "azure incremental backup",
			dest:       new(PXCBackupDestination("azure://container/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			wantBucket: "container",
			wantPrefix: "pfx/c1-2026-11-03-10:00:00-full.incr/",
		},
		{
			name:       "bucket name contains .incr",
			dest:       new(PXCBackupDestination("s3://my.incr-bucket/pfx/c1-2026-11-03-10:00:00-full.incr/c1-2026-11-03-11:00:00-incr")),
			wantBucket: "my.incr-bucket",
			wantPrefix: "pfx/c1-2026-11-03-10:00:00-full.incr/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, prefix := tt.dest.BucketAndPrefix()
			assert.Equal(t, tt.wantBucket, bucket)
			assert.Equal(t, tt.wantPrefix, prefix)
		})
	}
}

func TestPerconaXtraDBClusterBackup_GetType(t *testing.T) {
	tests := []struct {
		name   string
		backup *PerconaXtraDBClusterBackup
		want   PXCBackupType
	}{
		{
			name:   "empty status type defaults to full",
			backup: &PerconaXtraDBClusterBackup{},
			want:   BackupTypeFull,
		},
		{
			name: "explicit full",
			backup: &PerconaXtraDBClusterBackup{
				Status: PXCBackupStatus{Type: BackupTypeFull},
			},
			want: BackupTypeFull,
		},
		{
			name: "explicit incremental",
			backup: &PerconaXtraDBClusterBackup{
				Status: PXCBackupStatus{Type: BackupTypeIncremental},
			},
			want: BackupTypeIncremental,
		},
		{
			name: "spec type is ignored while status type is empty",
			backup: &PerconaXtraDBClusterBackup{
				Spec: PXCBackupSpec{Type: BackupTypeIncremental},
			},
			want: BackupTypeFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.backup.GetType())
		})
	}
}

func TestBackupStorageSpec_Equals(t *testing.T) {
	s3Storage := func(mutate func(*BackupStorageS3Spec)) *BackupStorageSpec {
		s3 := &BackupStorageS3Spec{
			Bucket:            "bucket",
			Prefix:            "pfx",
			Region:            "us-east-1",
			EndpointURL:       "https://minio:9000",
			CredentialsSecret: "cluster1-s3-credentials",
		}
		if mutate != nil {
			mutate(s3)
		}
		return &BackupStorageSpec{Type: BackupStorageS3, S3: s3}
	}
	azureStorage := func(mutate func(*BackupStorageAzureSpec)) *BackupStorageSpec {
		azure := &BackupStorageAzureSpec{
			ContainerPath:     "container/pfx",
			Prefix:            "nightly",
			Endpoint:          "https://azurite:10000",
			CredentialsSecret: "cluster1-azure-credentials",
		}
		if mutate != nil {
			mutate(azure)
		}
		return &BackupStorageSpec{Type: BackupStorageAzure, Azure: azure}
	}

	tests := []struct {
		name string
		a    *BackupStorageSpec
		b    *BackupStorageSpec
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "first nil",
			a:    nil,
			b:    s3Storage(nil),
			want: false,
		},
		{
			name: "second nil",
			a:    s3Storage(nil),
			b:    nil,
			want: false,
		},
		{
			name: "same s3 storage",
			a:    s3Storage(nil),
			b:    s3Storage(nil),
			want: true,
		},
		{
			name: "different s3 bucket",
			a:    s3Storage(nil),
			b:    s3Storage(func(s *BackupStorageS3Spec) { s.Bucket = "other-bucket" }),
			want: false,
		},
		{
			name: "different s3 prefix",
			a:    s3Storage(nil),
			b:    s3Storage(func(s *BackupStorageS3Spec) { s.Prefix = "other-pfx" }),
			want: false,
		},
		{
			name: "different s3 region",
			a:    s3Storage(nil),
			b:    s3Storage(func(s *BackupStorageS3Spec) { s.Region = "eu-west-1" }),
			want: false,
		},
		{
			name: "different s3 endpoint",
			a:    s3Storage(nil),
			b:    s3Storage(func(s *BackupStorageS3Spec) { s.EndpointURL = "https://s3.amazonaws.com" }),
			want: false,
		},
		{
			name: "different s3 credentials secret only",
			a:    s3Storage(nil),
			b:    s3Storage(func(s *BackupStorageS3Spec) { s.CredentialsSecret = "rotated-credentials" }),
			want: true,
		},
		{
			name: "different storage type",
			a:    s3Storage(nil),
			b:    azureStorage(nil),
			want: false,
		},
		{
			name: "same azure storage",
			a:    azureStorage(nil),
			b:    azureStorage(nil),
			want: true,
		},
		{
			name: "different azure container",
			a:    azureStorage(nil),
			b:    azureStorage(func(s *BackupStorageAzureSpec) { s.ContainerPath = "other-container/pfx" }),
			want: false,
		},
		{
			name: "different azure endpoint",
			a:    azureStorage(nil),
			b:    azureStorage(func(s *BackupStorageAzureSpec) { s.Endpoint = "https://other:10000" }),
			want: false,
		},
		{
			name: "nil s3 sub-spec on one side",
			a:    &BackupStorageSpec{Type: BackupStorageS3},
			b:    s3Storage(nil),
			want: false,
		},
		{
			name: "nil azure sub-spec on one side",
			a:    &BackupStorageSpec{Type: BackupStorageAzure},
			b:    azureStorage(nil),
			want: false,
		},
		{
			name: "nil sub-spec on both sides",
			a:    &BackupStorageSpec{Type: BackupStorageS3},
			b:    &BackupStorageSpec{Type: BackupStorageS3},
			want: true,
		},
		{
			name: "filesystem storages never match",
			a:    &BackupStorageSpec{Type: BackupStorageFilesystem, Volume: &VolumeSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			b:    &BackupStorageSpec{Type: BackupStorageFilesystem, Volume: &VolumeSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.a.Equals(tt.b))
			assert.Equal(t, tt.want, tt.b.Equals(tt.a), "Equals must be symmetric")
		})
	}
}

func TestPXCScheduledBackupScheduleTypeDefaulting(t *testing.T) {
	tests := []struct {
		name      string
		crVersion string
		backup    *BackupSpec
		want      []PXCBackupType
	}{
		{
			name:      "empty type defaults to full",
			crVersion: "1.21.0",
			backup: &BackupSpec{
				Schedule: []PXCScheduledBackupSchedule{{Name: "daily"}, {Name: "hourly"}},
			},
			want: []PXCBackupType{BackupTypeFull, BackupTypeFull},
		},
		{
			name:      "explicit type is preserved",
			crVersion: "1.21.0",
			backup: &BackupSpec{
				Schedule: []PXCScheduledBackupSchedule{
					{Name: "weekly", Type: BackupTypeFull},
					{Name: "hourly", Type: BackupTypeIncremental},
				},
			},
			want: []PXCBackupType{BackupTypeFull, BackupTypeIncremental},
		},
		{
			name:      "older cr version is left untouched",
			crVersion: "1.20.0",
			backup: &BackupSpec{
				Schedule: []PXCScheduledBackupSchedule{{Name: "daily"}},
			},
			want: []PXCBackupType{""},
		},
		{
			name:      "nil backup spec",
			crVersion: "1.21.0",
			backup:    nil,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &PerconaXtraDBCluster{
				Spec: PerconaXtraDBClusterSpec{
					CRVersion: tt.crVersion,
					Backup:    tt.backup,
				},
			}

			cr.setScheduledBackupTypeDefaults()

			var got []PXCBackupType
			if cr.Spec.Backup != nil {
				for _, sch := range cr.Spec.Backup.Schedule {
					got = append(got, sch.Type)
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
