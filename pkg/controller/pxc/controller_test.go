package pxc

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cm "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("PerconaXtraDB Cluster", Ordered, func() {
	ctx := context.Background()
	const ns = "pxc"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	crName := ns + "-reconciler"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

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
		cr, err := readDefaultCR(crName, ns)
		It("should read defautl cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should create PerconaXtraDBCluster", func() {
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})
	})

	It("Should reconcile PerconaXtraDBCluster", func() {
		_, err := reconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: crNamespacedName,
		})
		Expect(err).To(Succeed())
	})
})

var _ = Describe("Finalizer delete-ssl", Ordered, func() {
	ctx := context.Background()

	const crName = "del-ssl-fnlz"
	const ns = "del-ssl-fnlz"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}

	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))

		_, err = envtest.InstallCRDs(cfg, envtest.CRDInstallOptions{
			Paths: []string{filepath.Join("testdata", "cert-manager.yaml")},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	Context("delete-ssl finalizer specified", Ordered, func() {

		cr, err := readDefaultCR(crName, ns)

		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		cr.Finalizers = append(cr.Finalizers, "delete-ssl")
		cr.Spec.SSLSecretName = "cluster1-ssl"
		cr.Spec.SSLInternalSecretName = "cluster1-ssl-internal"

		It("Should create PerconaXtraDBCluster", func() {
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile once to create user secret", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("controller should create ssl-secrets", func() {
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Spec.SSLSecretName,
			}, secret)).Should(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Spec.SSLInternalSecretName,
			}, secret)).Should(Succeed())
		})

		It("controller should create issuers and certificates", func() {
			issuers := &cm.IssuerList{}
			Eventually(func() bool {

				opts := &client.ListOptions{Namespace: cr.Namespace}
				err := k8sClient.List(ctx, issuers, opts)

				return err == nil
			}, time.Second*30, time.Millisecond*250).Should(BeTrue())

			Expect(issuers.Items).ShouldNot(BeEmpty())

			certs := &cm.CertificateList{}
			Eventually(func() bool {

				opts := &client.ListOptions{Namespace: cr.Namespace}
				err := k8sClient.List(ctx, certs, opts)

				return err == nil
			}, time.Second*30, time.Millisecond*250).Should(BeTrue())

			Expect(certs.Items).ShouldNot(BeEmpty())
		})

		When("PXC cluster is deleted with delete-ssl finalizer certs should be removed", func() {
			It("should delete PXC cluster and reconcile changes", func() {
				Expect(k8sClient.Delete(ctx, cr)).Should(Succeed())

				_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
				Expect(err).NotTo(HaveOccurred())
			})

			It("controller should remove ssl-secrets", func() {
				secret := &corev1.Secret{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      cr.Spec.SSLSecretName,
					}, secret)

					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      cr.Spec.SSLInternalSecretName,
					}, secret)

					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      cr.Name + "-ca-cert",
					}, secret)

					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())
			})

			It("controller should delete issuers and certificates", func() {
				issuers := &cm.IssuerList{}
				Eventually(func() bool {

					opts := &client.ListOptions{Namespace: cr.Namespace}
					err := k8sClient.List(ctx, issuers, opts)

					return err == nil
				}, time.Second*30, time.Millisecond*250).Should(BeTrue())

				Expect(issuers.Items).Should(BeEmpty())

				certs := &cm.CertificateList{}
				Eventually(func() bool {

					opts := &client.ListOptions{Namespace: cr.Namespace}
					err := k8sClient.List(ctx, certs, opts)

					return err == nil
				}, time.Second*30, time.Millisecond*250).Should(BeTrue())

				Expect(certs.Items).Should(BeEmpty())
			})
		})
	})
})

var _ = Describe("Finalizer delete-proxysql-pvc", Ordered, func() {
	ctx := context.Background()

	const crName = "del-proxysql-pvc-fnlz"
	const ns = "del-proxysql-pvc-fnlz"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}

	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))

		_, err = envtest.InstallCRDs(cfg, envtest.CRDInstallOptions{
			Paths: []string{filepath.Join("testdata", "cert-manager.yaml")},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	Context("delete-proxysql-pvc finalizer specified", Ordered, func() {

		cr, err := readDefaultCR(crName, ns)

		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		cr.Finalizers = append(cr.Finalizers, "delete-proxysql-pvc")
		cr.Spec.SecretsName = "cluster1-secrets"
		cr.Spec.HAProxy.Enabled = false
		cr.Spec.ProxySQL.Enabled = true

		sfsWithOwner := appsv1.StatefulSet{}
		sfsProxy := statefulset.NewProxy(cr)

		It("Should create PerconaXtraDBCluster", func() {
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile once to create user secret and pvc", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should create proxysql sts", func() {

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      cr.Name + "-proxysql",
				Namespace: cr.Namespace,
			}, &sfsWithOwner)).Should(Succeed())
		})

		It("Should create secrets", func() {
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Spec.SecretsName,
			}, secret)).Should(Succeed())
		})

		It("should create proxysql PVC", func() {
			for _, claim := range sfsWithOwner.Spec.VolumeClaimTemplates {
				for i := 0; i < int(*sfsWithOwner.Spec.Replicas); i++ {
					pvc := claim.DeepCopy()
					pvc.Labels = sfsProxy.Labels()
					pvc.Name = strings.Join([]string{pvc.Name, sfsWithOwner.Name, strconv.Itoa(i)}, "-")
					pvc.Namespace = ns
					Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
				}
			}
		})

		It("controller should have proxysql pvc", func() {
			pvcList := corev1.PersistentVolumeClaimList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx,
					&pvcList,
					&client.ListOptions{
						Namespace: cr.Namespace,
						LabelSelector: labels.SelectorFromSet(map[string]string{
							"app.kubernetes.io/component": "proxysql",
						}),
					})
				return err == nil
			}, time.Second*15, time.Millisecond*250).Should(BeTrue())
			Expect(len(pvcList.Items)).Should(Equal(3))
		})

		When("PXC cluster is deleted with delete-proxysql-pvc finalizer sts and pvc should be removed and secrets kept", func() {
			It("should delete PXC cluster and reconcile changes", func() {
				Expect(k8sClient.Delete(ctx, cr)).Should(Succeed())

				_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
				Expect(err).NotTo(HaveOccurred())
			})

			It("controller should remove sts", func() {
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      cr.Name + "-proxysql",
						Namespace: cr.Namespace,
					}, &sfsWithOwner)
					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

			})

			It("controller should remove pvc for proxysql", func() {
				pvcList := corev1.PersistentVolumeClaimList{}
				Eventually(func() bool {
					err := k8sClient.List(ctx, &pvcList, &client.ListOptions{
						Namespace: cr.Namespace,
						LabelSelector: labels.SelectorFromSet(map[string]string{
							"app.kubernetes.io/component": "proxysql",
						}),
					})
					return err == nil
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

				for _, pvc := range pvcList.Items {
					By(fmt.Sprintf("checking pvc/%s", pvc.Name))
					Expect(pvc.DeletionTimestamp).ShouldNot(BeNil())
				}
			})

			It("controller should keep secrets", func() {
				secret := &corev1.Secret{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      cr.Spec.SecretsName,
					}, secret)

					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeFalse())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      "internal-" + cr.Name,
					}, secret)

					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeFalse())

			})
		})
	})
})

var _ = Describe("Finalizer delete-pxc-pvc", Ordered, func() {
	ctx := context.Background()

	const crName = "del-pxc-pvc-fnlz"
	const ns = "del-pxc-pvc-fnlz"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}

	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))

		_, err = envtest.InstallCRDs(cfg, envtest.CRDInstallOptions{
			Paths: []string{filepath.Join("testdata", "cert-manager.yaml")},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	Context("delete-pxc-pvc finalizer specified", Ordered, func() {

		cr, err := readDefaultCR(crName, ns)

		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})
		cr.Finalizers = append(cr.Finalizers, "delete-pxc-pvc")
		cr.Spec.SecretsName = "cluster1-secrets"

		sfsWithOwner := appsv1.StatefulSet{}
		stsApp := statefulset.NewNode(cr)

		It("Should create PerconaXtraDBCluster", func() {
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile once to create user secret", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should create pxc sts", func() {

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      cr.Name + "-pxc",
				Namespace: cr.Namespace,
			}, &sfsWithOwner)).Should(Succeed())
		})

		It("Should create secrets", func() {
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cr.Namespace,
				Name:      cr.Spec.SecretsName,
			}, secret)).Should(Succeed())
		})

		It("should create pxc PVC", func() {
			for _, claim := range sfsWithOwner.Spec.VolumeClaimTemplates {
				for i := 0; i < int(*sfsWithOwner.Spec.Replicas); i++ {
					pvc := claim.DeepCopy()
					pvc.Labels = stsApp.Labels()
					pvc.Name = strings.Join([]string{pvc.Name, sfsWithOwner.Name, strconv.Itoa(i)}, "-")
					pvc.Namespace = ns
					Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
				}
			}
		})

		It("controller should have pxc pvc", func() {
			pvcList := corev1.PersistentVolumeClaimList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx,
					&pvcList,
					&client.ListOptions{
						Namespace: cr.Namespace,
						LabelSelector: labels.SelectorFromSet(map[string]string{
							"app.kubernetes.io/component": "pxc",
						}),
					})
				return err == nil
			}, time.Second*25, time.Millisecond*250).Should(BeTrue())
			Expect(len(pvcList.Items)).Should(Equal(3))
		})

		When("PXC cluster is deleted with delete-pxc-pvc finalizer sts, pvc, and secrets should be removed", func() {
			It("should delete PXC cluster and reconcile changes", func() {
				Expect(k8sClient.Delete(ctx, cr)).Should(Succeed())

				_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
				Expect(err).NotTo(HaveOccurred())
			})

			It("controller should remove sts", func() {
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      cr.Name + "-pxc",
						Namespace: cr.Namespace,
					}, &sfsWithOwner)
					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

			})

			It("controller should remove pvc for pxc", func() {
				pvcList := corev1.PersistentVolumeClaimList{}
				Eventually(func() bool {
					err := k8sClient.List(ctx, &pvcList, &client.ListOptions{
						Namespace: cr.Namespace,
						LabelSelector: labels.SelectorFromSet(map[string]string{
							"app.kubernetes.io/component": "pxc",
						}),
					})
					return err == nil
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

				for _, pvc := range pvcList.Items {
					By(fmt.Sprintf("checking pvc/%s", pvc.Name))
					Expect(pvc.DeletionTimestamp).ShouldNot(BeNil())
				}
			})

			It("controller should delete secrets", func() {
				secret := &corev1.Secret{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      cr.Spec.SecretsName,
					}, secret)

					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      "internal-" + cr.Name,
					}, secret)

					return k8serrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())

			})
		})
	})
})
