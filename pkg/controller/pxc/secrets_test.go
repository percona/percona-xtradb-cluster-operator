package pxc

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Secrets generation", Ordered, func() {
	ctx := context.Background()
	const ns = "sec-gen"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	crName := ns + "-cr"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}
	cr, err := readDefaultCR(crName, ns)
	cr.Spec.GeneratedSecretsOptions = &v1.GeneratedSecretsOptions{
		Symbols:   "",
		MinLength: 22,
		MaxLength: 30,
	}
	It("should read default cr.yaml", func() {
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))
	})

	AfterAll(func() {
		// TODO(user): Attention if you improve this code by adding other context test you MUST
		// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	Context("Create Percona XtraDB cluster", func() {
		It("Should create PerconaXtraDBCluster", func() {
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})
	})

	It("should reconcile PerconaXtraDBCluster", func() {
		_, err := reconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: crNamespacedName,
		})
		Expect(err).To(Succeed())
	})

	Context("Check secrets generation", func() {
		userSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-secrets",
				Namespace: cr.Namespace,
			},
		}
		It("Should generate user secrets", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecret), userSecret)).To(Succeed())
		})
		for password := range userSecret.Data {
			It("Should generate user secrets without symbols", func() {
				Expect(strings.ContainsAny(string(password), "!#$%&()*+,-.<=>?@[]^_{}~")).To(BeFalse())
			})
			It("Should generate user secrets with length constraints", func() {
				Expect(len(string(password))).To(BeNumerically(">=", cr.Spec.GeneratedSecretsOptions.MinLength))
				Expect(len(string(password))).To(BeNumerically("<=", cr.Spec.GeneratedSecretsOptions.MaxLength))
			})
		}
	})
})
