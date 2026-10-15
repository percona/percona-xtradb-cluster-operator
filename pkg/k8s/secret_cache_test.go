package k8s

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const helmReleaseType corev1.SecretType = "helm.sh/release.v1"

type syncResultCache struct {
	cache.Cache
	synced bool
}

func (c syncResultCache) WaitForCacheSync(context.Context) bool {
	return c.synced
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

var _ = Describe("Excluded Secret types cache", func() {
	It("keeps only what the existence check needs", func() {
		secret := secretMetadata()
		secret.Name = "sh.helm.release.v1.app.v1"
		secret.Namespace = "pxc"
		secret.Labels = map[string]string{"owner": "helm"}
		secret.Annotations = map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"}
		secret.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "helm"}}
		secret.OwnerReferences = []metav1.OwnerReference{{Name: "owner"}}
		secret.Finalizers = []string{"example.com/finalizer"}

		out, err := keepNameOnly(secret)
		Expect(err).NotTo(HaveOccurred())
		kept, ok := out.(*metav1.PartialObjectMetadata)
		Expect(ok).To(BeTrue())
		Expect(kept.Name).To(Equal("sh.helm.release.v1.app.v1"))
		Expect(kept.Namespace).To(Equal("pxc"))
		Expect(kept.Labels).To(BeEmpty())
		Expect(kept.Annotations).To(BeEmpty())
		Expect(kept.ManagedFields).To(BeEmpty())
		Expect(kept.OwnerReferences).To(BeEmpty())
		Expect(kept.Finalizers).To(BeEmpty())
	})

	It("waits for every cache to sync", func(ctx SpecContext) {
		wrapper := &excludedSecretTypesCache{
			Cache:    syncResultCache{synced: true},
			excluded: map[corev1.SecretType]cache.Cache{helmReleaseType: syncResultCache{synced: false}},
		}
		Expect(wrapper.WaitForCacheSync(ctx)).To(BeFalse(), "an unsynced metadata cache must hold back the sync")

		wrapper.excluded[helmReleaseType] = syncResultCache{synced: true}
		Expect(wrapper.WaitForCacheSync(ctx)).To(BeTrue())

		wrapper.Cache = syncResultCache{synced: false}
		Expect(wrapper.WaitForCacheSync(ctx)).To(BeFalse(), "an unsynced main cache must hold back the sync")
	})

	Context("with an API server", Ordered, func() {
		const namespace = "pxc"

		var (
			testEnv        *envtest.Environment
			cfg            *rest.Config
			opaqueSecret   *corev1.Secret
			helmSecret     *corev1.Secret
			secretRequests atomic.Int64
		)

		BeforeAll(func(ctx SpecContext) {
			testEnv = &envtest.Environment{}
			var err error
			cfg, err = testEnv.Start()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(testEnv.Stop)

			k8sClient, err := client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())

			opaqueSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-ssl", Namespace: namespace},
				Type:       corev1.SecretTypeOpaque,
			}
			helmSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "sh.helm.release.v1.app.v1",
					Namespace:   namespace,
					Labels:      map[string]string{"owner": "helm"},
					Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
				},
				Type: helmReleaseType,
			}
			for _, secret := range []*corev1.Secret{opaqueSecret, helmSecret} {
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}
		})

		It("serves every Secret read from the caches", func() {
			cacheCfg := rest.CopyConfig(cfg)
			cacheCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
				return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if path := req.URL.Path; strings.HasSuffix(path, "/secrets") || strings.Contains(path, "/secrets/") {
						secretRequests.Add(1)
					}
					return rt.RoundTrip(req)
				})
			}

			newCache := NewExcludedSecretTypesCache([]corev1.SecretType{helmReleaseType})
			informerCache, err := newCache(cacheCfg, cache.Options{
				Scheme: scheme.Scheme,
				ByObject: map[client.Object]cache.ByObject{
					&corev1.Secret{}: {Field: fields.OneTermNotEqualSelector("type", string(helmReleaseType))},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			stopped := make(chan error, 1)
			go func() { stopped <- informerCache.Start(ctx) }()
			DeferCleanup(func() {
				cancel()
				Expect(<-stopped).To(Succeed())
			})

			_, err = informerCache.GetInformer(ctx, &corev1.Secret{})
			Expect(err).NotTo(HaveOccurred())
			Expect(informerCache.WaitForCacheSync(ctx)).To(BeTrue(), "caches did not sync")
			secretRequests.Store(0)

			Expect(informerCache.Get(ctx, client.ObjectKeyFromObject(opaqueSecret), &corev1.Secret{})).To(Succeed())

			err = informerCache.Get(ctx, client.ObjectKeyFromObject(helmSecret), &corev1.Secret{})
			Expect(err).To(HaveOccurred())
			Expect(k8serrors.IsNotFound(err)).To(BeFalse(), "an excluded Secret must not look missing")
			Expect(err.Error()).To(ContainSubstring("PXCO_SECRET_CACHE_EXCLUDED_TYPES"))

			wrapper, ok := informerCache.(*excludedSecretTypesCache)
			Expect(ok).To(BeTrue())
			helmMetadata := secretMetadata()
			Expect(wrapper.excluded[helmReleaseType].Get(ctx, client.ObjectKeyFromObject(helmSecret), helmMetadata)).To(Succeed())
			Expect(helmMetadata.Annotations).To(BeEmpty(), "the metadata cache must not keep annotations")
			Expect(helmMetadata.Labels).To(BeEmpty(), "the metadata cache must not keep labels")

			for _, missing := range []client.ObjectKey{{Namespace: namespace, Name: "missing"}, {Namespace: namespace}} {
				err = informerCache.Get(ctx, missing, &corev1.Secret{})
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "a missing Secret %q must still be NotFound, got %v", missing, err)
			}

			Expect(secretRequests.Load()).To(BeZero(), "after the sync, Secret reads must be served from the caches: no new list, get or watch")
		})
	})
})
