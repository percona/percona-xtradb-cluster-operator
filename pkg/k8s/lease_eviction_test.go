package k8s_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordv1 "k8s.io/api/coordination/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" // nolint

	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
)

const (
	leaseName      = "pxc-cluster1-backup-lock"
	leaseNamespace = "pxc"
	leaseSelf      = "backup2-22222222-2222-2222-2222-222222222222"
	leaseOther     = "backup1-11111111-1111-1111-1111-111111111111"
)

func heldLease(holder string) *coordv1.Lease {
	return &coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: leaseNamespace,
			UID:       types.UID("lease-uid"),
		},
		Spec: coordv1.LeaseSpec{
			AcquireTime:    &metav1.MicroTime{Time: time.Now()},
			HolderIdentity: &holder,
		},
	}
}

func leaseHolder(t *testing.T, cl client.Client) string {
	t.Helper()

	lease := new(coordv1.Lease)
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Name: leaseName, Namespace: leaseNamespace}, lease))
	require.NotNil(t, lease.Spec.HolderIdentity)

	return *lease.Spec.HolderIdentity
}

func TestAcquireLeaseEvictsStaleHolder(t *testing.T) {
	tests := map[string]struct {
		existing   *coordv1.Lease
		isStale    k8s.IsHolderStaleFunc
		wantHolder string
	}{
		"no lease yet, so it is created for us": {
			existing:   nil,
			isStale:    func(context.Context, string) (bool, error) { return false, nil },
			wantHolder: leaseSelf,
		},
		"we already hold it": {
			existing:   heldLease(leaseSelf),
			isStale:    func(context.Context, string) (bool, error) { return false, nil },
			wantHolder: leaseSelf,
		},
		"a live holder keeps it": {
			existing:   heldLease(leaseOther),
			isStale:    func(context.Context, string) (bool, error) { return false, nil },
			wantHolder: leaseOther,
		},
		"a stale holder is evicted and we take over": {
			existing:   heldLease(leaseOther),
			isStale:    func(context.Context, string) (bool, error) { return true, nil },
			wantHolder: leaseSelf,
		},
		"without a staleness check nothing is ever evicted": {
			existing:   heldLease(leaseOther),
			isStale:    nil,
			wantHolder: leaseOther,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			builder := fake.NewClientBuilder()
			if tt.existing != nil {
				builder = builder.WithObjects(tt.existing)
			}
			cl := builder.Build()

			lease, err := k8s.AcquireLease(context.Background(), cl,
				leaseName, leaseNamespace, leaseSelf, tt.isStale)
			require.NoError(t, err)

			require.NotNil(t, lease.Spec.HolderIdentity)
			assert.Equal(t, tt.wantHolder, *lease.Spec.HolderIdentity, "holder reported to the caller")
			assert.Equal(t, tt.wantHolder, leaseHolder(t, cl), "holder actually stored")
		})
	}
}

func TestReleaseLease(t *testing.T) {
	tests := map[string]struct {
		existing    *coordv1.Lease
		releaser    string
		wantErr     error
		wantDeleted bool
	}{
		"the holder releases it": {
			existing:    heldLease(leaseSelf),
			releaser:    leaseSelf,
			wantDeleted: true,
		},
		"a non-holder cannot release it": {
			existing:    heldLease(leaseOther),
			releaser:    leaseSelf,
			wantErr:     k8s.ErrNotTheHolder,
			wantDeleted: false,
		},
		"a lease with no holder is nobody's to release": {
			existing: &coordv1.Lease{
				ObjectMeta: metav1.ObjectMeta{Name: leaseName, Namespace: leaseNamespace},
			},
			releaser:    leaseSelf,
			wantErr:     k8s.ErrNotTheHolder,
			wantDeleted: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithObjects(tt.existing).Build()

			err := k8s.ReleaseLease(context.Background(), cl, leaseName, leaseNamespace, tt.releaser)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			getErr := cl.Get(context.Background(),
				types.NamespacedName{Name: leaseName, Namespace: leaseNamespace}, new(coordv1.Lease))
			assert.Equal(t, tt.wantDeleted, k8serrors.IsNotFound(getErr), "lease deleted")
		})
	}
}
