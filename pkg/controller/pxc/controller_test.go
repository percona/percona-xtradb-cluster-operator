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
	gs "github.com/onsi/gomega/gstruct"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
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

var _ = Describe("Authentication policy", Ordered, func() {
	ctx := context.Background()

	const ns = "auth-policy"
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
	})

	AfterAll(func() {
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	Context("Cluster is deployed with ProxySQL", Ordered, func() {
		const crName = "auth-policy-proxysql"
		crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

		cr, err := readDefaultCR(crName, ns)
		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create PerconaXtraDBCluster", func() {
			cr.Spec.HAProxy.Enabled = false
			cr.Spec.ProxySQL.Enabled = true

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should use mysql_native_password", func() {
			sts := appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-pxc",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&sts), &sts)
			Expect(err).NotTo(HaveOccurred())

			for _, c := range sts.Spec.Template.Spec.Containers {
				if c.Name == "pxc" {
					Expect(c.Env).Should(ContainElement(gs.MatchFields(gs.IgnoreExtras, gs.Fields{
						"Name":  Equal("DEFAULT_AUTHENTICATION_PLUGIN"),
						"Value": Equal("mysql_native_password"),
					})))
				}
			}
		})
	})

	Context("Cluster is deployed with HAProxy", Ordered, func() {
		const crName = "auth-policy-haproxy"
		crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

		cr, err := readDefaultCR(crName, ns)
		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create PerconaXtraDBCluster", func() {
			cr.Spec.HAProxy.Enabled = true
			cr.Spec.ProxySQL.Enabled = false

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should use caching_sha2_password", func() {
			sts := appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-pxc",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&sts), &sts)
			Expect(err).NotTo(HaveOccurred())

			envFound := false
			for _, c := range sts.Spec.Template.Spec.Containers {
				if c.Name == "pxc" {
					for _, e := range c.Env {
						if e.Name == "DEFAULT_AUTHENTICATION_PLUGIN" {
							envFound = true
							Expect(e.Value).To(Equal("caching_sha2_password"))
						}
					}
				}
			}

			Expect(envFound).To(BeTrue())
		})

		When("Proxy is switched from HAProxy to ProxySQL", func() {
			It("should update PerconaXtraDBCluster", func() {
				cr := &api.PerconaXtraDBCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      crName,
						Namespace: ns,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).Should(Succeed())

				cr.Spec.HAProxy.Enabled = false
				cr.Spec.ProxySQL.Enabled = true

				Expect(k8sClient.Update(ctx, cr)).Should(Succeed())
			})

			It("should NOT reconcile", func() {
				_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
				Expect(err).To(MatchError("failed to enable ProxySQL: for mysql version 8.0 you can't switch from HAProxy to ProxySQL"))
			})
		})
	})
})

var _ = Describe("Ignore labels and annotations", Ordered, func() {
	ctx := context.Background()

	const ns = "ignore-lbl-ants"
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
	})

	AfterAll(func() {
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	Context("HAProxy", Ordered, func() {
		const crName = "ignore-lbl-ants-h"
		crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

		cr, err := readDefaultCR(crName, ns)
		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create PerconaXtraDBCluster", func() {
			cr.Spec.HAProxy.Enabled = true
			cr.Spec.ProxySQL.Enabled = false

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("patches services with labels and annotations", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			orig := svc.DeepCopy()

			svc.ObjectMeta.Annotations["notIgnoredAnnotation"] = "true"
			svc.ObjectMeta.Annotations["ignoredAnnotation"] = "true"

			svc.ObjectMeta.Labels["notIgnoredLabel"] = "true"
			svc.ObjectMeta.Labels["ignoredLabel"] = "true"

			err = k8sClient.Patch(ctx, &svc, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("check all labels and annotations exist in the service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("notIgnoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("notIgnoredLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
		})

		It("should add ignored labels and annotations", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.IgnoreAnnotations = append(cr.Spec.IgnoreAnnotations, "ignoredAnnotation")
			cr.Spec.IgnoreLabels = append(cr.Spec.IgnoreLabels, "ignoredLabel")

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("check all labels and annotations exist in the service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("notIgnoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("notIgnoredLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
		})

		It("patches CR with service labels and annotations", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.HAProxy.ExposePrimary.Annotations = make(map[string]string)
			cr.Spec.HAProxy.ExposePrimary.Labels = make(map[string]string)

			cr.Spec.HAProxy.ExposePrimary.Annotations["crAnnotation"] = "true"
			cr.Spec.HAProxy.ExposePrimary.Labels["crLabel"] = "true"

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete all not ignored labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("crAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("notIgnoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("crLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("notIgnoredLabel"))
		})

		It("deletes service labels and annotations from CR", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			delete(cr.Spec.HAProxy.ExposePrimary.Annotations, "crAnnotation")
			delete(cr.Spec.HAProxy.ExposePrimary.Labels, "crLabel")

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not delete any labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("crAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("crLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
		})

		It("patches CR with more service labels and annotations", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.HAProxy.ExposePrimary.Annotations = make(map[string]string)
			cr.Spec.HAProxy.ExposePrimary.Labels = make(map[string]string)

			cr.Spec.HAProxy.ExposePrimary.Annotations["secondCrAnnotation"] = "true"
			cr.Spec.HAProxy.ExposePrimary.Annotations["thirdCrAnnotation"] = "true"

			cr.Spec.HAProxy.ExposePrimary.Labels["secondCrLabel"] = "true"
			cr.Spec.HAProxy.ExposePrimary.Labels["thirdCrLabel"] = "true"

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete previous labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("secondCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("thirdCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("crAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("secondCrLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("thirdCrLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("crLabel"))
		})

		It("deletes a label and an annotation from CR", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			delete(cr.Spec.HAProxy.ExposePrimary.Annotations, "secondCrAnnotation")
			delete(cr.Spec.HAProxy.ExposePrimary.Labels, "secondCrLabel")

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete removed service label and annotation from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("thirdCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("secondCrAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("thirdCrLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("secondCrLabel"))
		})

		It("deletes ignored labels and annotations from CR", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.IgnoreAnnotations = []string{}
			cr.Spec.IgnoreLabels = []string{}

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete unknown labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-haproxy",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("thirdCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("thirdCrLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("ignoredLabel"))
		})
	})

	Context("ProxySQL", Ordered, func() {
		const crName = "ignore-lbl-ants-p"
		crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

		cr, err := readDefaultCR(crName, ns)
		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create PerconaXtraDBCluster", func() {
			cr.Spec.HAProxy.Enabled = false
			cr.Spec.ProxySQL.Enabled = true

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("patches services with labels and annotations", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			orig := svc.DeepCopy()

			svc.ObjectMeta.Annotations["notIgnoredAnnotation"] = "true"
			svc.ObjectMeta.Annotations["ignoredAnnotation"] = "true"

			svc.ObjectMeta.Labels["notIgnoredLabel"] = "true"
			svc.ObjectMeta.Labels["ignoredLabel"] = "true"

			err = k8sClient.Patch(ctx, &svc, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("check all labels and annotations exist in the service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("notIgnoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("notIgnoredLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
		})

		It("should add ignored labels and annotations", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.IgnoreAnnotations = append(cr.Spec.IgnoreAnnotations, "ignoredAnnotation")
			cr.Spec.IgnoreLabels = append(cr.Spec.IgnoreLabels, "ignoredLabel")

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("check all labels and annotations exist in the service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("notIgnoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("notIgnoredLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
		})

		It("patches CR with service labels and annotations", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.ProxySQL.Expose.Annotations = make(map[string]string)
			cr.Spec.ProxySQL.Expose.Labels = make(map[string]string)

			cr.Spec.ProxySQL.Expose.Annotations["crAnnotation"] = "true"
			cr.Spec.ProxySQL.Expose.Labels["crLabel"] = "true"

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete all not ignored labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("crAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("notIgnoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("crLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("notIgnoredLabel"))
		})

		It("deletes service labels and annotations from CR", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			delete(cr.Spec.ProxySQL.Expose.Annotations, "crAnnotation")
			delete(cr.Spec.ProxySQL.Expose.Labels, "crLabel")

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not delete any labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("crAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("crLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
		})

		It("patches CR with more service labels and annotations", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.ProxySQL.Expose.Annotations = make(map[string]string)
			cr.Spec.ProxySQL.Expose.Labels = make(map[string]string)

			cr.Spec.ProxySQL.Expose.Annotations["secondCrAnnotation"] = "true"
			cr.Spec.ProxySQL.Expose.Annotations["thirdCrAnnotation"] = "true"

			cr.Spec.ProxySQL.Expose.Labels["secondCrLabel"] = "true"
			cr.Spec.ProxySQL.Expose.Labels["thirdCrLabel"] = "true"

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete previous labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("secondCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("thirdCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("crAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("secondCrLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("thirdCrLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("crLabel"))
		})

		It("deletes a label and an annotation from CR", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			delete(cr.Spec.ProxySQL.Expose.Annotations, "secondCrAnnotation")
			delete(cr.Spec.ProxySQL.Expose.Labels, "secondCrLabel")

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete removed service label and annotation from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("thirdCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).To(HaveKey("ignoredAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("secondCrAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("thirdCrLabel"))
			Expect(svc.ObjectMeta.Labels).To(HaveKey("ignoredLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("secondCrLabel"))
		})

		It("deletes ignored labels and annotations from CR", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)
			Expect(err).NotTo(HaveOccurred())

			orig := cr.DeepCopy()

			cr.Spec.IgnoreAnnotations = []string{}
			cr.Spec.IgnoreLabels = []string{}

			err = k8sClient.Patch(ctx, cr, client.MergeFrom(orig))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete unknown labels and annotations from service", func() {
			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-proxysql",
					Namespace: ns,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&svc), &svc)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.ObjectMeta.Annotations).To(HaveKey("thirdCrAnnotation"))
			Expect(svc.ObjectMeta.Annotations).ToNot(HaveKey("ignoredAnnotation"))

			Expect(svc.ObjectMeta.Labels).To(HaveKey("thirdCrLabel"))
			Expect(svc.ObjectMeta.Labels).ToNot(HaveKey("ignoredLabel"))
		})
	})
})

var _ = Describe("PostStart/PreStop lifecycle hooks", Ordered, func() {
	ctx := context.Background()

	const ns = "lifecycle"
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
	})

	AfterAll(func() {
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	checkLifecycleHooks := func(crName, component string) {
		sts := appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName + "-" + component,
				Namespace: ns,
			},
		}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&sts), &sts)
		Expect(err).NotTo(HaveOccurred())

		for _, c := range sts.Spec.Template.Spec.Containers {
			if c.Name == component {
				Expect(c.Lifecycle.PostStart).ShouldNot(BeNil())
				Expect(c.Lifecycle.PostStart.Exec).ShouldNot(BeNil())
				Expect(c.Lifecycle.PostStart.Exec.Command).Should(Equal([]string{"echo", "poststart"}))

				Expect(c.Lifecycle.PreStop).ShouldNot(BeNil())
				Expect(c.Lifecycle.PreStop.Exec).ShouldNot(BeNil())
				Expect(c.Lifecycle.PreStop.Exec.Command).Should(Equal([]string{"echo", "prestop"}))
			}
		}
	}

	Context("Cluster is deployed with ProxySQL", Ordered, func() {
		const crName = "proxysql-lifecycle"
		crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

		cr, err := readDefaultCR(crName, ns)
		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create PerconaXtraDBCluster with PXC and ProxySQL container lifecycle hooks", func() {
			cr.Spec.HAProxy.Enabled = false
			cr.Spec.ProxySQL.Enabled = true

			cr.Spec.PXC.Lifecycle = corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"echo", "poststart"},
					},
				},
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"echo", "prestop"},
					},
				},
			}

			cr.Spec.ProxySQL.Lifecycle = corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"echo", "poststart"},
					},
				},
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"echo", "prestop"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("pxc container should have poststart and prestop hooks set", func() {
			checkLifecycleHooks(crName, "pxc")
		})

		It("proxysql container should have poststart and prestop hooks set", func() {
			checkLifecycleHooks(crName, "proxysql")
		})
	})

	Context("Cluster is deployed with HAProxy", Ordered, func() {
		const crName = "haproxy-lifecycle"
		crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

		cr, err := readDefaultCR(crName, ns)
		It("should read default cr.yaml", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create PerconaXtraDBCluster with HAProxy container lifecycle hooks", func() {
			cr.Spec.HAProxy.Enabled = true
			cr.Spec.ProxySQL.Enabled = false

			cr.Spec.HAProxy.Lifecycle = corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"echo", "poststart"},
					},
				},
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"echo", "prestop"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("should reconcile", func() {
			_, err := reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("haproxy container should have poststart and prestop hooks set", func() {
			checkLifecycleHooks(crName, "haproxy")
		})
	})
})

var _ = Describe("Liveness/Readiness Probes", Ordered, func() {
	ctx := context.Background()

	const ns = "probes"
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
	})

	AfterAll(func() {
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	defaultReadiness := corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/var/lib/mysql/readiness-check.sh",
				},
			},
		},
		InitialDelaySeconds: int32(15),
		TimeoutSeconds:      int32(15),
		PeriodSeconds:       int32(30),
		SuccessThreshold:    int32(1),
		FailureThreshold:    int32(5),
	}
	defaultLiveness := corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/var/lib/mysql/liveness-check.sh",
				},
			},
		},
		InitialDelaySeconds: int32(300),
		TimeoutSeconds:      int32(5),
		PeriodSeconds:       int32(10),
		SuccessThreshold:    int32(1),
		FailureThreshold:    int32(3),
	}

	DescribeTable("PXC probes",
		func(probes func() (corev1.Probe, corev1.Probe)) {
			const crName = "probes"
			crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

			cr, err := readDefaultCR(crName, ns)
			Expect(err).NotTo(HaveOccurred())

			cr.ObjectMeta.Finalizers = []string{}

			readiness, liveness := probes()
			cr.Spec.PXC.ReadinessProbes = readiness
			cr.Spec.PXC.LivenessProbes = liveness

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())

			_, err = reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sts := appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "probes-pxc", Namespace: ns}, &sts)
			Expect(err).NotTo(HaveOccurred())

			for _, ct := range sts.Spec.Template.Spec.Containers {
				if ct.Name != "pxc" {
					continue
				}

				Expect(*ct.ReadinessProbe).To(Equal(readiness))
				Expect(*ct.LivenessProbe).To(Equal(liveness))
			}

			Expect(k8sClient.Delete(ctx, cr)).Should(Succeed())
		},
		Entry("[readiness] custom initial delay seconds", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultReadiness.DeepCopy()
			readiness.InitialDelaySeconds = defaultReadiness.InitialDelaySeconds + 10

			return *readiness, defaultLiveness
		}),
		Entry("[readiness] custom timeout seconds", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultReadiness.DeepCopy()
			readiness.TimeoutSeconds = defaultReadiness.TimeoutSeconds + 10

			return *readiness, defaultLiveness
		}),
		Entry("[readiness] custom period seconds", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultReadiness.DeepCopy()
			readiness.PeriodSeconds = defaultReadiness.PeriodSeconds + 10

			return *readiness, defaultLiveness
		}),
		Entry("[readiness] custom success threshold", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultReadiness.DeepCopy()
			readiness.SuccessThreshold = defaultReadiness.SuccessThreshold + 1

			return *readiness, defaultLiveness
		}),
		Entry("[readiness] custom failure threshold", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultReadiness.DeepCopy()
			readiness.FailureThreshold = defaultReadiness.FailureThreshold + 1

			return *readiness, defaultLiveness
		}),
		Entry("[liveness] custom initial delay seconds", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultLiveness.DeepCopy()
			liveness.InitialDelaySeconds = defaultLiveness.InitialDelaySeconds + 10

			return defaultReadiness, *liveness
		}),
		Entry("[liveness] custom timeout seconds", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultLiveness.DeepCopy()
			liveness.TimeoutSeconds = defaultLiveness.TimeoutSeconds + 10

			return defaultReadiness, *liveness
		}),
		Entry("[liveness] custom period seconds", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultLiveness.DeepCopy()
			liveness.PeriodSeconds = defaultLiveness.PeriodSeconds + 10

			return defaultReadiness, *liveness
		}),
		Entry("[liveness] custom success threshold", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultLiveness.DeepCopy()
			liveness.SuccessThreshold = defaultLiveness.SuccessThreshold + 1

			return defaultReadiness, *liveness
		}),
		Entry("[liveness] custom failure threshold", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultLiveness.DeepCopy()
			liveness.FailureThreshold = defaultLiveness.FailureThreshold + 1

			return defaultReadiness, *liveness
		}),
	)

	defaultHAProxyReadiness := corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/usr/local/bin/readiness-check.sh",
				},
			},
		},
		InitialDelaySeconds: int32(15),
		TimeoutSeconds:      int32(1),
		PeriodSeconds:       int32(5),
		SuccessThreshold:    int32(1),
		FailureThreshold:    int32(3),
	}
	defaultHAProxyLiveness := corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/usr/local/bin/liveness-check.sh",
				},
			},
		},
		InitialDelaySeconds: int32(60),
		TimeoutSeconds:      int32(5),
		PeriodSeconds:       int32(30),
		SuccessThreshold:    int32(1),
		FailureThreshold:    int32(4),
	}

	DescribeTable("HAProxy probes",
		func(probes func() (corev1.Probe, corev1.Probe)) {
			const crName = "probes"
			crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}

			cr, err := readDefaultCR(crName, ns)
			Expect(err).NotTo(HaveOccurred())

			cr.ObjectMeta.Finalizers = []string{}
			cr.Spec.HAProxy.Enabled = true
			cr.Spec.ProxySQL.Enabled = false

			readiness, liveness := probes()
			cr.Spec.HAProxy.ReadinessProbes = readiness
			cr.Spec.HAProxy.LivenessProbes = liveness

			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())

			_, err = reconciler().Reconcile(ctx, ctrl.Request{NamespacedName: crNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sts := appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "probes-haproxy", Namespace: ns}, &sts)
			Expect(err).NotTo(HaveOccurred())

			for _, ct := range sts.Spec.Template.Spec.Containers {
				if ct.Name != "haproxy" {
					continue
				}

				Expect(*ct.ReadinessProbe).To(Equal(readiness))
				Expect(*ct.LivenessProbe).To(Equal(liveness))
			}

			Expect(k8sClient.Delete(ctx, cr)).Should(Succeed())
		},
		Entry("[readiness] custom initial delay seconds", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultHAProxyReadiness.DeepCopy()
			readiness.InitialDelaySeconds = defaultHAProxyReadiness.InitialDelaySeconds + 10

			return *readiness, defaultHAProxyLiveness
		}),
		Entry("[readiness] custom timeout seconds", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultHAProxyReadiness.DeepCopy()
			readiness.TimeoutSeconds = defaultHAProxyReadiness.TimeoutSeconds + 10

			return *readiness, defaultHAProxyLiveness
		}),
		Entry("[readiness] custom period seconds", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultHAProxyReadiness.DeepCopy()
			readiness.PeriodSeconds = defaultHAProxyReadiness.PeriodSeconds + 10

			return *readiness, defaultHAProxyLiveness
		}),
		Entry("[readiness] custom success threshold", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultHAProxyReadiness.DeepCopy()
			readiness.SuccessThreshold = defaultHAProxyReadiness.SuccessThreshold + 1

			return *readiness, defaultHAProxyLiveness
		}),
		Entry("[readiness] custom failure threshold", func() (corev1.Probe, corev1.Probe) {
			readiness := defaultHAProxyReadiness.DeepCopy()
			readiness.FailureThreshold = defaultHAProxyReadiness.FailureThreshold + 1

			return *readiness, defaultHAProxyLiveness
		}),
		Entry("[liveness] custom initial delay seconds", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultHAProxyLiveness.DeepCopy()
			liveness.InitialDelaySeconds = defaultHAProxyLiveness.InitialDelaySeconds + 10

			return defaultHAProxyReadiness, *liveness
		}),
		Entry("[liveness] custom timeout seconds", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultHAProxyLiveness.DeepCopy()
			liveness.TimeoutSeconds = defaultHAProxyLiveness.TimeoutSeconds + 10

			return defaultHAProxyReadiness, *liveness
		}),
		Entry("[liveness] custom period seconds", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultHAProxyLiveness.DeepCopy()
			liveness.PeriodSeconds = defaultHAProxyLiveness.PeriodSeconds + 10

			return defaultHAProxyReadiness, *liveness
		}),
		Entry("[liveness] custom success threshold", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultHAProxyLiveness.DeepCopy()
			liveness.SuccessThreshold = defaultHAProxyLiveness.SuccessThreshold + 1

			return defaultHAProxyReadiness, *liveness
		}),
		Entry("[liveness] custom failure threshold", func() (corev1.Probe, corev1.Probe) {
			liveness := defaultHAProxyLiveness.DeepCopy()
			liveness.FailureThreshold = defaultHAProxyLiveness.FailureThreshold + 1

			return defaultHAProxyReadiness, *liveness
		}),
	)
})
