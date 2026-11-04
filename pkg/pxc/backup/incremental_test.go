package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

const (
	incrTestNamespace = "test-ns"
	incrTestCluster   = "cluster1"

	// baseLeaf and otherBaseLeaf are object-storage leaf names of two full
	// backups, i.e. the two chain keys used throughout these tests.
	baseLeaf      = "cluster1-2026-11-03-10:00:00-full"
	otherBaseLeaf = "cluster1-2026-11-02-10:00:00-full"
)

var incrBaseTime = time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)

// at returns a timestamp offset from the fixed test clock, so cases can order
// backups by writing minutes.
func at(minutes int) time.Time {
	return incrBaseTime.Add(time.Duration(minutes) * time.Minute)
}

type bcpOption func(*api.PerconaXtraDBClusterBackup)

func withCluster(name string) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.Spec.PXCCluster = name }
}

func withStatusType(t api.PXCBackupType) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.Status.Type = t }
}

func withState(s api.PXCBackupState) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.Status.State = s }
}

func withDestination(d string) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.Status.Destination = api.PXCBackupDestination(d) }
}

func withBaseAnnotation(v string) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) {
		b.Annotations = map[string]string{naming.AnnotationBaseBackupName: v}
	}
}

func withoutAnnotations() bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.Annotations = nil }
}

func withCompletedAt(minutes int) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) {
		t := metav1.NewTime(at(minutes))
		b.Status.CompletedAt = &t
	}
}

func withoutCompletedAt() bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.Status.CompletedAt = nil }
}

func withCreatedAt(minutes int) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.CreationTimestamp = metav1.NewTime(at(minutes)) }
}

func withS3(bucket string) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) {
		b.Status.StorageType = api.BackupStorageS3
		b.Status.StorageName = bucket
		b.Status.S3 = &api.BackupStorageS3Spec{Bucket: bucket, Region: "us-west-1"}
		b.Status.Azure = nil
	}
}

func withAzure(container string) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) {
		b.Status.StorageType = api.BackupStorageAzure
		b.Status.StorageName = container
		b.Status.Azure = &api.BackupStorageAzureSpec{ContainerPath: container}
		b.Status.S3 = nil
	}
}

func withSpecBase(name string) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) { b.Spec.IncrementalBaseBackupName = &name }
}

// fullBcp builds a succeeded full backup with a destination and a completion
// time, which is the shape most cases need; options override any of it.
func fullBcp(name string, opts ...bcpOption) *api.PerconaXtraDBClusterBackup {
	completed := metav1.NewTime(incrBaseTime)
	b := &api.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         incrTestNamespace,
			CreationTimestamp: metav1.NewTime(incrBaseTime),
		},
		Spec: api.PXCBackupSpec{
			PXCCluster: incrTestCluster,
			Type:       api.BackupTypeFull,
		},
		Status: api.PXCBackupStatus{
			Type:        api.BackupTypeFull,
			State:       api.BackupSucceeded,
			CompletedAt: &completed,
			Destination: api.PXCBackupDestination("s3://bucket-a/" + baseLeaf),
		},
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// incrBcp builds a succeeded incremental backup chained to baseLeaf.
func incrBcp(name string, opts ...bcpOption) *api.PerconaXtraDBClusterBackup {
	completed := metav1.NewTime(incrBaseTime)
	b := &api.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         incrTestNamespace,
			CreationTimestamp: metav1.NewTime(incrBaseTime),
			Annotations:       map[string]string{naming.AnnotationBaseBackupName: baseLeaf},
		},
		Spec: api.PXCBackupSpec{
			PXCCluster: incrTestCluster,
			Type:       api.BackupTypeIncremental,
		},
		Status: api.PXCBackupStatus{
			Type:        api.BackupTypeIncremental,
			State:       api.BackupSucceeded,
			CompletedAt: &completed,
			Destination: api.PXCBackupDestination("s3://bucket-a/" + baseLeaf + ".incr/" + name + "-incr"),
		},
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

func s3Storage(bucket string) *api.BackupStorageSpec {
	return &api.BackupStorageSpec{
		Type: api.BackupStorageS3,
		S3:   &api.BackupStorageS3Spec{Bucket: bucket, Region: "us-west-1"},
	}
}

func azureStorage(container string) *api.BackupStorageSpec {
	return &api.BackupStorageSpec{
		Type:  api.BackupStorageAzure,
		Azure: &api.BackupStorageAzureSpec{ContainerPath: container},
	}
}

func testCluster() *api.PerconaXtraDBCluster {
	return &api.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{Name: incrTestCluster, Namespace: incrTestNamespace},
	}
}

// incrFakeClient builds a fake client with the PXCClusterBackupField index that
// every chain helper lists through, mirroring the manager-side registration in
// pkg/controller/pxc/controller.go.
func incrFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, api.SchemeBuilder.AddToScheme(scheme))

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithIndex(&api.PerconaXtraDBClusterBackup{}, PXCClusterBackupField, func(obj client.Object) []string {
			bcp, ok := obj.(*api.PerconaXtraDBClusterBackup)
			if !ok {
				return nil
			}
			return []string{bcp.Spec.PXCCluster}
		}).
		Build()
}

func backupNames(backups []api.PerconaXtraDBClusterBackup) []string {
	names := make([]string, 0, len(backups))
	for i := range backups {
		names = append(names, backups[i].Name)
	}
	return names
}

func TestSortBackupsByCompletion(t *testing.T) {
	tests := []struct {
		name    string
		backups []api.PerconaXtraDBClusterBackup
		want    []string
	}{
		{
			name: "all completed, reverse order",
			backups: []api.PerconaXtraDBClusterBackup{
				*incrBcp("c", withCompletedAt(30)),
				*incrBcp("b", withCompletedAt(20)),
				*incrBcp("a", withCompletedAt(10)),
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "already sorted stays sorted",
			backups: []api.PerconaXtraDBClusterBackup{
				*incrBcp("a", withCompletedAt(10)),
				*incrBcp("b", withCompletedAt(20)),
				*incrBcp("c", withCompletedAt(30)),
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "none completed falls back to creation timestamp",
			backups: []api.PerconaXtraDBClusterBackup{
				*incrBcp("c", withoutCompletedAt(), withCreatedAt(30)),
				*incrBcp("a", withoutCompletedAt(), withCreatedAt(10)),
				*incrBcp("b", withoutCompletedAt(), withCreatedAt(20)),
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "mixed nil and non-nil completion times",
			backups: []api.PerconaXtraDBClusterBackup{
				*incrBcp("running-latest", withoutCompletedAt(), withCreatedAt(40)),
				*incrBcp("done-first", withCompletedAt(10), withCreatedAt(5)),
				*incrBcp("running-middle", withoutCompletedAt(), withCreatedAt(20)),
				*incrBcp("done-third", withCompletedAt(30), withCreatedAt(5)),
			},
			want: []string{"done-first", "running-middle", "done-third", "running-latest"},
		},
		{
			name: "equal timestamps keep input order",
			backups: []api.PerconaXtraDBClusterBackup{
				*incrBcp("second", withCompletedAt(10)),
				*incrBcp("first", withCompletedAt(10)),
				*incrBcp("third", withCompletedAt(10)),
			},
			want: []string{"second", "first", "third"},
		},
		{
			name:    "empty slice",
			backups: []api.PerconaXtraDBClusterBackup{},
			want:    []string{},
		},
		{
			name: "single element",
			backups: []api.PerconaXtraDBClusterBackup{
				*incrBcp("only", withoutCompletedAt()),
			},
			want: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() { sortBackupsByCompletion(tt.backups) })
			assert.Equal(t, tt.want, backupNames(tt.backups))
		})
	}
}

func TestGetLastFullBackup(t *testing.T) {
	tests := []struct {
		name     string
		objs     []client.Object
		storage  *api.BackupStorageSpec
		want     string
		wantErr  string
		wantErrs error
	}{
		{
			name:     "no backups at all",
			objs:     nil,
			storage:  s3Storage("bucket-a"),
			wantErrs: ErrNoFullBackupFound,
		},
		{
			name: "only incremental backups",
			objs: []client.Object{
				incrBcp("incr-1", withS3("bucket-a"), withCompletedAt(10)),
				incrBcp("incr-2", withS3("bucket-a"), withCompletedAt(20)),
			},
			storage:  s3Storage("bucket-a"),
			wantErrs: ErrNoFullBackupFound,
		},
		{
			name: "single succeeded full backup",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withCompletedAt(10)),
			},
			storage: s3Storage("bucket-a"),
			want:    "full-1",
		},
		{
			name: "newest of several full backups",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("full-3", withS3("bucket-a"), withCompletedAt(30)),
				fullBcp("full-2", withS3("bucket-a"), withCompletedAt(20)),
			},
			storage: s3Storage("bucket-a"),
			want:    "full-3",
		},
		{
			name: "newest full is in another storage, older one in the target storage wins",
			objs: []client.Object{
				fullBcp("full-old-right-storage", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("full-new-wrong-storage", withS3("bucket-b"), withCompletedAt(30)),
				fullBcp("full-new-azure", withAzure("container-a"), withCompletedAt(40)),
			},
			storage: s3Storage("bucket-a"),
			want:    "full-old-right-storage",
		},
		{
			name: "azure storage selects the azure backup",
			objs: []client.Object{
				fullBcp("full-s3", withS3("bucket-a"), withCompletedAt(30)),
				fullBcp("full-azure", withAzure("container-a"), withCompletedAt(10)),
			},
			storage: azureStorage("container-a"),
			want:    "full-azure",
		},
		{
			name: "nil storage matches any storage",
			objs: []client.Object{
				fullBcp("full-s3", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("full-azure", withAzure("container-a"), withCompletedAt(30)),
			},
			storage: nil,
			want:    "full-azure",
		},
		{
			name: "failed and running full backups are skipped",
			objs: []client.Object{
				fullBcp("full-failed", withS3("bucket-a"), withCompletedAt(40), withState(api.BackupFailed)),
				fullBcp("full-running", withS3("bucket-a"), withoutCompletedAt(), withState(api.BackupRunning)),
				fullBcp("full-ok", withS3("bucket-a"), withCompletedAt(10)),
			},
			storage: s3Storage("bucket-a"),
			want:    "full-ok",
		},
		{
			name: "backup with empty status type is treated as full",
			objs: []client.Object{
				fullBcp("legacy-full", withStatusType(""), withS3("bucket-a"), withCompletedAt(10)),
			},
			storage: s3Storage("bucket-a"),
			want:    "legacy-full",
		},
		{
			name: "succeeded full backup without completedAt is an error naming that backup",
			objs: []client.Object{
				fullBcp("good-full", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("broken-full", withS3("bucket-a"), withoutCompletedAt()),
			},
			storage: s3Storage("bucket-a"),
			wantErr: "backup test-ns/broken-full is succeeded but completedAt is not set",
		},
		{
			name: "succeeded full backup without completedAt in another storage is skipped, not an error",
			objs: []client.Object{
				fullBcp("broken-full-other-storage", withS3("bucket-b"), withoutCompletedAt()),
				fullBcp("good-full", withS3("bucket-a"), withCompletedAt(10)),
			},
			storage: s3Storage("bucket-a"),
			want:    "good-full",
		},
		{
			name: "backups of another cluster are excluded",
			objs: []client.Object{
				fullBcp("other-cluster-full", withCluster("cluster2"), withS3("bucket-a"), withCompletedAt(40)),
				fullBcp("our-full", withS3("bucket-a"), withCompletedAt(10)),
			},
			storage: s3Storage("bucket-a"),
			want:    "our-full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := GetLastFullBackup(t.Context(), cl, testCluster(), tt.storage)

			switch {
			case tt.wantErrs != nil:
				require.ErrorIs(t, err, tt.wantErrs)
				assert.Nil(t, got)
			case tt.wantErr != "":
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
			default:
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.want, got.Name)
			}
		})
	}
}

func TestGetLatestIncrementalBackupInChain(t *testing.T) {
	tests := []struct {
		name          string
		objs          []client.Object
		input         *api.PerconaXtraDBClusterBackup
		onlySucceeded bool
		want          string
		wantErr       string
		wantErrs      error
	}{
		{
			name:     "chain with no members",
			objs:     []client.Object{fullBcp("base")},
			input:    fullBcp("base"),
			wantErrs: ErrNoIncrementalBackupFound,
		},
		{
			name: "only incrementals of another base",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("other-1", withBaseAnnotation(otherBaseLeaf), withCompletedAt(10)),
			},
			input:    fullBcp("base"),
			wantErrs: ErrNoIncrementalBackupFound,
		},
		{
			name: "single incremental in chain",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
		},
		{
			name: "three incrementals returns the newest",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-3", withCompletedAt(30)),
				incrBcp("incr-2", withCompletedAt(20)),
			},
			input: fullBcp("base"),
			want:  "incr-3",
		},
		{
			name: "onlySucceeded skips a failed tail",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-failed", withCompletedAt(20), withState(api.BackupFailed)),
			},
			input:         fullBcp("base"),
			onlySucceeded: true,
			want:          "incr-1",
		},
		{
			name: "without onlySucceeded the failed tail is returned",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-failed", withCompletedAt(20), withState(api.BackupFailed)),
			},
			input:         fullBcp("base"),
			onlySucceeded: false,
			want:          "incr-2-failed",
		},
		{
			name: "without onlySucceeded a running tail is ordered by creation time",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-running", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupRunning)),
			},
			input:         fullBcp("base"),
			onlySucceeded: false,
			want:          "incr-2-running",
		},
		{
			name: "incrementals of a different base are excluded",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("other-newer", withBaseAnnotation(otherBaseLeaf), withCompletedAt(40)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
		},
		{
			name: "incremental input resolves the same chain as the base",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
				incrBcp("other-newer", withBaseAnnotation(otherBaseLeaf), withCompletedAt(40)),
			},
			input: incrBcp("incr-1", withCompletedAt(10)),
			want:  "incr-2",
		},
		{
			name: "newest incremental given as input returns itself",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
			},
			input: incrBcp("incr-2", withCompletedAt(20)),
			want:  "incr-2",
		},
		{
			name: "listed incremental without the annotation is skipped",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("orphan", withoutAnnotations(), withCompletedAt(40)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
		},
		{
			name: "incremental input without the annotation is an error",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
			},
			input:   incrBcp("orphan", withoutAnnotations()),
			wantErr: "incremental backup test-ns/orphan has no percona.com/base-backup-name annotation",
		},
		{
			name: "full backup without a destination owns no chain",
			objs: []client.Object{
				incrBcp("incr-1", withCompletedAt(10)),
			},
			input:    fullBcp("base", withDestination("")),
			wantErrs: ErrNoIncrementalBackupFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := GetLatestIncrementalBackupInChain(t.Context(), cl, tt.input, tt.onlySucceeded)

			switch {
			case tt.wantErrs != nil:
				require.ErrorIs(t, err, tt.wantErrs)
				assert.Nil(t, got)
			case tt.wantErr != "":
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
			default:
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.want, got.Name)
			}
		})
	}
}

func TestListDependentIncrementalBackups(t *testing.T) {
	tests := []struct {
		name    string
		objs    []client.Object
		input   *api.PerconaXtraDBClusterBackup
		want    []string
		wantErr string
	}{
		{
			name:  "no dependents returns an empty non-nil slice",
			objs:  []client.Object{fullBcp("base")},
			input: fullBcp("base"),
			want:  []string{},
		},
		{
			name: "dependents are returned oldest first",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-3", withCompletedAt(30)),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
			},
			input: fullBcp("base"),
			want:  []string{"incr-1", "incr-2", "incr-3"},
		},
		{
			name: "unfinished dependents are included and ordered by creation time",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-2-running", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupRunning)),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-3-failed", withCompletedAt(30), withState(api.BackupFailed)),
			},
			input: fullBcp("base"),
			want:  []string{"incr-1", "incr-2-running", "incr-3-failed"},
		},
		{
			name: "incrementals of another base are excluded",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("other-1", withBaseAnnotation(otherBaseLeaf), withCompletedAt(20)),
				incrBcp("orphan", withoutAnnotations(), withCompletedAt(30)),
			},
			input: fullBcp("base"),
			want:  []string{"incr-1"},
		},
		{
			name: "full backups are never dependents",
			objs: []client.Object{
				fullBcp("base"),
				fullBcp("another-full", withBaseAnnotation(baseLeaf), withCompletedAt(20)),
				incrBcp("incr-1", withCompletedAt(10)),
			},
			input: fullBcp("base"),
			want:  []string{"incr-1"},
		},
		{
			name: "an incremental has no dependents",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
			},
			input: incrBcp("incr-1", withCompletedAt(10)),
			want:  []string{},
		},
		{
			name: "full backup without a destination has no dependents",
			objs: []client.Object{
				incrBcp("incr-1", withCompletedAt(10)),
			},
			input: fullBcp("base", withDestination("")),
			want:  []string{},
		},
		{
			name: "dependents of another cluster are excluded",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("other-cluster-incr", withCluster("cluster2"), withCompletedAt(20)),
			},
			input: fullBcp("base"),
			want:  []string{"incr-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := ListDependentIncrementalBackups(t.Context(), cl, tt.input)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got, "the result must be an empty slice, never nil")
			assert.Equal(t, tt.want, backupNames(got))
		})
	}
}

func TestResolveIncrementalBase(t *testing.T) {
	tests := []struct {
		name     string
		objs     []client.Object
		cr       *api.PerconaXtraDBClusterBackup
		storage  *api.BackupStorageSpec
		want     string
		wantErr  string
		wantErrs error
	}{
		{
			name: "explicit base name resolves",
			objs: []client.Object{
				fullBcp("pinned-full", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("newer-full", withS3("bucket-a"), withCompletedAt(40)),
			},
			cr:      incrBcp("incr-1", withSpecBase("pinned-full"), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			want:    "pinned-full",
		},
		{
			name: "explicit base name pointing at an incremental is rejected",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withCompletedAt(10)),
				incrBcp("incr-base", withS3("bucket-a"), withCompletedAt(20)),
			},
			cr:      incrBcp("incr-2", withSpecBase("incr-base"), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			wantErr: `base backup test-ns/incr-base is of type "incremental", only a full backup can be the base of a chain`,
		},
		{
			name: "explicit base name that does not exist",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withCompletedAt(10)),
			},
			cr:      incrBcp("incr-1", withSpecBase("missing-full"), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			wantErr: "get base backup test-ns/missing-full",
		},
		{
			name: "explicit base that has not succeeded",
			objs: []client.Object{
				fullBcp("failed-full", withS3("bucket-a"), withCompletedAt(10), withState(api.BackupFailed)),
			},
			cr:      incrBcp("incr-1", withSpecBase("failed-full"), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			wantErr: `base backup test-ns/failed-full is not succeeded, its state is "Failed"`,
		},
		{
			name: "explicit base in a different storage",
			objs: []client.Object{
				fullBcp("full-elsewhere", withS3("bucket-b"), withCompletedAt(10)),
			},
			cr:      incrBcp("incr-1", withSpecBase("full-elsewhere"), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			wantErr: "base backup test-ns/full-elsewhere is in storage",
		},
		{
			name: "explicit base in a different storage backend",
			objs: []client.Object{
				fullBcp("full-azure", withAzure("container-a"), withCompletedAt(10)),
			},
			cr:      incrBcp("incr-1", withSpecBase("full-azure"), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			wantErr: "base backup test-ns/full-azure is in storage",
		},
		{
			name: "empty explicit base name falls back to implicit resolution",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("full-2", withS3("bucket-a"), withCompletedAt(20)),
			},
			cr:      incrBcp("incr-1", withSpecBase(""), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			want:    "full-2",
		},
		{
			name: "implicit resolution picks the latest full backup in the storage",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("full-2", withS3("bucket-a"), withCompletedAt(30)),
				fullBcp("full-newer-elsewhere", withS3("bucket-b"), withCompletedAt(40)),
				incrBcp("incr-newest", withS3("bucket-a"), withCompletedAt(50)),
			},
			cr:      incrBcp("incr-1", withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			want:    "full-2",
		},
		{
			name: "implicit resolution with no full backup returns the sentinel",
			objs: []client.Object{
				incrBcp("incr-1", withS3("bucket-a"), withCompletedAt(10)),
			},
			cr:       incrBcp("incr-2", withS3("bucket-a")),
			storage:  s3Storage("bucket-a"),
			wantErrs: ErrNoFullBackupFound,
		},
		{
			name: "nil storage skips the storage check",
			objs: []client.Object{
				fullBcp("full-azure", withAzure("container-a"), withCompletedAt(10)),
			},
			cr:      incrBcp("incr-1", withSpecBase("full-azure"), withS3("bucket-a")),
			storage: nil,
			want:    "full-azure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := ResolveIncrementalBase(t.Context(), cl, tt.cr, testCluster(), tt.storage)

			switch {
			case tt.wantErrs != nil:
				require.ErrorIs(t, err, tt.wantErrs)
				assert.Nil(t, got)
			case tt.wantErr != "":
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
			default:
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.want, got.Name)
			}
		})
	}
}

func TestStorageSpecFromStatus(t *testing.T) {
	tests := []struct {
		name   string
		status *api.PXCBackupStatus
		want   *api.BackupStorageSpec
	}{
		{
			name:   "nil status",
			status: nil,
			want:   nil,
		},
		{
			name:   "empty status",
			status: &api.PXCBackupStatus{},
			want:   &api.BackupStorageSpec{},
		},
		{
			name: "s3 status",
			status: &api.PXCBackupStatus{
				StorageType: api.BackupStorageS3,
				S3: &api.BackupStorageS3Spec{
					Bucket:      "bucket-a",
					Prefix:      "pfx",
					Region:      "us-west-1",
					EndpointURL: "https://s3.example.com",
				},
			},
			want: &api.BackupStorageSpec{
				Type: api.BackupStorageS3,
				S3: &api.BackupStorageS3Spec{
					Bucket:      "bucket-a",
					Prefix:      "pfx",
					Region:      "us-west-1",
					EndpointURL: "https://s3.example.com",
				},
			},
		},
		{
			name: "azure status",
			status: &api.PXCBackupStatus{
				StorageType: api.BackupStorageAzure,
				Azure: &api.BackupStorageAzureSpec{
					ContainerPath: "container-a",
					Prefix:        "pfx",
					Endpoint:      "https://azure.example.com",
				},
			},
			want: &api.BackupStorageSpec{
				Type: api.BackupStorageAzure,
				Azure: &api.BackupStorageAzureSpec{
					ContainerPath: "container-a",
					Prefix:        "pfx",
					Endpoint:      "https://azure.example.com",
				},
			},
		},
		{
			name: "pvc status",
			status: &api.PXCBackupStatus{
				StorageType: api.BackupStorageFilesystem,
				PVC:         &corev1.PersistentVolumeClaimSpec{StorageClassName: ptr.To("standard")},
			},
			want: &api.BackupStorageSpec{
				Type: api.BackupStorageFilesystem,
				Volume: &api.VolumeSpec{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{StorageClassName: ptr.To("standard")},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StorageSpecFromStatus(tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestStorageSpecFromStatusEquality locks in the property every caller actually
// relies on: two backups that went to the same place compare equal through
// (*api.BackupStorageSpec).Equals, and two that did not do not.
func TestStorageSpecFromStatusEquality(t *testing.T) {
	tests := []struct {
		name    string
		storage *api.BackupStorageSpec
		status  *api.PXCBackupStatus
		want    bool
	}{
		{
			name:    "same s3 bucket",
			storage: s3Storage("bucket-a"),
			status:  &fullBcp("x", withS3("bucket-a")).Status,
			want:    true,
		},
		{
			name:    "different s3 bucket",
			storage: s3Storage("bucket-a"),
			status:  &fullBcp("x", withS3("bucket-b")).Status,
			want:    false,
		},
		{
			name:    "s3 against azure",
			storage: s3Storage("bucket-a"),
			status:  &fullBcp("x", withAzure("container-a")).Status,
			want:    false,
		},
		{
			name:    "same azure container",
			storage: azureStorage("container-a"),
			status:  &fullBcp("x", withAzure("container-a")).Status,
			want:    true,
		},
		{
			name:    "storage against a status with no storage recorded",
			storage: s3Storage("bucket-a"),
			status:  &api.PXCBackupStatus{},
			want:    false,
		},
		{
			name:    "filesystem never compares equal",
			storage: &api.BackupStorageSpec{Type: api.BackupStorageFilesystem},
			status:  &api.PXCBackupStatus{StorageType: api.BackupStorageFilesystem},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.storage.Equals(StorageSpecFromStatus(tt.status)))
		})
	}
}

// TestChainKey covers the chain identifier derivation directly, since both chain
// queries route through it.
func TestChainKey(t *testing.T) {
	tests := []struct {
		name    string
		backup  *api.PerconaXtraDBClusterBackup
		want    string
		wantErr string
	}{
		{
			name:   "full backup uses its own destination leaf name",
			backup: fullBcp("base"),
			want:   baseLeaf,
		},
		{
			name:   "full backup with an azure destination",
			backup: fullBcp("base", withDestination("azure://container-a/pfx/"+baseLeaf)),
			want:   baseLeaf,
		},
		{
			name:   "full backup with no destination yet",
			backup: fullBcp("base", withDestination("")),
			want:   "",
		},
		{
			name:   "legacy backup with an empty status type counts as full",
			backup: fullBcp("base", withStatusType("")),
			want:   baseLeaf,
		},
		{
			name:   "incremental uses the persisted annotation",
			backup: incrBcp("incr-1"),
			want:   baseLeaf,
		},
		{
			name:    "incremental without the annotation",
			backup:  incrBcp("incr-1", withoutAnnotations()),
			wantErr: "incremental backup test-ns/incr-1 has no percona.com/base-backup-name annotation",
		},
		{
			name:    "incremental with an empty annotation value",
			backup:  incrBcp("incr-1", withBaseAnnotation("")),
			wantErr: "incremental backup test-ns/incr-1 has no percona.com/base-backup-name annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chainKey(tt.backup)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetBackupByDestinationLeaf(t *testing.T) {
	tests := []struct {
		name     string
		objs     []client.Object
		leaf     string
		want     string
		wantErrs error
	}{
		{
			name: "finds the backup whose destination ends in the leaf",
			objs: []client.Object{
				fullBcp("full-1", withDestination("s3://bucket-a/"+baseLeaf)),
			},
			leaf: baseLeaf,
			want: "full-1",
		},
		{
			name: "picks the right one out of several",
			objs: []client.Object{
				fullBcp("full-older", withDestination("s3://bucket-a/"+otherBaseLeaf)),
				fullBcp("full-wanted", withDestination("s3://bucket-a/"+baseLeaf)),
				incrBcp("incr-1"),
			},
			leaf: baseLeaf,
			want: "full-wanted",
		},
		{
			name: "matches through a nested storage prefix",
			objs: []client.Object{
				fullBcp("full-1", withDestination("s3://bucket-a/pfx/subdir/"+baseLeaf)),
			},
			leaf: baseLeaf,
			want: "full-1",
		},
		{
			name: "matches an azure destination",
			objs: []client.Object{
				fullBcp("full-1", withAzure("container-a"), withDestination("azure://container-a/"+baseLeaf)),
			},
			leaf: baseLeaf,
			want: "full-1",
		},
		{
			name: "no backup carries that leaf",
			objs: []client.Object{
				fullBcp("full-1", withDestination("s3://bucket-a/"+otherBaseLeaf)),
			},
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "an empty leaf matches nothing",
			objs: []client.Object{
				fullBcp("full-1", withDestination("s3://bucket-a/"+baseLeaf)),
			},
			leaf:     "",
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "a backup with no destination is never a match",
			objs: []client.Object{
				fullBcp("full-no-dest", withDestination("")),
			},
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "a backup of another cluster is not a match",
			objs: []client.Object{
				fullBcp("full-elsewhere",
					withCluster("cluster2"),
					withDestination("s3://bucket-a/"+baseLeaf),
				),
			},
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "an incremental backup is returned when its leaf matches",
			objs: []client.Object{
				incrBcp("incr-1", withDestination("s3://bucket-a/"+baseLeaf+".incr/incr-1-incr")),
			},
			leaf: "incr-1-incr",
			want: "incr-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := GetBackupByDestinationLeaf(t.Context(), cl, testCluster(), tt.leaf)

			if tt.wantErrs != nil {
				require.ErrorIs(t, err, tt.wantErrs)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.Name)
		})
	}
}

// TestGetBackupByDestinationLeafReturnsACopy makes sure callers cannot reach
// back into the lister's result. setIncrementalBase mutates the backup it gets
// back, and doing that to shared memory would corrupt an unrelated object.
func TestGetBackupByDestinationLeafReturnsACopy(t *testing.T) {
	stored := fullBcp("full-1", withDestination("s3://bucket-a/"+baseLeaf))
	cl := incrFakeClient(t, stored)

	got, err := GetBackupByDestinationLeaf(t.Context(), cl, testCluster(), baseLeaf)
	require.NoError(t, err)

	got.Status.SourcePod = "mutated"

	reread, err := GetBackupByDestinationLeaf(t.Context(), cl, testCluster(), baseLeaf)
	require.NoError(t, err)
	assert.Empty(t, reread.Status.SourcePod)
}
