package k8s_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	coordv1 "k8s.io/api/coordination/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake" // nolint
)

var _ = Describe("Lease", func() {
	It("should be create a lease", func() {
		cl := fake.NewFakeClient()

		ctx := context.Background()

		name := "backup-lock"
		namespace := "test"
		holder := "backup1"

		lease, err := k8s.AcquireLease(ctx, cl, name, namespace, holder)
		Expect(err).ToNot(HaveOccurred())

		freshLease := new(coordv1.Lease)
		nn := types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}
		err = cl.Get(ctx, nn, freshLease)
		Expect(err).ToNot(HaveOccurred())
		Expect(freshLease.Spec.AcquireTime).NotTo(BeNil())
		Expect(freshLease.Spec.HolderIdentity, lease.Spec.HolderIdentity)
	})

	It("should be delete a lease", func() {
		ctx := context.Background()

		name := "backup-lock"
		namespace := "test"
		holder := "backup1"

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

		cl := fake.NewFakeClient(lease)

		err := k8s.ReleaseLease(ctx, cl, name, namespace)
		Expect(err).ToNot(HaveOccurred())

		freshLease := new(coordv1.Lease)
		nn := types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}
		err = cl.Get(ctx, nn, freshLease)
		Expect(err).To(HaveOccurred())
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	})
})
