package k8s

import (
	"context"
	"time"

	"github.com/pkg/errors"
	coordv1 "k8s.io/api/coordination/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrNotTheHolder = errors.New("not the holder")
)

type IsHolderStaleFunc func(ctx context.Context, holder string) (bool, error)

func AcquireLease(
	ctx context.Context,
	c client.Client,
	name, namespace, holder string,
	isHolderStale IsHolderStaleFunc,
) (*coordv1.Lease, error) {
	lease := new(coordv1.Lease)

	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, lease); err != nil {
		if !k8serrors.IsNotFound(err) {
			return lease, errors.Wrap(err, "get lease")
		}

		return createLease(ctx, c, name, namespace, holder)
	}

	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == holder {
		return lease, nil
	}

	if isHolderStale == nil {
		return lease, nil
	}

	current := ""
	if lease.Spec.HolderIdentity != nil {
		current = *lease.Spec.HolderIdentity
	}

	stale, err := isHolderStale(ctx, current)
	if err != nil {
		return lease, errors.Wrap(err, "check whether the lease holder is stale")
	}
	if !stale {
		return lease, nil
	}

	// Delete with preconditions so this cannot remove a lease that somebody
	// else acquired between the Get above and here.
	err = c.Delete(ctx, lease, client.Preconditions{
		UID:             &lease.UID,
		ResourceVersion: &lease.ResourceVersion,
	})
	if err != nil {
		if k8serrors.IsNotFound(err) || k8serrors.IsConflict(err) {
			// Somebody else changed it first. Leave the lease alone and let the
			// caller try again rather than racing it.
			return lease, nil
		}
		return lease, errors.Wrap(err, "evict stale lease holder")
	}

	return createLease(ctx, c, name, namespace, holder)
}

func createLease(ctx context.Context, c client.Client, name, namespace, holder string) (*coordv1.Lease, error) {
	lease := &coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: coordv1.LeaseSpec{
			AcquireTime:    &metav1.MicroTime{Time: time.Now()},
			HolderIdentity: &holder,
		},
	}

	if err := c.Create(ctx, lease); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// Another acquirer won the race. Return what is actually stored so
			// the caller sees it is not the holder.
			existing := new(coordv1.Lease)
			if getErr := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, existing); getErr != nil {
				return lease, errors.Wrap(getErr, "get lease after conflict")
			}
			return existing, nil
		}
		return lease, errors.Wrap(err, "create lease")
	}

	return lease, nil
}

func ReleaseLease(ctx context.Context, c client.Client, name, namespace, holder string) error {
	lease := new(coordv1.Lease)

	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, lease); err != nil {
		return errors.Wrap(err, "get lease")
	}

	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != holder {
		// Either nobody owns it or somebody else does; in both cases this
		// caller must not delete it.
		return ErrNotTheHolder
	}

	// Preconditions stop this from deleting a lease that was released and then
	// re-acquired by another backup between the Get above and here - which
	// would let two backups run at once, the exact thing the lease prevents.
	err := c.Delete(ctx, lease, client.Preconditions{
		UID:             &lease.UID,
		ResourceVersion: &lease.ResourceVersion,
	})
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ErrNotTheHolder
		}
		return errors.Wrap(err, "delete lease")
	}

	return nil
}
