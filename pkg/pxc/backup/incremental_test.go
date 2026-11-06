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
	withS3("bucket-a")(b)
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
	withS3("bucket-a")(b)
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
	// Azure storage comparison reads the account name out of the storage's
	// credentials secret.
	require.NoError(t, corev1.AddToScheme(scheme))

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
			name: "creation order has no bearing on the result",
			backups: []api.PerconaXtraDBClusterBackup{
				*incrBcp("ran-second", withCreatedAt(5), withCompletedAt(30)),
				*incrBcp("ran-first", withCreatedAt(20), withCompletedAt(10)),
			},
			want: []string{"ran-first", "ran-second"},
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
				*incrBcp("only", withCompletedAt(10)),
			},
			want: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortBackupsByCompletion(tt.backups)
			assert.Equal(t, tt.want, backupNames(tt.backups))
		})
	}
}

func TestSucceededChainMembers(t *testing.T) {
	tests := []struct {
		name    string
		members []api.PerconaXtraDBClusterBackup
		want    []string
		wantErr string
	}{
		{
			name:    "no members",
			members: []api.PerconaXtraDBClusterBackup{},
			want:    []string{},
		},
		{
			name: "succeeded members are kept in the order given",
			members: []api.PerconaXtraDBClusterBackup{
				*incrBcp("b", withCompletedAt(20)),
				*incrBcp("a", withCompletedAt(10)),
			},
			want: []string{"b", "a"},
		},
		{
			name: "a running member is dropped",
			members: []api.PerconaXtraDBClusterBackup{
				*incrBcp("done", withCompletedAt(10)),
				*incrBcp("running", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupRunning)),
			},
			want: []string{"done"},
		},
		{
			name: "a new member is dropped",
			members: []api.PerconaXtraDBClusterBackup{
				*incrBcp("done", withCompletedAt(10)),
				*incrBcp("queued", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupNew)),
			},
			want: []string{"done"},
		},
		{
			name: "a failed member is dropped",
			members: []api.PerconaXtraDBClusterBackup{
				*incrBcp("done", withCompletedAt(10)),
				*incrBcp("failed", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupFailed)),
			},
			want: []string{"done"},
		},
		{
			name: "a succeeded member with no completion time is an error",
			members: []api.PerconaXtraDBClusterBackup{
				*incrBcp("done", withCompletedAt(10)),
				*incrBcp("impossible", withoutCompletedAt()),
			},
			wantErr: "backup test-ns/impossible is succeeded but completedAt is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := succeededChainMembers(tt.members)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, backupNames(got))
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
				azureSecret("creds", "account-a"),
				fullBcp("full-s3", withS3("bucket-a"), withCompletedAt(30)),
				fullBcp("full-azure", withAzureCreds("container-a", "creds"), withCompletedAt(10)),
			},
			storage: azureStorageWithCreds("container-a", "creds"),
			want:    "full-azure",
		},
		{
			name: "an azure backup in another storage account is not a candidate",
			objs: []client.Object{
				azureSecret("live-creds", "account-a"),
				azureSecret("other-creds", "account-b"),
				fullBcp("full-here", withAzureCreds("container-a", "live-creds"), withCompletedAt(10)),
				fullBcp("full-elsewhere", withAzureCreds("container-a", "other-creds"), withCompletedAt(30)),
			},
			storage: azureStorageWithCreds("container-a", "live-creds"),
			want:    "full-here",
		},
		{
			name: "nil storage matches no storage",
			objs: []client.Object{
				fullBcp("full-s3", withS3("bucket-a"), withCompletedAt(10)),
				fullBcp("full-azure", withAzure("container-a"), withCompletedAt(30)),
			},
			storage:  nil,
			wantErrs: ErrNoFullBackupFound,
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
		name     string
		objs     []client.Object
		input    *api.PerconaXtraDBClusterBackup
		want     string
		wantErr  string
		wantErrs error
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
			name: "a failed tail is skipped",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-failed", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupFailed)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
		},
		{
			name: "a running tail is skipped: it has no LSN to hand over yet",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-running", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupRunning)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
		},
		{
			name: "a chain of nothing but unfinished members is empty",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1-running", withoutCompletedAt(), withCreatedAt(10), withState(api.BackupRunning)),
				incrBcp("incr-2-failed", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupFailed)),
			},
			input:    fullBcp("base"),
			wantErrs: ErrNoIncrementalBackupFound,
		},
		{
			name: "a succeeded member with no completion time is an error",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withoutCompletedAt()),
			},
			input:   fullBcp("base"),
			wantErr: "backup test-ns/incr-2 is succeeded but completedAt is not set",
		},
		{
			name: "a member of a same-keyed chain in another storage is excluded",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-elsewhere", withS3("bucket-b"), withCompletedAt(40)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
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

			got, err := GetLatestIncrementalBackupInChain(t.Context(), cl, tt.input)

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

func TestNextDeletableDependent(t *testing.T) {
	tests := []struct {
		name    string
		objs    []client.Object
		input   *api.PerconaXtraDBClusterBackup
		want    string
		wantErr string
	}{
		{
			name:  "no dependents",
			objs:  []client.Object{fullBcp("base")},
			input: fullBcp("base"),
			want:  "",
		},
		{
			name: "the tail of a wholly succeeded chain goes first",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-3", withCompletedAt(30)),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
			},
			input: fullBcp("base"),
			want:  "incr-3",
		},
		{
			name: "a running member goes before the succeeded tail",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
				incrBcp("incr-3-running", withoutCompletedAt(), withCreatedAt(30), withState(api.BackupRunning)),
			},
			input: fullBcp("base"),
			want:  "incr-3-running",
		},
		{
			name: "a running member created before the tail completed still goes first",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCreatedAt(10), withCompletedAt(50)),
				incrBcp("incr-2-running", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupRunning)),
			},
			input: fullBcp("base"),
			want:  "incr-2-running",
		},
		{
			name: "a failed member goes before the succeeded tail",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-failed", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupFailed)),
			},
			input: fullBcp("base"),
			want:  "incr-2-failed",
		},
		{
			name: "a member of a same-keyed chain in another storage is not a dependent",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-elsewhere", withS3("bucket-b"), withCompletedAt(40)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
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
			want:  "incr-1",
		},
		{
			name: "full backups are never dependents",
			objs: []client.Object{
				fullBcp("base"),
				fullBcp("another-full", withBaseAnnotation(baseLeaf), withCompletedAt(20)),
				incrBcp("incr-1", withCompletedAt(10)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
		},
		{
			name: "an incremental has no dependents",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
			},
			input: incrBcp("incr-1", withCompletedAt(10)),
			want:  "",
		},
		{
			name: "full backup without a destination has no dependents",
			objs: []client.Object{
				incrBcp("incr-1", withCompletedAt(10)),
			},
			input: fullBcp("base", withDestination("")),
			want:  "",
		},
		{
			name: "dependents of another cluster are excluded",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("other-cluster-incr", withCluster("cluster2"), withCompletedAt(20)),
			},
			input: fullBcp("base"),
			want:  "incr-1",
		},
		{
			name: "a succeeded member with no completion time is an error",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withoutCompletedAt()),
			},
			input:   fullBcp("base"),
			wantErr: "backup test-ns/incr-2 is succeeded but completedAt is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := NextDeletableDependent(t.Context(), cl, tt.input)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.want == "" {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.Name)
		})
	}
}

func TestHasUnfinishedChainMember(t *testing.T) {
	tests := []struct {
		name    string
		objs    []client.Object
		input   *api.PerconaXtraDBClusterBackup
		want    bool
		wantErr string
	}{
		{
			name:  "an empty chain has nothing unfinished",
			objs:  []client.Object{fullBcp("base")},
			input: fullBcp("base"),
			want:  false,
		},
		{
			name: "a wholly succeeded chain has nothing unfinished",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2", withCompletedAt(20)),
			},
			input: fullBcp("base"),
			want:  false,
		},
		{
			name: "a running member is unfinished",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-running", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupRunning)),
			},
			input: fullBcp("base"),
			want:  true,
		},
		{
			name: "a member still waiting on the lease is unfinished",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-new", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupNew)),
			},
			input: fullBcp("base"),
			want:  true,
		},
		{
			name: "a suspended member is unfinished: it can still resume",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-suspended", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupSuspended)),
			},
			input: fullBcp("base"),
			want:  true,
		},
		{
			// Terminal, so it will never be appended to the chain. Treating it
			// as unfinished would block the real tail from ever being deleted.
			name: "a failed member is finished",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-failed", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupFailed)),
			},
			input: fullBcp("base"),
			want:  false,
		},
		{
			name: "an unfinished member of another base does not count",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("other-running", withBaseAnnotation(otherBaseLeaf), withoutCompletedAt(), withState(api.BackupRunning)),
			},
			input: fullBcp("base"),
			want:  false,
		},
		{
			name: "an unfinished member of a same-keyed chain in another storage does not count",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-elsewhere-running", withS3("bucket-b"), withoutCompletedAt(), withState(api.BackupRunning)),
			},
			input: fullBcp("base"),
			want:  false,
		},
		{
			name: "an unfinished member of another cluster does not count",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("other-cluster-running", withCluster("cluster2"), withoutCompletedAt(), withState(api.BackupRunning)),
			},
			input: fullBcp("base"),
			want:  false,
		},
		{
			name: "an incremental resolves the same chain as its base",
			objs: []client.Object{
				fullBcp("base"),
				incrBcp("incr-1", withCompletedAt(10)),
				incrBcp("incr-2-running", withoutCompletedAt(), withCreatedAt(20), withState(api.BackupRunning)),
			},
			input: incrBcp("incr-1", withCompletedAt(10)),
			want:  true,
		},
		{
			name: "an incremental without the annotation is an error",
			objs: []client.Object{
				fullBcp("base"),
			},
			input:   incrBcp("orphan", withoutAnnotations()),
			wantErr: "incremental backup test-ns/orphan has no percona.com/base-backup-name annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := HasUnfinishedChainMember(t.Context(), cl, tt.input)

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
			name: "explicit base name from another cluster is rejected",
			objs: []client.Object{
				fullBcp("full-elsewhere",
					withCluster("cluster2"),
					withS3("bucket-a"),
					withCompletedAt(10),
				),
			},
			cr:      incrBcp("incr-1", withSpecBase("full-elsewhere"), withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			wantErr: "belongs to cluster",
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
			name: "nil storage matches no storage",
			objs: []client.Object{
				fullBcp("full-azure", withAzure("container-a"), withCompletedAt(10)),
			},
			cr:      incrBcp("incr-1", withSpecBase("full-azure"), withS3("bucket-a")),
			storage: nil,
			wantErr: "is in storage",
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
			got := tt.status.IntoBackupStorageSpec()
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
			assert.Equal(t, tt.want, tt.storage.Equals(tt.status.IntoBackupStorageSpec()))
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
		storage  *api.BackupStorageSpec
		leaf     string
		want     string
		wantErr  string
		wantErrs error
	}{
		{
			name: "finds the backup whose destination ends in the leaf",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf)),
			},
			storage: s3Storage("bucket-a"),
			leaf:    baseLeaf,
			want:    "full-1",
		},
		{
			name: "picks the right one out of several",
			objs: []client.Object{
				fullBcp("full-older", withS3("bucket-a"), withDestination("s3://bucket-a/"+otherBaseLeaf)),
				fullBcp("full-wanted", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf)),
				incrBcp("incr-1", withS3("bucket-a")),
			},
			storage: s3Storage("bucket-a"),
			leaf:    baseLeaf,
			want:    "full-wanted",
		},
		{
			name: "matches through a nested storage prefix",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withDestination("s3://bucket-a/pfx/subdir/"+baseLeaf)),
			},
			storage: s3Storage("bucket-a"),
			leaf:    baseLeaf,
			want:    "full-1",
		},
		{
			name: "matches an azure destination",
			objs: []client.Object{
				azureSecret("creds", "account-a"),
				fullBcp("full-1", withAzureCreds("container-a", "creds"), withDestination("azure://container-a/"+baseLeaf)),
			},
			storage: azureStorageWithCreds("container-a", "creds"),
			leaf:    baseLeaf,
			want:    "full-1",
		},
		{
			name: "no backup carries that leaf",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withDestination("s3://bucket-a/"+otherBaseLeaf)),
			},
			storage:  s3Storage("bucket-a"),
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "an empty leaf matches nothing",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf)),
			},
			storage:  s3Storage("bucket-a"),
			leaf:     "",
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "a backup with no destination is never a match",
			objs: []client.Object{
				fullBcp("full-no-dest", withS3("bucket-a"), withDestination("")),
			},
			storage:  s3Storage("bucket-a"),
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "a backup of another cluster is not a match",
			objs: []client.Object{
				fullBcp("full-elsewhere",
					withCluster("cluster2"),
					withS3("bucket-a"),
					withDestination("s3://bucket-a/"+baseLeaf),
				),
			},
			storage:  s3Storage("bucket-a"),
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "an incremental backup is returned when its leaf matches",
			objs: []client.Object{
				incrBcp("incr-1", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf+".incr/incr-1-incr")),
			},
			storage: s3Storage("bucket-a"),
			leaf:    "incr-1-incr",
			want:    "incr-1",
		},
		{
			name: "the same leaf in another storage is not a match",
			objs: []client.Object{
				fullBcp("full-elsewhere", withS3("bucket-b"), withDestination("s3://bucket-b/"+baseLeaf)),
			},
			storage:  s3Storage("bucket-a"),
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "of two backups sharing a leaf the one in this storage is taken",
			objs: []client.Object{
				fullBcp("full-here", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf)),
				fullBcp("full-elsewhere", withS3("bucket-b"), withDestination("s3://bucket-b/"+baseLeaf)),
			},
			storage: s3Storage("bucket-a"),
			leaf:    baseLeaf,
			want:    "full-here",
		},
		{
			name: "the same leaf in another azure container is not a match",
			objs: []client.Object{
				azureSecret("creds", "account-a"),
				fullBcp("full-elsewhere", withAzureCreds("container-b", "creds"), withDestination("azure://container-b/"+baseLeaf)),
			},
			storage:  azureStorageWithCreds("container-a", "creds"),
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "the same leaf in the same container of another account is not a match",
			objs: []client.Object{
				azureSecret("live-creds", "account-a"),
				azureSecret("other-creds", "account-b"),
				fullBcp("full-elsewhere", withAzureCreds("container-a", "other-creds"), withDestination("azure://container-a/"+baseLeaf)),
			},
			storage:  azureStorageWithCreds("container-a", "live-creds"),
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
		{
			name: "two backups claiming one destination is an error, not a guess",
			objs: []client.Object{
				fullBcp("full-a", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf)),
				fullBcp("full-b", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf)),
			},
			storage: s3Storage("bucket-a"),
			leaf:    baseLeaf,
			wantErr: "both claim the destination",
		},
		{
			name: "without a storage nothing matches",
			objs: []client.Object{
				fullBcp("full-1", withS3("bucket-b"), withDestination("s3://bucket-b/"+baseLeaf)),
			},
			storage:  nil,
			leaf:     baseLeaf,
			wantErrs: ErrBaseBackupNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := GetBackupByDestinationLeaf(t.Context(), cl, testCluster(), tt.storage, tt.leaf)

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

// TestGetBackupByDestinationLeafReturnsACopy makes sure callers cannot reach
// back into the lister's result. setIncrementalBase mutates the backup it gets
// back, and doing that to shared memory would corrupt an unrelated object.
func TestGetBackupByDestinationLeafReturnsACopy(t *testing.T) {
	stored := fullBcp("full-1", withS3("bucket-a"), withDestination("s3://bucket-a/"+baseLeaf))
	cl := incrFakeClient(t, stored)
	storage := s3Storage("bucket-a")

	got, err := GetBackupByDestinationLeaf(t.Context(), cl, testCluster(), storage, baseLeaf)
	require.NoError(t, err)

	got.Status.SourcePod = "mutated"

	reread, err := GetBackupByDestinationLeaf(t.Context(), cl, testCluster(), storage, baseLeaf)
	require.NoError(t, err)
	assert.Empty(t, reread.Status.SourcePod)
}

// azureSecret builds a credentials secret holding the storage account name,
// which is the only place that name exists: it is not a field of the CRD.
func azureSecret(name, account string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: incrTestNamespace},
		Data: map[string][]byte{
			api.AzureStorageAccountNameKey: []byte(account),
			"AZURE_STORAGE_ACCOUNT_KEY":    []byte("key"),
		},
	}
}

// azureStorageWithCreds is azureStorage with a credentials secret, so that the
// storage account behind it can be resolved.
func azureStorageWithCreds(container, secret string) *api.BackupStorageSpec {
	stg := azureStorage(container)
	stg.Azure.CredentialsSecret = secret
	return stg
}

// withAzureCreds records a credentials secret alongside the azure storage a
// backup landed in, mirroring what prepareStatus copies into the status.
func withAzureCreds(container, secret string) bcpOption {
	return func(b *api.PerconaXtraDBClusterBackup) {
		withAzure(container)(b)
		b.Status.Azure.CredentialsSecret = secret
	}
}

func TestIsUsingSameStorage(t *testing.T) {
	tests := []struct {
		name    string
		objs    []client.Object
		bcp     *api.PerconaXtraDBClusterBackup
		storage *api.BackupStorageSpec
		want    bool
		wantErr string
	}{
		{
			name:    "a nil storage matches nothing",
			bcp:     fullBcp("x", withS3("bucket-a")),
			storage: nil,
			want:    false,
		},
		{
			name:    "a nil backup matches nothing",
			bcp:     nil,
			storage: s3Storage("bucket-a"),
			want:    false,
		},
		{
			name:    "same s3 bucket",
			bcp:     fullBcp("x", withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			want:    true,
		},
		{
			name:    "different s3 bucket",
			bcp:     fullBcp("x", withS3("bucket-b")),
			storage: s3Storage("bucket-a"),
			want:    false,
		},
		{
			// S3 bucket names are unique within an endpoint, so no secret is
			// read and a missing one cannot fail the comparison.
			name:    "s3 never reads a secret",
			bcp:     fullBcp("x", withS3("bucket-a")),
			storage: s3Storage("bucket-a"),
			want:    true,
		},
		{
			name: "same azure container in the same storage account",
			objs: []client.Object{
				azureSecret("live-creds", "account-a"),
				azureSecret("recorded-creds", "account-a"),
			},
			bcp:     fullBcp("x", withAzureCreds("container-a", "recorded-creds")),
			storage: azureStorageWithCreds("container-a", "live-creds"),
			want:    true,
		},
		{
			name: "same azure container in different storage accounts",
			objs: []client.Object{
				azureSecret("live-creds", "account-a"),
				azureSecret("recorded-creds", "account-b"),
			},
			bcp:     fullBcp("x", withAzureCreds("container-a", "recorded-creds")),
			storage: azureStorageWithCreds("container-a", "live-creds"),
			want:    false,
		},
		{
			name: "one azure account reached through two different secrets",
			objs: []client.Object{
				azureSecret("creds", "account-a"),
				azureSecret("creds-copy", "account-a"),
			},
			bcp:     fullBcp("x", withAzureCreds("container-a", "creds-copy")),
			storage: azureStorageWithCreds("container-a", "creds"),
			want:    true,
		},
		{
			name: "different azure containers in one account still differ",
			objs: []client.Object{
				azureSecret("creds", "account-a"),
			},
			bcp:     fullBcp("x", withAzureCreds("container-b", "creds")),
			storage: azureStorageWithCreds("container-a", "creds"),
			want:    false,
		},
		{
			name:    "an azure storage with no credentials secret is an error",
			bcp:     fullBcp("x", withAzure("container-a")),
			storage: azureStorage("container-a"),
			wantErr: "azure storage has no credentialsSecret",
		},
		{
			name: "a missing azure credentials secret is an error",
			bcp:  fullBcp("x", withAzureCreds("container-a", "absent-creds")),

			storage: azureStorageWithCreds("container-a", "absent-creds"),
			wantErr: `secrets "absent-creds" not found`,
		},
		{
			name: "an azure credentials secret without the account name is an error",
			objs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "blank-creds", Namespace: incrTestNamespace},
					Data:       map[string][]byte{"AZURE_STORAGE_ACCOUNT_KEY": []byte("key")},
				},
			},
			bcp:     fullBcp("x", withAzureCreds("container-a", "blank-creds")),
			storage: azureStorageWithCreds("container-a", "blank-creds"),
			wantErr: "azure storage account name is not found in the credentials secret",
		},
		{
			// The types differ, so there is nothing an account could settle.
			name: "azure storage against an s3 backup reads no secret",
			objs: []client.Object{
				azureSecret("live-creds", "account-a"),
			},
			bcp:     fullBcp("x", withS3("bucket-a")),
			storage: azureStorageWithCreds("container-a", "live-creds"),
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := incrFakeClient(t, tt.objs...)

			got, err := tt.bcp.IsUsingSameStorage(t.Context(), cl, tt.storage)

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
