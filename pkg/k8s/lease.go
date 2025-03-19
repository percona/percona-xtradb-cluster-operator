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

func AcquireLease(ctx context.Context, c client.Client, name, namespace, holder string) (*coordv1.Lease, error) {
	lease := new(coordv1.Lease)

	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, lease); err != nil {
		if !k8serrors.IsNotFound(err) {
			return lease, errors.Wrap(err, "get lease")
		}

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
			return lease, errors.Wrap(err, "create lease")
		}

		return lease, nil
	}

	return lease, nil
}

func ReleaseLease(ctx context.Context, c client.Client, name, namespace, holder string) error {
	lease := new(coordv1.Lease)

	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, lease); err != nil {
		return errors.Wrap(err, "get lease")
	}

	if lease.Spec.HolderIdentity == nil {
		// TODO: What to do?
	}

	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != holder {
		return ErrNotTheHolder
	}

	if err := c.Delete(ctx, lease); err != nil {
		return errors.Wrap(err, "delete lease")
	}

	return nil
}
