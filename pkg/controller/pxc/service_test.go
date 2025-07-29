package pxc

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
)

var _ = Describe("Service labels and annotations", Ordered, func() {
	ctx := context.Background()
	const ns = "svc-ls-an"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	crName := ns + "-cr"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}
	cr, err := readDefaultCR(crName, ns)
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

	checkLabelsAndAnnotations := func(services []*corev1.Service) {
		Context("update service labels manually", func() {
			It("should update service labels manually", func() {
				for i := range services {
					svc := new(corev1.Service)

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(services[i]), svc)).To(Succeed())

					svc.Labels["manual-label"] = "test"
					svc.Labels["ignored-label"] = "test"
					svc.Annotations["manual-annotation"] = "test"
					svc.Annotations["ignored-annotation"] = "test"
					Expect(k8sClient.Update(ctx, svc)).To(Succeed())
				}
			})

			It("should reconcile PerconaXtraDBCluster", func() {
				_, err := reconciler().Reconcile(ctx, reconcile.Request{
					NamespacedName: crNamespacedName,
				})
				Expect(err).To(Succeed())
			})
			It("should check if manual labels and annotations are still there", func() {
				for i := range services {
					svc := new(corev1.Service)

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(services[i]), svc)).To(Succeed())

					Expect(svc.Labels["manual-label"]).To(Equal("test"))
					Expect(svc.Annotations["manual-annotation"]).To(Equal("test"))
					Expect(svc.Labels["ignored-label"]).To(Equal("test"))
					Expect(svc.Annotations["ignored-annotation"]).To(Equal("test"))
				}
			})
		})

		Context("set service labels and annotations", func() {
			It("should update cr", func() {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
				cr.Spec.IgnoreAnnotations = []string{"ignored-annotation"}
				cr.Spec.IgnoreLabels = []string{"ignored-label"}
				cr.Spec.PXC.Expose.Labels = map[string]string{"cr-label": "test"}
				cr.Spec.PXC.Expose.Annotations = map[string]string{"cr-annotation": "test"}
				cr.Spec.HAProxy.ExposePrimary.Labels = map[string]string{"cr-label": "test"}
				cr.Spec.HAProxy.ExposePrimary.Annotations = map[string]string{"cr-annotation": "test"}

				if cr.Spec.HAProxy.ExposeReplicas == nil {
					cr.Spec.HAProxy.ExposeReplicas = &pxcv1.ReplicasServiceExpose{
						ServiceExpose: pxcv1.ServiceExpose{
							Enabled: true,
						},
					}
				}

				cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Labels = map[string]string{"cr-label": "test"}
				cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Annotations = map[string]string{"cr-annotation": "test"}
				cr.Spec.ProxySQL.Expose.Labels = map[string]string{"cr-label": "test"}
				cr.Spec.ProxySQL.Expose.Annotations = map[string]string{"cr-annotation": "test"}
				Expect(k8sClient.Update(ctx, cr)).Should(Succeed())
			})
			It("should reconcile PerconaXtraDBCluster", func() {
				_, err := reconciler().Reconcile(ctx, reconcile.Request{
					NamespacedName: crNamespacedName,
				})
				Expect(err).To(Succeed())
			})
			It("check labels and annotations", func() {
				for i := range services {
					svc := new(corev1.Service)

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(services[i]), svc)).To(Succeed())

					Expect(svc.Labels["manual-label"]).To(Equal(""))
					Expect(svc.Annotations["manual-annotation"]).To(Equal(""))
					Expect(svc.Labels["ignored-label"]).To(Equal("test"))
					Expect(svc.Annotations["ignored-annotation"]).To(Equal("test"))
					Expect(svc.Labels["cr-label"]).To(Equal("test"))
					Expect(svc.Annotations["cr-annotation"]).To(Equal("test"))
				}
			})
		})
		Context("remove ignored labels and annotations", func() {
			It("should update cr", func() {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
				cr.Spec.IgnoreAnnotations = []string{}
				cr.Spec.IgnoreLabels = []string{}
				Expect(k8sClient.Update(ctx, cr)).Should(Succeed())
			})
			It("should reconcile PerconaXtraDBCluster", func() {
				_, err := reconciler().Reconcile(ctx, reconcile.Request{
					NamespacedName: crNamespacedName,
				})
				Expect(err).To(Succeed())
			})
			It("should check if there are no ignored labels and annotations", func() {
				for i := range services {
					svc := new(corev1.Service)

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(services[i]), svc)).To(Succeed())

					Expect(svc.Labels["ignored-label"]).To(Equal(""))
					Expect(svc.Annotations["ignored-annotation"]).To(Equal(""))
					Expect(svc.Labels["cr-label"]).To(Equal("test"))
					Expect(svc.Annotations["cr-annotation"]).To(Equal("test"))
				}
			})
		})
	}

	services := []*corev1.Service{
		pxc.NewServicePXC(cr),
		pxc.NewServiceHAProxy(cr),
		pxc.NewServiceHAProxyReplicas(cr),
	}

	Context("check haproxy cluster", func() {
		checkLabelsAndAnnotations(services)
	})

	It("should delete services", func() {
		for _, svc := range services {
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
		}
	})

	It("should switch to ProxySQL and remove serviceLabels, serviceAnnotations", func() {
		haproxySts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-haproxy",
				Namespace: cr.Namespace,
			},
		}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(haproxySts), haproxySts)).To(Succeed())
		Expect(k8sClient.Delete(ctx, haproxySts)).To(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
		cr.Spec.HAProxy.Enabled = false
		cr.Spec.ProxySQL.Enabled = true

		cr.Spec.PXC.Expose.Labels = nil
		cr.Spec.PXC.Expose.Annotations = nil
		cr.Spec.HAProxy.ExposePrimary.Labels = nil
		cr.Spec.HAProxy.ExposePrimary.Annotations = nil
		cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Labels = nil
		cr.Spec.HAProxy.ExposeReplicas.ServiceExpose.Annotations = nil
		cr.Spec.ProxySQL.Expose.Labels = nil
		cr.Spec.ProxySQL.Expose.Annotations = nil
		Expect(k8sClient.Update(ctx, cr)).To(Succeed())
	})
	It("should reconcile PerconaXtraDBCluster", func() {
		_, err := reconciler().Reconcile(ctx, reconcile.Request{
			NamespacedName: crNamespacedName,
		})
		Expect(err).To(Succeed())
	})

	Context("check proxysql cluster", func() {
		checkLabelsAndAnnotations([]*corev1.Service{
			pxc.NewServicePXC(cr),
			pxc.NewServiceProxySQL(cr),
		})
	})
})
