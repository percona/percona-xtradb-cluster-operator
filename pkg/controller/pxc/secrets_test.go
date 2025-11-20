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
	const defaultSymbols = "!#$%&()*+,-.<=>?@[]^_{}~"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	crName := ns + "-cr"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}
	cr, err := readDefaultCR(crName, ns)
	It("Should read default cr.yaml", func() {
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

	Context("Create cluster with default password generation behavior", func() {
		It("Should create PerconaXtraDBCluster", func() {
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("Should reconcile PerconaXtraDBCluster", func() {
			_, err := reconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: crNamespacedName,
			})
			Expect(err).To(Succeed())
		})
		userSecrets := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-secrets",
				Namespace: cr.Namespace,
			},
		}
		It("Should generate user secrets", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecrets), userSecrets)).To(Succeed())
		})
		for password := range userSecrets.Data {
			It("Should generate passwords with default symbols", func() {
				Expect(strings.ContainsAny(password, defaultSymbols)).To(BeTrue())
			})
			It("Should generate passwords with default length constraints", func() {
				Expect(len(password)).To(BeNumerically(">=", 16))
				Expect(len(password)).To(BeNumerically("<=", 20))
			})
		}
	})

	Context("Check user secrets generation with custom password generation options", func() {
		cr := &v1.PerconaXtraDBCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: ns,
			},
		}
		userSecrets := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-secrets",
				Namespace: cr.Namespace,
			},
		}
		It("Should retrieve cluster configuration", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).Should(Succeed())
		})
		It("Should retrieve current user secrets", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecrets), userSecrets)).To(Succeed())
		})
		It("Should update PerconaXtraDBCluster with custom options", func() {
			cr.Spec.PasswordGenerationOptions = &v1.PasswordGenerationOptions{
				Symbols:   "",
				MinLength: 22,
				MaxLength: 30,
			}
			Expect(k8sClient.Update(ctx, cr)).Should(Succeed())
		})
		It("Should reconcile PerconaXtraDBCluster", func() {
			_, err := reconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: crNamespacedName,
			})
			Expect(err).To(Succeed())
		})
		It("Should not change existing user secrets", func() {
			userSecretsBis := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name + "-secrets",
					Namespace: cr.Namespace,
				},
			}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecretsBis), userSecretsBis)).To(Succeed())
			Expect(userSecretsBis.Data).Should(Equal(userSecrets.Data))
		})
		It("Should remove existing user secret", func() {
			Expect(k8sClient.Delete(ctx, userSecrets)).Should(Succeed())
		})
		It("Should reconcile PerconaXtraDBCluster", func() {
			_, err := reconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: crNamespacedName,
			})
			Expect(err).To(Succeed())
		})
		It("Should generate user secrets", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecrets), userSecrets)).To(Succeed())
		})
		for password := range userSecrets.Data {
			It("Should generate passwords without symbols", func() {
				Expect(strings.ContainsAny(password, defaultSymbols)).To(BeFalse())
			})
			It("Should generate passwords with length constraints", func() {
				Expect(len(password)).To(BeNumerically(">=", cr.Spec.PasswordGenerationOptions.MinLength))
				Expect(len(password)).To(BeNumerically("<=", cr.Spec.PasswordGenerationOptions.MaxLength))
			})
		}
	})
})
