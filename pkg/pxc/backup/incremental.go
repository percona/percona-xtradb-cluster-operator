package backup

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

var (
	ErrNoFullBackupFound        = errors.New("no full backup found in storage")
	ErrNoIncrementalBackupFound = errors.New("no incremental backup found in chain")
	ErrBaseBackupNotFound       = errors.New("the base backup recorded for this chain no longer exists")
)

// sortBackupsByCompletion sorts succeeded backups in place, oldest first, by
// Status.CompletedAt. This should be called only on succeeded backups.
func sortBackupsByCompletion(backups []api.PerconaXtraDBClusterBackup) {
	sort.SliceStable(backups, func(i, j int) bool {
		return backups[i].Status.CompletedAt.Before(backups[j].Status.CompletedAt)
	})
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

		if !bcp.DeletionTimestamp.IsZero() {
			continue
		}
		match, err := bcp.IsUsingSameStorage(ctx, cl, storage)
		if err != nil {
			return nil, errors.Wrapf(err, "compare the storage of backup %s/%s", bcp.GetNamespace(), bcp.GetName())
		}
		if !match {
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

// GetLatestIncrementalBackupInChain returns the newest succeeded incremental
// backup in the chain that the given backup belongs to, or
// ErrNoIncrementalBackupFound when no member of the chain has succeeded.
func GetLatestIncrementalBackupInChain(
	ctx context.Context,
	cl client.Client,
	backup *api.PerconaXtraDBClusterBackup,
) (*api.PerconaXtraDBClusterBackup, error) {
	members, err := chainMembers(ctx, cl, backup)
	if err != nil {
		return nil, err
	}

	succeeded, err := succeededChainMembers(members)
	if err != nil {
		return nil, err
	}
	if len(succeeded) == 0 {
		return nil, ErrNoIncrementalBackupFound
	}

	sortBackupsByCompletion(succeeded)

	return succeeded[len(succeeded)-1].DeepCopy(), nil
}

// HasUnfinishedChainMember reports whether any member of the chain that the
// given backup belongs to has yet to reach a terminal state.
func HasUnfinishedChainMember(
	ctx context.Context,
	cl client.Client,
	backup *api.PerconaXtraDBClusterBackup,
) (bool, error) {
	members, err := chainMembers(ctx, cl, backup)
	if err != nil {
		return false, err
	}

	for i := range members {
		if !members[i].Status.State.IsTerminal() {
			return true, nil
		}
	}

	return false, nil
}

// NextDeletableDependent returns the incremental backup to remove next while
// tearing down the chain of the given full backup, or nil when nothing is
// chained to it any more.
func NextDeletableDependent(
	ctx context.Context,
	cl client.Client,
	fullBackup *api.PerconaXtraDBClusterBackup,
) (*api.PerconaXtraDBClusterBackup, error) {
	if fullBackup.GetType() != api.BackupTypeFull {
		return nil, nil
	}

	members, err := chainMembers(ctx, cl, fullBackup)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, nil
	}

	for i := range members {
		if members[i].Status.State != api.BackupSucceeded {
			return members[i].DeepCopy(), nil
		}
	}

	succeeded, err := succeededChainMembers(members)
	if err != nil {
		return nil, err
	}

	sortBackupsByCompletion(succeeded)

	return succeeded[len(succeeded)-1].DeepCopy(), nil
}

// succeededChainMembers returns the members that succeeded, in the order they
// were given.
func succeededChainMembers(members []api.PerconaXtraDBClusterBackup) ([]api.PerconaXtraDBClusterBackup, error) {
	succeeded := make([]api.PerconaXtraDBClusterBackup, 0, len(members))
	for _, m := range members {
		if m.Status.State != api.BackupSucceeded {
			continue
		}
		if m.Status.CompletedAt == nil {
			return nil, errors.Errorf("backup %s/%s is succeeded but completedAt is not set", m.GetNamespace(), m.GetName())
		}
		succeeded = append(succeeded, m)
	}

	return succeeded, nil
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

	if !base.DeletionTimestamp.IsZero() {
		return nil, errors.Errorf("base backup %s/%s is being deleted, so it cannot be the base of incremental backup %s/%s",
			base.GetNamespace(), base.GetName(), cr.GetNamespace(), cr.GetName())
	}

	inSameStorage, err := base.IsUsingSameStorage(ctx, cl, storage)
	if err != nil {
		return nil, errors.Wrapf(err, "compare the storage of base backup %s/%s", base.GetNamespace(), base.GetName())
	}
	if !inSameStorage {
		return nil, errors.Errorf("base backup %s/%s is in storage %q, which is not the storage of incremental backup %s/%s",
			base.GetNamespace(), base.GetName(), base.Status.StorageName, cr.GetNamespace(), cr.GetName())
	}

	if base.Spec.PXCCluster != cr.Spec.PXCCluster {
		return nil, errors.Errorf("base backup %s/%s belongs to cluster %q, which is not the cluster of incremental backup %s/%s",
			base.GetNamespace(), base.GetName(), base.Spec.PXCCluster, cr.GetNamespace(), cr.GetName())
	}

	return base, nil
}

// GetBackupByDestinationLeaf returns the cluster's backup that landed in the
// given storage under the given object-storage leaf name, or
// ErrBaseBackupNotFound when there is none.
func GetBackupByDestinationLeaf(
	ctx context.Context,
	cl client.Client,
	cluster *api.PerconaXtraDBCluster,
	storage *api.BackupStorageSpec,
	leaf string,
) (*api.PerconaXtraDBClusterBackup, error) {
	if leaf == "" {
		return nil, ErrBaseBackupNotFound
	}

	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := cl.List(ctx, &bcpList,
		client.InNamespace(cluster.GetNamespace()),
		client.MatchingFields{PXCClusterBackupField: cluster.GetName()},
	); err != nil {
		return nil, errors.Wrapf(err, "list backups of cluster %s/%s", cluster.GetNamespace(), cluster.GetName())
	}

	var found *api.PerconaXtraDBClusterBackup
	for i := range bcpList.Items {
		bcp := &bcpList.Items[i]
		if bcp.Status.Destination.BackupName() != leaf {
			continue
		}
		match, err := bcp.IsUsingSameStorage(ctx, cl, storage)
		if err != nil {
			return nil, errors.Wrapf(err, "compare the storage of backup %s/%s", bcp.GetNamespace(), bcp.GetName())
		}
		if !match {
			continue
		}
		if found != nil {
			// Two backups of one cluster sharing a leaf within one storage share
			// an object-storage destination too, so one has overwritten the
			// other. Which of them is being asked for cannot be answered, and
			// answering anyway picks a base at random.
			return nil, errors.Errorf("backups %s/%s and %s/%s of cluster %s both claim the destination %q",
				found.GetNamespace(), found.GetName(), bcp.GetNamespace(), bcp.GetName(), cluster.GetName(), leaf)
		}
		found = bcp.DeepCopy()
	}

	if found == nil {
		return nil, ErrBaseBackupNotFound
	}

	return found, nil
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

// chainMembers returns every incremental backup that belongs to the same chain
// as owner, whatever state it is in.
func chainMembers(
	ctx context.Context,
	cl client.Client,
	owner *api.PerconaXtraDBClusterBackup,
) ([]api.PerconaXtraDBClusterBackup, error) {
	key, err := chainKey(owner)
	if err != nil {
		return nil, err
	}
	if key == "" {
		// A backup that never recorded a destination owns no chain yet.
		return nil, nil
	}

	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := cl.List(ctx, &bcpList,
		client.InNamespace(owner.GetNamespace()),
		client.MatchingFields{PXCClusterBackupField: owner.Spec.PXCCluster},
	); err != nil {
		return nil, errors.Wrapf(err, "list backups of cluster %s/%s", owner.GetNamespace(), owner.Spec.PXCCluster)
	}

	ownerStorage := owner.Status.IntoBackupStorageSpec()

	members := make([]api.PerconaXtraDBClusterBackup, 0, len(bcpList.Items))
	for _, bcp := range bcpList.Items {
		if bcp.GetType() != api.BackupTypeIncremental {
			continue
		}
		if bcp.GetAnnotations()[naming.AnnotationBaseBackupName] != key {
			continue
		}

		match, err := bcp.IsUsingSameStorage(ctx, cl, ownerStorage)
		if err != nil {
			return nil, errors.Wrapf(err, "compare the storage of backup %s/%s", bcp.GetNamespace(), bcp.GetName())
		}
		if !match {
			continue
		}

		members = append(members, bcp)
	}

	sort.Slice(members, func(i, j int) bool { return members[i].GetName() < members[j].GetName() })

	return members, nil
}
