package backup

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

var (
	ErrNoFullBackupFound        = errors.New("no full backup found in storage")
	ErrNoIncrementalBackupFound = errors.New("no incremental backup found in chain")
)

func StorageSpecFromStatus(status *api.PXCBackupStatus) *api.BackupStorageSpec {
	if status == nil {
		return nil
	}

	spec := &api.BackupStorageSpec{
		Type:  status.StorageType,
		S3:    status.S3,
		Azure: status.Azure,
	}
	if status.PVC != nil {
		spec.Volume = &api.VolumeSpec{PersistentVolumeClaim: status.PVC}
	}

	return spec
}

// sortBackupsByCompletion sorts backups in place, oldest first, by
// Status.CompletedAt, falling back to CreationTimestamp when CompletedAt is nil.
func sortBackupsByCompletion(backups []api.PerconaXtraDBClusterBackup) {
	sort.SliceStable(backups, func(i, j int) bool {
		return completionTime(&backups[i]).Before(completionTime(&backups[j]))
	})
}

// completionTime returns the time a backup finished, or its creation time when
// it has not finished yet. The result is never nil.
func completionTime(bcp *api.PerconaXtraDBClusterBackup) *metav1.Time {
	if bcp.Status.CompletedAt != nil {
		return bcp.Status.CompletedAt
	}
	return &bcp.CreationTimestamp
}

// GetLastFullBackup returns the newest succeeded full backup of the cluster that
// landed in the given storage, or ErrNoFullBackupFound when there is none.
func GetLastFullBackup(
	ctx context.Context,
	cl client.Client,
	cluster *api.PerconaXtraDBCluster,
	storage *api.BackupStorageSpec,
) (*api.PerconaXtraDBClusterBackup, error) {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := cl.List(ctx, &bcpList,
		client.InNamespace(cluster.GetNamespace()),
		client.MatchingFields{PXCClusterBackupField: cluster.GetName()},
	); err != nil {
		return nil, errors.Wrapf(err, "list backups of cluster %s/%s", cluster.GetNamespace(), cluster.GetName())
	}

	candidates := make([]api.PerconaXtraDBClusterBackup, 0, len(bcpList.Items))
	for _, bcp := range bcpList.Items {
		if bcp.GetType() != api.BackupTypeFull {
			continue
		}
		if bcp.Status.State != api.BackupSucceeded {
			continue
		}
		if storage != nil && !storage.Equals(StorageSpecFromStatus(&bcp.Status)) {
			continue
		}
		if bcp.Status.CompletedAt == nil {
			return nil, errors.Errorf("backup %s/%s is succeeded but completedAt is not set", bcp.GetNamespace(), bcp.GetName())
		}
		candidates = append(candidates, bcp)
	}

	if len(candidates) == 0 {
		return nil, ErrNoFullBackupFound
	}

	sortBackupsByCompletion(candidates)

	return candidates[len(candidates)-1].DeepCopy(), nil
}

// GetLatestIncrementalBackupInChain returns the newest incremental backup in the
// chain that the given backup belongs to, or ErrNoIncrementalBackupFound when
// the chain is still empty.
func GetLatestIncrementalBackupInChain(
	ctx context.Context,
	cl client.Client,
	backup *api.PerconaXtraDBClusterBackup,
	onlySucceeded bool,
) (*api.PerconaXtraDBClusterBackup, error) {
	key, err := chainKey(backup)
	if err != nil {
		return nil, err
	}
	if key == "" {
		// A full backup that never recorded a destination owns no chain yet.
		return nil, ErrNoIncrementalBackupFound
	}

	members, err := listChainMembers(ctx, cl, backup, key, onlySucceeded)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, ErrNoIncrementalBackupFound
	}

	sortBackupsByCompletion(members)

	return members[len(members)-1].DeepCopy(), nil
}

// ListDependentIncrementalBackups returns every incremental backup chained to
// the given full backup, oldest first, including unfinished ones.
func ListDependentIncrementalBackups(
	ctx context.Context,
	cl client.Client,
	fullBackup *api.PerconaXtraDBClusterBackup,
) ([]api.PerconaXtraDBClusterBackup, error) {
	if fullBackup.GetType() != api.BackupTypeFull {
		return []api.PerconaXtraDBClusterBackup{}, nil
	}

	key := fullBackup.Status.Destination.BackupName()
	if key == "" {
		// Nothing can be chained to a backup that never recorded a destination.
		return []api.PerconaXtraDBClusterBackup{}, nil
	}

	members, err := listChainMembers(ctx, cl, fullBackup, key, false)
	if err != nil {
		return nil, err
	}

	sortBackupsByCompletion(members)

	return members, nil
}

// ResolveIncrementalBase returns the full backup that cr should be chained to.
func ResolveIncrementalBase(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
	storage *api.BackupStorageSpec,
) (*api.PerconaXtraDBClusterBackup, error) {
	base, err := resolveBaseCandidate(ctx, cl, cr, cluster, storage)
	if err != nil {
		return nil, err
	}

	if base.Status.State != api.BackupSucceeded {
		return nil, errors.Errorf("base backup %s/%s is not succeeded, its state is %q",
			base.GetNamespace(), base.GetName(), base.Status.State)
	}

	if storage != nil && !storage.Equals(StorageSpecFromStatus(&base.Status)) {
		return nil, errors.Errorf("base backup %s/%s is in storage %q, which is not the storage of incremental backup %s/%s",
			base.GetNamespace(), base.GetName(), base.Status.StorageName, cr.GetNamespace(), cr.GetName())
	}

	return base, nil
}

func resolveBaseCandidate(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
	storage *api.BackupStorageSpec,
) (*api.PerconaXtraDBClusterBackup, error) {
	if cr.Spec.IncrementalBaseBackupName == nil || *cr.Spec.IncrementalBaseBackupName == "" {
		base, err := GetLastFullBackup(ctx, cl, cluster, storage)
		if err != nil {
			return nil, errors.Wrap(err, "get last full backup")
		}
		return base, nil
	}

	nn := types.NamespacedName{Namespace: cr.GetNamespace(), Name: *cr.Spec.IncrementalBaseBackupName}

	base := &api.PerconaXtraDBClusterBackup{}
	if err := cl.Get(ctx, nn, base); err != nil {
		return nil, errors.Wrapf(err, "get base backup %s", nn)
	}

	if base.GetType() != api.BackupTypeFull {
		return nil, errors.Errorf("base backup %s is of type %q, only a full backup can be the base of a chain",
			nn, base.GetType())
	}

	return base, nil
}

func chainKey(backup *api.PerconaXtraDBClusterBackup) (string, error) {
	if backup.GetType() == api.BackupTypeFull {
		return backup.Status.Destination.BackupName(), nil
	}

	key := backup.GetAnnotations()[naming.AnnotationBaseBackupName]
	if key == "" {
		return "", errors.Errorf("incremental backup %s/%s has no %s annotation, its chain cannot be resolved",
			backup.GetNamespace(), backup.GetName(), naming.AnnotationBaseBackupName)
	}

	return key, nil
}

// listChainMembers returns the incremental backups of owner's cluster whose base
// annotation equals key, unsorted. Passing onlySucceeded restricts the result to
// backups that finished successfully.
func listChainMembers(
	ctx context.Context,
	cl client.Client,
	owner *api.PerconaXtraDBClusterBackup,
	key string,
	onlySucceeded bool,
) ([]api.PerconaXtraDBClusterBackup, error) {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := cl.List(ctx, &bcpList,
		client.InNamespace(owner.GetNamespace()),
		client.MatchingFields{PXCClusterBackupField: owner.Spec.PXCCluster},
	); err != nil {
		return nil, errors.Wrapf(err, "list backups of cluster %s/%s", owner.GetNamespace(), owner.Spec.PXCCluster)
	}

	members := make([]api.PerconaXtraDBClusterBackup, 0, len(bcpList.Items))
	for _, bcp := range bcpList.Items {
		if bcp.GetType() != api.BackupTypeIncremental {
			continue
		}
		if onlySucceeded && bcp.Status.State != api.BackupSucceeded {
			continue
		}
		if bcp.GetAnnotations()[naming.AnnotationBaseBackupName] != key {
			continue
		}
		members = append(members, bcp)
	}

	return members, nil
}
