package k8s

import (
	"context"
	stderrors "errors"
	"net/http"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// secretRequests counts the Secret gets and watches that carry Secret data,
// that is all of them except the metadata informer's.
type secretRequests struct {
	gets    atomic.Int64
	watches atomic.Int64
}

func (r *secretRequests) wrap(cfg *rest.Config) *rest.Config {
	cfg = rest.CopyConfig(cfg)
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			metadataOnly := strings.Contains(req.Header.Get("Accept"), "PartialObjectMetadata")
			switch path := req.URL.Path; {
			case metadataOnly:
			case req.Method == http.MethodGet && strings.Contains(path, "/secrets/"):
				r.gets.Add(1)
			case strings.HasSuffix(path, "/secrets") && req.URL.Query().Get("watch") == "true":
				r.watches.Add(1)
			}
			return rt.RoundTrip(req)
		})
	}
	return cfg
}

// fetchSecret fetches the Secret as Get does, without the timing.
func fetchSecret(c *secretCache, ctx context.Context, key client.ObjectKey, metadataVersion string) (*corev1.Secret, error) {
	secret, _, err := c.timedFetch(ctx, key, metadataVersion)
	return secret, err
}

// callersWaitingIn counts the goroutines blocked in a select in fn.
func callersWaitingIn(fn string) int {
	stacks := make([]byte, 1<<20)
	for n := goruntime.Stack(stacks, true); ; n = goruntime.Stack(stacks, true) {
		if n < len(stacks) {
			stacks = stacks[:n]
			break
		}
		stacks = make([]byte, 2*len(stacks))
	}
	waiting := 0
	for _, stack := range strings.Split(string(stacks), "\n\n") {
		if strings.Contains(stack, "[select") && strings.Contains(stack, fn+"(") {
			waiting++
		}
	}
	return waiting
}

// releasedOnCleanup returns a channel and a function closing it, which
// also runs when the spec ends, so that readers blocked on the channel do
// not outlive a failed spec.
func releasedOnCleanup() (<-chan struct{}, func()) {
	released := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(released) }) }
	DeferCleanup(release)
	return released, release
}

func closedChannel() <-chan struct{} {
	release := make(chan struct{})
	close(release)
	return release
}

// blockingReader returns its Secret, or err if set, once release is
// closed, or the context's error if it is done first.
type blockingReader struct {
	client.Reader
	release <-chan struct{}
	secret  *corev1.Secret
	err     error
	calls   atomic.Int64
}

func (r *blockingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	r.calls.Add(1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
	}
	if r.err != nil {
		return r.err
	}
	r.secret.DeepCopyInto(obj.(*corev1.Secret))
	return nil
}

// failingReader fails every read with err.
type failingReader struct {
	client.Reader
	err error
}

func (r failingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return r.err
}

// slowReader fails every read with err after delay.
type slowReader struct {
	client.Reader
	err   error
	delay time.Duration
	calls atomic.Int64
}

func (r *slowReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	r.calls.Add(1)
	time.Sleep(r.delay)
	return r.err
}

// flakyReader fails the first failures reads with err.
type flakyReader struct {
	client.Reader
	err      error
	failures int64
	calls    atomic.Int64
}

func (r *flakyReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if r.calls.Add(1) <= r.failures {
		return r.err
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

// metadataCache answers metadata reads with metadata, or with err if set.
type metadataCache struct {
	cache.Cache
	metadata *metav1.PartialObjectMetadata
	err      error
}

func (c metadataCache) Get(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	if c.err != nil {
		return c.err
	}
	c.metadata.DeepCopyInto(obj.(*metav1.PartialObjectMetadata))
	return nil
}

var _ = Describe("Secret cache", func() {
	DescribeTable("tells failures to reach the API server from answers",
		func(err error, transient bool) {
			Expect(transientError(errors.Wrap(err, "get Secret"))).To(Equal(transient))
		},
		Entry("throttling", k8serrors.NewTooManyRequests("throttled", 1), true),
		Entry("server error", k8serrors.NewServiceUnavailable("unavailable"), true),
		Entry("server timeout", k8serrors.NewServerTimeout(corev1.Resource("secrets"), "get", 1), true),
		Entry("TLS handshake timeout", stderrors.New("net/http: TLS handshake timeout"), true),
		Entry("deadline", context.DeadlineExceeded, true),
		Entry("not found", k8serrors.NewNotFound(corev1.Resource("secrets"), "s"), false),
		Entry("forbidden", k8serrors.NewForbidden(corev1.Resource("secrets"), "s", nil), false),
	)

	It("refuses to select Secrets by label or field", func() {
		selected := labels.SelectorFromSet(labels.Set{"app": "pxc"})
		var secrets client.Object = &corev1.Secret{}
		for _, options := range []cache.Options{
			{DefaultLabelSelector: selected},
			{DefaultFieldSelector: fields.OneTermEqualSelector("metadata.name", "s")},
			{ByObject: map[client.Object]cache.ByObject{secrets: {Label: selected}}},
			{DefaultNamespaces: map[string]cache.Config{"pxc": {LabelSelector: selected}}},
			{ByObject: map[client.Object]cache.ByObject{secrets: {Namespaces: map[string]cache.Config{"pxc": {LabelSelector: selected}}}}},
		} {
			_, err := NewSecretCache()(&rest.Config{}, options)
			Expect(err).To(MatchError(ContainSubstring("cannot be selected by label or field")), "options %+v", options)
		}
	})

	It("is told apart from the stock cache", func() {
		Expect(IsSecretCache(&secretCache{})).To(BeTrue())
		Expect(IsSecretCache(&informertest.FakeInformers{})).To(BeFalse())
	})

	It("drops the metadata that can be large", func() {
		secret := secretMetadata()
		secret.Name = "sh.helm.release.v1.app.v1"
		secret.Namespace = "pxc"
		secret.ResourceVersion = "42"
		secret.Labels = map[string]string{"owner": "helm"}
		secret.Annotations = map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"}
		secret.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "helm"}}
		secret.OwnerReferences = []metav1.OwnerReference{{Name: "owner"}}
		secret.Finalizers = []string{"example.com/finalizer"}

		out, err := dropBulkyMetadata(secret)
		Expect(err).NotTo(HaveOccurred())
		kept, ok := out.(*metav1.PartialObjectMetadata)
		Expect(ok).To(BeTrue())
		Expect(kept.Name).To(Equal("sh.helm.release.v1.app.v1"))
		Expect(kept.Namespace).To(Equal("pxc"))
		Expect(kept.ResourceVersion).To(Equal("42"))
		Expect(kept.Labels).To(Equal(map[string]string{"owner": "helm"}))
		Expect(kept.OwnerReferences).To(Equal([]metav1.OwnerReference{{Name: "owner"}}))
		Expect(kept.Annotations).To(BeEmpty())
		Expect(kept.ManagedFields).To(BeEmpty())
		Expect(kept.Finalizers).To(BeEmpty())
	})

	It("runs the default transform before its own", func() {
		byObject, err := withSecretTransform(nil, func(in any) (any, error) {
			in.(*metav1.PartialObjectMetadata).SetLabels(map[string]string{"set": "by default"})
			return in, nil
		}, scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		var transform toolscache.TransformFunc
		for _, options := range byObject {
			transform = options.Transform
		}
		out, err := transform(secretMetadata())
		Expect(err).NotTo(HaveOccurred())
		Expect(out.(*metav1.PartialObjectMetadata).Labels).To(HaveKeyWithValue("set", "by default"))
	})

	It("runs a Secret transform set by the caller before its own", func() {
		var callers client.Object = &corev1.Secret{}
		byObject, err := withSecretTransform(map[client.Object]cache.ByObject{
			callers: {Transform: func(in any) (any, error) {
				in.(*metav1.PartialObjectMetadata).SetLabels(map[string]string{"set": "by caller"})
				return in, nil
			}},
		}, nil, scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		Expect(byObject).To(HaveLen(1), "Secrets must have one entry, whatever its key")

		secret := secretMetadata()
		secret.Annotations = map[string]string{"large": "value"}
		out, err := byObject[callers].Transform(secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.(*metav1.PartialObjectMetadata).Labels).To(HaveKeyWithValue("set", "by caller"))
		Expect(out.(*metav1.PartialObjectMetadata).Annotations).To(BeEmpty())
	})

	It("lists Secrets in the namespaces their informer watches", func() {
		var secrets client.Object = &corev1.Secret{}
		namespaces, err := secretNamespaces(cache.Options{
			Scheme:            scheme.Scheme,
			DefaultNamespaces: map[string]cache.Config{"a": {}, "b": {}},
			ByObject:          map[client.Object]cache.ByObject{secrets: {Namespaces: map[string]cache.Config{"b": {}}}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(namespaces).To(Equal([]string{"b"}), "Namespaces set for Secrets must take precedence over DefaultNamespaces")

		namespaces, err = secretNamespaces(cache.Options{Scheme: scheme.Scheme, DefaultNamespaces: map[string]cache.Config{"a": {}, "b": {}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(namespaces).To(Equal([]string{"a", "b"}))

		namespaces, err = secretNamespaces(cache.Options{
			Scheme:            scheme.Scheme,
			DefaultNamespaces: map[string]cache.Config{"a": {}},
			ByObject:          map[client.Object]cache.ByObject{secrets: {Namespaces: map[string]cache.Config{}}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(namespaces).To(BeEmpty(), "empty Namespaces set for Secrets means all namespaces")
	})

	It("evicts Secrets that go unread", func() {
		now := time.Now()
		read := client.ObjectKey{Namespace: "pxc", Name: "read"}
		unread := client.ObjectKey{Namespace: "pxc", Name: "unread"}
		c := &secretCache{
			now: func() time.Time { return now },
			entries: map[client.ObjectKey]*secretEntry{
				read:   {lastRead: now.Add(-secretIdleTimeout / 2)},
				unread: {lastRead: now.Add(-secretIdleTimeout - time.Second)},
			},
		}
		c.evictIdle()
		Expect(c.entries).To(HaveKey(read))
		Expect(c.entries).NotTo(HaveKey(unread))
	})

	Context("with the operator's own writes", func() {
		key := client.ObjectKey{Namespace: "pxc", Name: "cluster1-secrets"}
		var (
			c   *secretCache
			now time.Time
		)
		version := func(rv string) *metav1.PartialObjectMetadata {
			metadata := secretMetadata()
			metadata.ResourceVersion = rv
			return metadata
		}
		written := func(rv string) *corev1.Secret {
			return &corev1.Secret{
				TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name, ResourceVersion: rv},
				Data:       map[string][]byte{"root": []byte("written")},
			}
		}

		BeforeEach(func() {
			now = time.Now()
			c = &secretCache{now: func() time.Time { return now }, entries: map[client.ObjectKey]*secretEntry{}}
		})

		It("serves an update while the informer shows the version before it", func() {
			c.wrote(written("5"), version("4"), true)
			Expect(c.stored(key, version("4"), true)).To(Equal(written("5")))
		})

		It("serves a create while the informer has not seen the Secret", func() {
			c.wrote(written("5"), secretMetadata(), false)
			Expect(c.stored(key, secretMetadata(), false)).To(Equal(written("5")))
		})

		It("keeps serving the write once the informer shows it", func() {
			c.wrote(written("5"), version("4"), true)
			Expect(c.stored(key, version("5"), true)).To(Equal(written("5")))
			Expect(c.entries[key].write).To(BeNil())

			now = now.Add(writeVisibilityTimeout)
			Expect(c.stored(key, version("5"), true)).To(Equal(written("5")))
		})

		It("drops the write when the informer shows another change", func() {
			c.wrote(written("5"), version("4"), true)
			Expect(c.stored(key, version("6"), true)).To(BeNil())
			Expect(c.entries[key].write).To(BeNil(), "the outdated Secret is kept only to serve if a read fails")
			Expect(c.outdated(key, version("6"))).To(Equal(written("5")))
		})

		It("keeps an outdated Secret it serves after a failed read", func() {
			c.wrote(written("5"), version("4"), true)
			Expect(c.stored(key, version("6"), true)).To(BeNil())
			now = now.Add(secretIdleTimeout)
			Expect(c.outdated(key, version("6"))).To(Equal(written("5")))
			now = now.Add(time.Minute)
			c.evictIdle()
			Expect(c.entries).To(HaveKey(key), "serving the outdated Secret is a read")
		})

		It("drops the write when the informer shows the Secret deleted", func() {
			c.wrote(written("5"), version("4"), true)
			Expect(c.stored(key, secretMetadata(), false)).To(BeNil())
			Expect(c.entries).NotTo(HaveKey(key))
		})

		It("keeps a Secret recreated under the same name when the old one's deletion arrives", func() {
			recreated := written("5")
			recreated.UID = "new"
			c.wrote(recreated, version("4"), true)
			deleted := version("4")
			deleted.Name, deleted.Namespace, deleted.UID = key.Name, key.Namespace, "old"
			c.forget(toolscache.DeletedFinalStateUnknown{Key: key.String(), Obj: deleted})
			Expect(c.entries).To(HaveKey(key))

			deleted.UID = "new"
			c.forget(toolscache.DeletedFinalStateUnknown{Key: key.String(), Obj: deleted})
			Expect(c.entries).NotTo(HaveKey(key))
		})

		It("serves a replacement while the informer shows the old Secret deleted", func() {
			replacement := written("6")
			replacement.UID = "new"
			old := version("5")
			old.UID = "old"
			c.wrote(replacement, old, true)
			Expect(c.stored(key, secretMetadata(), false)).To(Equal(replacement))

			c.wrote(written("7"), version("6"), true)
			Expect(c.stored(key, secretMetadata(), false)).To(BeNil(), "a Secret deleted after the write must be NotFound")
		})

		It("stops trusting a replacement the informer shows deleted sooner", func() {
			replacement := written("6")
			replacement.UID = "new"
			old := version("5")
			old.UID = "old"
			c.wrote(replacement, old, true)
			now = now.Add(createVisibilityTimeout)
			Expect(c.stored(key, secretMetadata(), false)).To(BeNil())
		})

		It("keeps the newer of two overlapping writes", func() {
			c.wrote(written("6"), version("4"), true)
			c.wrote(written("5"), version("4"), true)
			Expect(c.stored(key, version("4"), true)).To(Equal(written("6")))
		})

		It("serves a write while the informer shows an earlier write", func() {
			c.wrote(written("7"), version("4"), true)
			Expect(c.stored(key, version("5"), true)).To(Equal(written("7")))
		})

		It("replaces a write with a newer Secret a fetch got", func() {
			c.wrote(written("5"), version("4"), true)
			release := make(chan struct{})
			close(release)
			c.reader = &blockingReader{release: release, secret: written("7")}
			_, err := fetchSecret(c, context.Background(), key, "4")
			Expect(err).NotTo(HaveOccurred())
			Expect(c.stored(key, version("4"), true)).To(Equal(written("7")))
			Expect(c.entries[key].write).NotTo(BeNil(), "the informer has not shown the write yet")
			Expect(c.stored(key, version("7"), true)).To(Equal(written("7")))
			Expect(c.entries[key].write).To(BeNil())
		})

		It("serves a replacement while the informer shows a newer version of the replaced Secret", func() {
			replacement := written("10")
			replacement.UID = "new"
			old := version("5")
			old.UID = "old"
			c.wrote(replacement, old, true)
			changed := version("6")
			changed.UID = "old"
			Expect(c.stored(key, changed, true)).To(Equal(replacement))
		})

		It("keeps serving a replacement after a fetch got a newer version of it", func() {
			replacement := written("10")
			replacement.UID = "new"
			old := version("5")
			old.UID = "old"
			c.wrote(replacement, old, true)
			patched := written("11")
			patched.UID = "new"
			release := make(chan struct{})
			close(release)
			c.reader = &blockingReader{release: release, secret: patched}
			_, err := fetchSecret(c, context.Background(), key, "5")
			Expect(err).NotTo(HaveOccurred())
			Expect(c.stored(key, secretMetadata(), false)).To(Equal(patched), "the informer shows the replaced Secret deleted")
		})

		It("drops the stored Secret when a Delete fails", func() {
			c.wrote(written("5"), secretMetadata(), false)
			writer := &secretWritingClient{Client: fake.NewClientBuilder().Build(), secrets: c}
			err := writer.Delete(context.Background(), written("5"))
			Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "got %v", err)
			Expect(c.entries).NotTo(HaveKey(key))
		})

		It("keeps a newer Secret when an older fetch finishes later", func() {
			c.entries[key] = &secretEntry{metadataVersion: "5", secret: written("9"), lastRead: now}
			release := make(chan struct{})
			close(release)
			c.reader = &blockingReader{release: release, secret: written("8")}
			_, err := fetchSecret(c, context.Background(), key, "4")
			Expect(err).NotTo(HaveOccurred())
			Expect(c.entries[key].secret).To(Equal(written("9")))
		})

		It("stops trusting a write the informer never shows", func() {
			c.wrote(written("5"), version("4"), true)
			now = now.Add(writeVisibilityTimeout)
			Expect(c.stored(key, version("4"), true)).To(BeNil())
		})

		It("stops trusting a create the informer never shows sooner", func() {
			c.wrote(written("5"), secretMetadata(), false)
			now = now.Add(createVisibilityTimeout - time.Second)
			Expect(c.stored(key, secretMetadata(), false)).To(Equal(written("5")))
			now = now.Add(time.Second)
			Expect(c.stored(key, secretMetadata(), false)).To(BeNil())
		})

		It("keeps a write made while a fetch was in flight", func() {
			released, release := releasedOnCleanup()
			reader := &blockingReader{release: released, secret: written("4")}
			c.reader = reader
			done := make(chan error, 1)
			go func() {
				_, err := fetchSecret(c, context.Background(), key, "4")
				done <- err
			}()
			Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
			c.wrote(written("5"), version("4"), true)
			release()
			Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
			Expect(c.stored(key, version("4"), true)).To(Equal(written("5")))
		})

		It("does not store a fetch that started before the stored Secret was dropped", func() {
			released, release := releasedOnCleanup()
			reader := &blockingReader{release: released, secret: written("4")}
			c.reader = reader
			done := make(chan error, 1)
			go func() {
				_, err := fetchSecret(c, context.Background(), key, "4")
				done <- err
			}()
			Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
			c.forgetKey(key)
			release()
			Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
			Expect(c.outdated(key, version("5"))).To(BeNil(), "a newer version may have been read since the drop")
		})

		It("does not share a fetch that started before the Secret was dropped", func() {
			released, release := releasedOnCleanup()
			reader := &blockingReader{release: released, secret: written("4")}
			c.reader = reader
			done := make(chan error, 2)
			fetch := func() {
				_, err := fetchSecret(c, context.Background(), key, "4")
				done <- err
			}
			go fetch()
			Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
			c.forgetKey(key)
			go fetch()
			Eventually(reader.calls.Load).Should(BeEquivalentTo(2), "a read after the operator's write must not get the Secret as read before it")
			release()
			Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
			Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
		})

		It("does not store a fetch that started before its namespace was dropped", func() {
			released, release := releasedOnCleanup()
			reader := &blockingReader{release: released, secret: written("4")}
			c.reader = reader
			done := make(chan error, 1)
			go func() {
				_, err := fetchSecret(c, context.Background(), key, "4")
				done <- err
			}()
			Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
			c.forgetNamespace(key.Namespace)
			release()
			Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
			Expect(c.outdated(key, version("5"))).To(BeNil(), "a newer version may have been read since the drop")
		})

		It("does not replace the Secret that replaced a dropped one with a fetch that started before the drop", func() {
			released, release := releasedOnCleanup()
			deleted := written("4")
			deleted.UID = "deleted"
			reader := &blockingReader{release: released, secret: deleted}
			c.reader = reader
			done := make(chan error, 1)
			go func() {
				_, err := fetchSecret(c, context.Background(), key, "4")
				done <- err
			}()
			Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
			c.forgetKey(key)
			recreated := written("5")
			recreated.UID = "recreated"
			c.mu.Lock()
			c.entries[key] = &secretEntry{metadataVersion: "4", secret: recreated, lastRead: now}
			c.mu.Unlock()
			release()
			Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
			Expect(c.entries[key].secret.UID).To(BeEquivalentTo("recreated"), "a deleted Secret must not replace the one read after the drop")
		})

		It("stores a fetch that started before another Secret was dropped", func() {
			c.entries[key] = &secretEntry{metadataVersion: "5", secret: written("5"), lastRead: now}
			released, release := releasedOnCleanup()
			reader := &blockingReader{release: released, secret: written("7")}
			c.reader = reader
			done := make(chan error, 1)
			go func() {
				_, err := fetchSecret(c, context.Background(), key, "7")
				done <- err
			}()
			Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
			c.forgetKey(client.ObjectKey{Namespace: key.Namespace, Name: "other"})
			release()
			Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
			Expect(c.outdated(key, version("8"))).To(Equal(written("7")), "an earlier version must not be served after a newer one was read")
		})

		It("keeps the newer metadata of two fetches that got the same version", func() {
			c.entries[key] = &secretEntry{metadataVersion: "5", secret: written("5"), lastRead: now}
			c.reader = &blockingReader{release: closedChannel(), secret: written("5")}
			_, err := fetchSecret(c, context.Background(), key, "4")
			Expect(err).NotTo(HaveOccurred())
			Expect(c.stored(key, version("5"), true)).To(Equal(written("5")), "a stored Secret must be served without fetching it again")
		})

		It("does not serve a deleted Secret as last read for one recreated under its name", func() {
			deleted := written("4")
			deleted.UID = "deleted"
			c.entries[key] = &secretEntry{metadataVersion: "4", secret: deleted, lastRead: now}
			recreated := version("7")
			recreated.UID = "recreated"
			Expect(c.outdated(key, recreated)).To(BeNil())
		})

		It("keeps a Secret that is read", func() {
			c.entries[key] = &secretEntry{metadataVersion: "5", secret: written("5"), lastRead: now}
			now = now.Add(secretIdleTimeout - time.Minute)
			Expect(c.stored(key, version("5"), true)).NotTo(BeNil())
			now = now.Add(2 * time.Minute)
			c.evictIdle()
			Expect(c.entries).To(HaveKey(key))
		})

		It("stores a Secret fetched again at the metadata that shows it", func() {
			c.entries[key] = &secretEntry{metadataVersion: "5", secret: written("7"), lastRead: now}
			c.reader = &blockingReader{release: closedChannel(), secret: written("7")}
			_, err := fetchSecret(c, context.Background(), key, "7")
			Expect(err).NotTo(HaveOccurred())
			Expect(c.stored(key, version("7"), true)).To(Equal(written("7")), "a stored Secret must be served without fetching it again")
		})
	})

	It("finishes a shared fetch for the other callers when the first gives up", func() {
		key := client.ObjectKey{Namespace: "pxc", Name: "cluster1-secrets"}
		released, release := releasedOnCleanup()
		reader := &blockingReader{release: released, secret: &corev1.Secret{Data: map[string][]byte{"root": []byte("fetched")}}}
		c := &secretCache{reader: reader, now: time.Now, entries: map[client.ObjectKey]*secretEntry{}}

		firstCtx, cancelFirst := context.WithCancel(context.Background())
		first := make(chan error, 1)
		go func() {
			_, err := fetchSecret(c, firstCtx, key, "4")
			first <- err
		}()
		Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
		second := make(chan error, 1)
		go func() {
			_, err := fetchSecret(c, context.Background(), key, "4")
			second <- err
		}()
		Eventually(func() int { return callersWaitingIn("(*secretCache).timedFetch") }).WithTimeout(10 * time.Second).Should(Equal(2))
		cancelFirst()
		Eventually(first).WithTimeout(10 * time.Second).Should(Receive(MatchError(context.Canceled)))
		release()
		Eventually(second).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
		Expect(reader.calls.Load()).To(BeEquivalentTo(1), "the callers must share one request")
	})

	It("returns the context's error when it ends while waiting to retry", func() {
		key := client.ObjectKey{Namespace: "pxc", Name: "cluster1-secrets"}
		unavailable := k8serrors.NewServiceUnavailable("unavailable")
		c := &secretCache{reader: failingReader{err: unavailable}, now: time.Now, entries: map[client.ObjectKey]*secretEntry{}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		_, err := c.retryFetch(ctx, key, "4", unavailable)

		Expect(err).To(MatchError(context.Canceled))
		Expect(time.Since(start)).To(BeNumerically("<", fetchRetryDelays[0]), "the retry must not wait once the context ended")
	})

	It("waits for a slow fetch of a Secret with no stored copy", func() {
		key := client.ObjectKey{Namespace: "pxc", Name: "cluster1-secrets"}
		metadata := secretMetadata()
		metadata.Namespace, metadata.Name, metadata.ResourceVersion = key.Namespace, key.Name, "4"
		wait := outdatedFetchWait
		DeferCleanup(func() { outdatedFetchWait = wait })
		outdatedFetchWait = 50 * time.Millisecond
		released, release := releasedOnCleanup()
		time.AfterFunc(3*outdatedFetchWait, release)
		c := &secretCache{
			Cache:   metadataCache{metadata: metadata},
			scheme:  scheme.Scheme,
			reader:  &blockingReader{release: released, secret: &corev1.Secret{Data: map[string][]byte{"root": []byte("fetched")}}},
			now:     time.Now,
			entries: map[client.ObjectKey]*secretEntry{},
		}

		secret := &corev1.Secret{}
		Expect(c.Get(context.Background(), key, secret)).To(Succeed(), "only a read with a stored copy to serve stops waiting early")
		Expect(secret.Data).To(HaveKeyWithValue("root", []byte("fetched")))
	})

	It("stops retrying a fetch that failed slowly", func() {
		key := client.ObjectKey{Namespace: "pxc", Name: "cluster1-secrets"}
		failedFastWithin, delays := fetchFailedFast, fetchRetryDelays
		DeferCleanup(func() { fetchFailedFast, fetchRetryDelays = failedFastWithin, delays })
		fetchFailedFast, fetchRetryDelays = 50*time.Millisecond, []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
		unavailable := k8serrors.NewServiceUnavailable("unavailable")
		reader := &slowReader{err: unavailable, delay: 2 * fetchFailedFast}
		c := &secretCache{reader: reader, now: time.Now, entries: map[client.ObjectKey]*secretEntry{}}

		_, err := c.retryFetch(context.Background(), key, "4", unavailable)

		Expect(err).To(MatchError(unavailable))
		Expect(reader.calls.Load()).To(BeEquivalentTo(1), "a slow failure must not be retried")
	})

	It("returns the context's error from a read when it ends while waiting to retry", func() {
		key := client.ObjectKey{Namespace: "pxc", Name: "cluster1-secrets"}
		metadata := secretMetadata()
		metadata.Namespace, metadata.Name, metadata.ResourceVersion = key.Namespace, key.Name, "4"
		c := &secretCache{
			Cache:   metadataCache{metadata: metadata},
			scheme:  scheme.Scheme,
			reader:  failingReader{err: k8serrors.NewServiceUnavailable("unavailable")},
			now:     time.Now,
			entries: map[client.ObjectKey]*secretEntry{},
		}
		ctx, cancel := context.WithTimeout(context.Background(), fetchRetryDelays[0]/2)
		defer cancel()

		err := c.Get(ctx, key, &corev1.Secret{})

		Expect(err).To(MatchError(context.DeadlineExceeded))
		Expect(IsSecretUnavailable(err)).To(BeFalse(), "the caller's own deadline is not a failure to reach the API server")
	})

	Context("with a writing client", func() {
		inNamespace := func(namespace string) *corev1.Secret {
			return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: namespace}}
		}
		var c *secretCache

		BeforeEach(func() {
			c = &secretCache{now: time.Now, entries: map[client.ObjectKey]*secretEntry{}}
			for _, namespace := range []string{"a", "b"} {
				c.entries[client.ObjectKeyFromObject(inNamespace(namespace))] = &secretEntry{secret: inNamespace(namespace), lastRead: time.Now()}
			}
		})

		It("drops a Secret written while its metadata could not be read", func(ctx SpecContext) {
			c.Cache = metadataCache{err: k8serrors.NewServiceUnavailable("unavailable")}
			writer := &secretWritingClient{Client: fake.NewClientBuilder().WithObjects(inNamespace("a")).Build(), secrets: c}

			Expect(writer.Update(ctx, inNamespace("a"))).To(Succeed())
			Expect(c.entries).NotTo(HaveKey(client.ObjectKeyFromObject(inNamespace("a"))))
			Expect(c.entries).To(HaveKey(client.ObjectKeyFromObject(inNamespace("b"))))
		})

		It("does not serve a write once the informer shows a newer write made by someone else", func(ctx SpecContext) {
			metadata := secretMetadata()
			metadata.Namespace, metadata.Name, metadata.ResourceVersion = "a", "s", "999"
			c.Cache = metadataCache{metadata: metadata}
			writer := &secretWritingClient{
				Client: fake.NewClientBuilder().WithObjects(inNamespace("a")).WithInterceptorFuncs(interceptor.Funcs{
					Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
						err := cl.Update(ctx, obj, opts...)
						metadata.ResourceVersion = "2000"
						return err
					},
				}).Build(),
				secrets: c,
			}

			Expect(writer.Update(ctx, inNamespace("a"))).To(Succeed())
			Expect(c.stored(client.ObjectKeyFromObject(inNamespace("a")), metadata, true)).To(BeNil(),
				"the metadata must be read before the write: read after, it hides the newer write")
		})

		It("drops the Secrets of all namespaces when they are deleted in bulk across namespaces", func(ctx SpecContext) {
			writer := &secretWritingClient{Client: fake.NewClientBuilder().WithObjects(inNamespace("a"), inNamespace("b")).Build(), secrets: c}

			Expect(writer.DeleteAllOf(ctx, &corev1.Secret{})).To(Succeed())
			Expect(c.entries).To(BeEmpty())
		})
	})

	It("lists Secrets in each namespace it is restricted to", func(ctx SpecContext) {
		inNamespace := func(namespace string) *corev1.Secret {
			return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: namespace}}
		}
		reader := fake.NewClientBuilder().WithObjects(inNamespace("a"), inNamespace("b"), inNamespace("c")).Build()
		c := &secretCache{reader: reader, scheme: scheme.Scheme, namespaces: []string{"a", "b"}, now: time.Now, entries: map[client.ObjectKey]*secretEntry{}}

		secrets := &corev1.SecretList{ListMeta: metav1.ListMeta{ResourceVersion: "earlier", Continue: "earlier"}}
		Expect(c.List(ctx, secrets)).To(Succeed())
		Expect(secrets.Items).To(HaveLen(2))
		Expect([]string{secrets.Items[0].Namespace, secrets.Items[1].Namespace}).To(ConsistOf("a", "b"))
		Expect(secrets.Items[0].Kind).To(Equal("Secret"), "items must have their kind, as from the stock cache")
		Expect(secrets.ResourceVersion).To(BeEmpty())
		Expect(secrets.Continue).To(BeEmpty())

		Expect(c.List(ctx, &corev1.SecretList{}, client.InNamespace("c"))).To(MatchError(ContainSubstring("restricted to namespaces")))
		Expect(c.List(ctx, &corev1.SecretList{}, client.Limit(1))).To(MatchError(ContainSubstring("pagination is not supported")))
	})

	It("shares one request between concurrent fetches of a Secret", func() {
		key := client.ObjectKey{Namespace: "pxc", Name: "cluster1-secrets"}
		released, release := releasedOnCleanup()
		reader := &blockingReader{release: released, secret: &corev1.Secret{Data: map[string][]byte{"root": []byte("fetched")}}}
		c := &secretCache{reader: reader, now: time.Now, entries: map[client.ObjectKey]*secretEntry{}}

		const callers = 8
		var started atomic.Int64
		type result struct {
			secret *corev1.Secret
			err    error
		}
		results := make(chan result, callers)
		for range callers {
			go func() {
				started.Add(1)
				secret, err := fetchSecret(c, context.Background(), key, "4")
				results <- result{secret, err}
			}()
		}
		Eventually(reader.calls.Load).Should(BeEquivalentTo(1))
		Eventually(started.Load).Should(BeEquivalentTo(callers))
		Eventually(func() int { return callersWaitingIn("(*secretCache).timedFetch") }).Should(Equal(callers))
		release()
		for range callers {
			var got result
			Eventually(results).WithTimeout(10 * time.Second).Should(Receive(&got))
			Expect(got.err).NotTo(HaveOccurred())
			Expect(got.secret.Data).To(HaveKeyWithValue("root", []byte("fetched")))
		}
		Expect(reader.calls.Load()).To(BeEquivalentTo(1))
	})

	Context("with an API server", Ordered, func() {
		const namespace = "pxc"

		var (
			cfg        *rest.Config
			k8sClient  client.Client
			writer     client.Client
			c          *secretCache
			requests   *secretRequests
			userSecret *corev1.Secret
			helmSecret *corev1.Secret
		)

		get := func(ctx context.Context, key client.ObjectKey) (*corev1.Secret, error) {
			secret := &corev1.Secret{}
			return secret, c.Get(ctx, key, secret)
		}
		// isStored reads the stored entries under the lock: the informer's
		// delete handler changes them concurrently.
		isStored := func(key client.ObjectKey) bool {
			c.mu.Lock()
			defer c.mu.Unlock()
			_, ok := c.entries[key]
			return ok
		}

		BeforeAll(func(ctx SpecContext) {
			testEnv := &envtest.Environment{}
			var err error
			cfg, err = testEnv.Start()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(testEnv.Stop)

			k8sClient, err = client.New(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())

			userSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-secrets", Namespace: namespace},
				StringData: map[string]string{"root": "first"},
			}
			helmSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "sh.helm.release.v1.app.v1",
					Namespace:   namespace,
					Labels:      map[string]string{"owner": "helm"},
					Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
				},
				Type:       "helm.sh/release.v1",
				StringData: map[string]string{"release": "large"},
			}
			for _, secret := range []*corev1.Secret{userSecret, helmSecret} {
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}

			requests = &secretRequests{}
			informerCache, err := NewSecretCache()(requests.wrap(cfg), cache.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			var ok bool
			c, ok = informerCache.(*secretCache)
			Expect(ok).To(BeTrue())

			cacheCtx, cancel := context.WithCancel(context.Background())
			stopped := make(chan error, 1)
			go func() { stopped <- c.Start(cacheCtx) }()
			DeferCleanup(func() {
				cancel()
				Eventually(stopped).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
			})
			Expect(c.WaitForCacheSync(ctx)).To(BeTrue(), "cache did not sync")

			writer, err = NewSecretClient(requests.wrap(cfg), client.Options{Scheme: scheme.Scheme, Cache: &client.CacheOptions{Reader: c}})
			Expect(err).NotTo(HaveOccurred())
		})

		BeforeEach(func() {
			requests.gets.Store(0)
		})

		AfterAll(func() {
			if requests == nil {
				return // BeforeAll failed and reported why.
			}
			Expect(requests.watches.Load()).To(BeZero(), "Secrets must only be watched as metadata")
		})

		It("fetches a Secret once and serves later reads from the cache", SpecTimeout(time.Minute), func(ctx SpecContext) {
			for range 3 {
				secret, err := get(ctx, client.ObjectKeyFromObject(userSecret))
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Data).To(HaveKeyWithValue("root", []byte("first")))
				Expect(secret.GroupVersionKind()).To(Equal(secretGVK), "Secrets must have their kind set, as from the informer cache")
			}
			Expect(requests.gets.Load()).To(BeEquivalentTo(1))
		})

		It("serves Secret metadata", SpecTimeout(time.Minute), func(ctx SpecContext) {
			_, err := c.GetInformer(ctx, secretMetadata())
			Expect(err).NotTo(HaveOccurred())
			metadata := secretMetadata()
			Expect(c.Get(ctx, client.ObjectKeyFromObject(userSecret), metadata)).To(Succeed())
			Expect(metadata.UID).To(Equal(userSecret.UID))
			Expect(requests.gets.Load()).To(BeZero(), "metadata must be read from the informer")
		})

		It("returns a copy", SpecTimeout(time.Minute), func(ctx SpecContext) {
			secret, err := get(ctx, client.ObjectKeyFromObject(userSecret))
			Expect(err).NotTo(HaveOccurred())
			secret.Data["root"] = []byte("changed by the caller")

			secret, err = get(ctx, client.ObjectKeyFromObject(userSecret))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("root", []byte("first")))
		})

		It("answers missing Secrets without API requests", SpecTimeout(time.Minute), func(ctx SpecContext) {
			for _, missing := range []client.ObjectKey{{Namespace: namespace, Name: "missing"}, {Namespace: namespace}} {
				_, err := get(ctx, missing)
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "a missing Secret %q must be NotFound, got %v", missing, err)
			}
			Expect(requests.gets.Load()).To(BeZero())
		})

		It("keeps neither data nor large metadata in the informer", SpecTimeout(time.Minute), func(ctx SpecContext) {
			metadata := secretMetadata()
			Expect(c.Cache.Get(ctx, client.ObjectKeyFromObject(helmSecret), metadata)).To(Succeed())
			Expect(metadata.Annotations).To(BeEmpty())
			Expect(metadata.Labels).To(HaveKeyWithValue("owner", "helm"))
			Expect(isStored(client.ObjectKeyFromObject(helmSecret))).To(BeFalse(), "a Secret nobody read must not be stored")
		})

		It("returns annotations, including ones changed by an annotation-only write", SpecTimeout(time.Minute), func(ctx SpecContext) {
			annotated := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cluster1-internal",
					Namespace:   namespace,
					Annotations: map[string]string{"cert-manager.io/issuer-kind": "Issuer"},
				},
				StringData: map[string]string{"root": "first"},
			}
			Expect(k8sClient.Create(ctx, annotated)).To(Succeed())
			Eventually(func() (map[string]string, error) {
				secret, err := get(ctx, client.ObjectKeyFromObject(annotated))
				return secret.Annotations, err
			}).WithContext(ctx).WithTimeout(10 * time.Second).Should(HaveKeyWithValue("cert-manager.io/issuer-kind", "Issuer"))

			secret, err := get(ctx, client.ObjectKeyFromObject(annotated))
			Expect(err).NotTo(HaveOccurred())
			secret.Annotations["grant-for-1.10.0-xtrabackup-user"] = "done"
			Expect(writer.Update(ctx, secret)).To(Succeed())
			secret, err = get(ctx, client.ObjectKeyFromObject(annotated))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Annotations).To(HaveKeyWithValue("grant-for-1.10.0-xtrabackup-user", "done"), "the operator must see its own annotation write")

			external := secret.DeepCopy()
			external.Annotations["changed-elsewhere"] = "true"
			Expect(k8sClient.Update(ctx, external)).To(Succeed())
			Eventually(func() (map[string]string, error) {
				secret, err := get(ctx, client.ObjectKeyFromObject(annotated))
				return secret.Annotations, err
			}).WithContext(ctx).WithTimeout(10*time.Second).Should(HaveKeyWithValue("changed-elsewhere", "true"), "an annotation-only change made elsewhere must be read")
		})

		It("fetches a Secret again after it changes", SpecTimeout(time.Minute), func(ctx SpecContext) {
			updated := userSecret.DeepCopy()
			updated.StringData = map[string]string{"root": "second"}
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			Eventually(func(g Gomega) {
				secret, err := get(ctx, client.ObjectKeyFromObject(userSecret))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.Data).To(HaveKeyWithValue("root", []byte("second")))
			}).WithContext(ctx).Should(Succeed())

			requests.gets.Store(0)
			_, err := get(ctx, client.ObjectKeyFromObject(userSecret))
			Expect(err).NotTo(HaveOccurred())
			Expect(requests.gets.Load()).To(BeZero(), "an unchanged Secret must be served from the cache")
		})

		It("serves a changed Secret as last read when reading it fails with a server error", SpecTimeout(time.Minute), func(ctx SpecContext) {
			changed := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-ssl-internal", Namespace: namespace},
				StringData: map[string]string{"tls.crt": "cert"},
			}
			Expect(k8sClient.Create(ctx, changed)).To(Succeed())
			Eventually(func() error {
				_, err := get(ctx, client.ObjectKeyFromObject(changed))
				return err
			}).WithContext(ctx).Should(Succeed())

			reader := c.reader
			DeferCleanup(func() { c.reader = reader })
			c.reader = failingReader{err: k8serrors.NewServiceUnavailable("unavailable")}
			changed.Labels = map[string]string{"changed": "true"}
			Expect(k8sClient.Update(ctx, changed)).To(Succeed())
			Eventually(func(g Gomega) {
				metadata := secretMetadata()
				g.Expect(c.Cache.Get(ctx, client.ObjectKeyFromObject(changed), metadata)).To(Succeed())
				g.Expect(metadata.ResourceVersion).To(Equal(changed.ResourceVersion))
			}).WithContext(ctx).Should(Succeed())

			secret, err := get(ctx, client.ObjectKeyFromObject(changed))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("tls.crt", []byte("cert")))

			c.reader = failingReader{err: k8serrors.NewForbidden(corev1.Resource("secrets"), changed.Name, nil)}
			_, err = get(ctx, client.ObjectKeyFromObject(changed))
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), "only failures to reach the API server are served from the last read, got %v", err)

			c.reader = failingReader{err: k8serrors.NewServiceUnavailable("unavailable")}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			_, err = get(canceled, client.ObjectKeyFromObject(changed))
			Expect(err).To(HaveOccurred(), "the caller's context ending is not a failure to reach the API server")
			// Wait for the fetch the canceled read started before the reader is restored.
			_, err = get(ctx, client.ObjectKeyFromObject(changed))
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not serve a deleted Secret's data for one recreated under its name", SpecTimeout(time.Minute), func(ctx SpecContext) {
			deleted := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-recreated", Namespace: namespace},
				StringData: map[string]string{"root": "deleted"},
			}
			Expect(k8sClient.Create(ctx, deleted)).To(Succeed())
			key := client.ObjectKeyFromObject(deleted)
			Eventually(func() error {
				_, err := get(ctx, key)
				return err
			}).WithContext(ctx).Should(Succeed())
			c.mu.Lock()
			entry := c.entries[key]
			c.mu.Unlock()
			Expect(entry).NotTo(BeNil())

			Expect(k8sClient.Delete(ctx, deleted)).To(Succeed())
			recreated := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: deleted.Name, Namespace: namespace}, StringData: map[string]string{"root": "recreated"}}
			Expect(k8sClient.Create(ctx, recreated)).To(Succeed())
			Eventually(func(g Gomega) {
				metadata := secretMetadata()
				g.Expect(c.Cache.Get(ctx, key, metadata)).To(Succeed())
				g.Expect(metadata.UID).To(Equal(recreated.UID))
			}).WithContext(ctx).Should(Succeed())
			// As after a relist that showed the new Secret without the deletion.
			c.mu.Lock()
			c.entries[key] = entry
			c.mu.Unlock()

			reader := c.reader
			DeferCleanup(func() { c.reader = reader })
			c.reader = failingReader{err: k8serrors.NewServiceUnavailable("unavailable")}
			secret, err := get(ctx, key)
			Expect(IsSecretUnavailable(err)).To(BeTrue(), "the deleted Secret must not be served, got %v with %v", err, secret.Data)
			Expect(isStored(key)).To(BeFalse(), "the deleted Secret must not be kept")
		})

		It("reads a Secret with no stored copy again when reading it fails with a server error", SpecTimeout(time.Minute), func(ctx SpecContext) {
			unread := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "s3-credentials", Namespace: namespace},
				StringData: map[string]string{"AWS_ACCESS_KEY_ID": "key"},
			}
			Expect(k8sClient.Create(ctx, unread)).To(Succeed())
			Eventually(func() error {
				return c.Cache.Get(ctx, client.ObjectKeyFromObject(unread), secretMetadata())
			}).WithContext(ctx).Should(Succeed())

			reader := c.reader
			DeferCleanup(func() { c.reader = reader })
			flaky := &flakyReader{Reader: reader, err: k8serrors.NewServiceUnavailable("unavailable"), failures: 2}
			c.reader = flaky
			secret, err := get(ctx, client.ObjectKeyFromObject(unread))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("AWS_ACCESS_KEY_ID", []byte("key")))
			Expect(flaky.calls.Load()).To(BeEquivalentTo(3))

			Expect(k8sClient.Delete(ctx, unread)).To(Succeed())
			Eventually(func() bool { return isStored(client.ObjectKeyFromObject(unread)) }).WithContext(ctx).Should(BeFalse())
			Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: unread.Name, Namespace: namespace}})).To(Succeed())
			Eventually(func() error {
				return c.Cache.Get(ctx, client.ObjectKeyFromObject(unread), secretMetadata())
			}).WithContext(ctx).Should(Succeed())
			forbidden := &flakyReader{Reader: reader, err: k8serrors.NewForbidden(corev1.Resource("secrets"), unread.Name, nil), failures: 1}
			c.reader = forbidden
			_, err = get(ctx, client.ObjectKeyFromObject(unread))
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), "got %v", err)
			Expect(IsSecretUnavailable(err)).To(BeFalse())
			Expect(forbidden.calls.Load()).To(BeEquivalentTo(1), "an answer from the API server is not retried")

			c.reader = failingReader{err: k8serrors.NewServiceUnavailable("unavailable")}
			_, err = get(ctx, client.ObjectKeyFromObject(unread))
			Expect(k8serrors.IsServiceUnavailable(err)).To(BeTrue(), "got %v", err)
			Expect(IsSecretUnavailable(errors.Wrap(err, "get credentials"))).To(BeTrue(), "callers must be able to retry a Secret the cache could not read")
		})

		It("serves a changed Secret as last read when reading it is slow", SpecTimeout(time.Minute), func(ctx SpecContext) {
			slowSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "slow", Namespace: namespace},
				StringData: map[string]string{"tls.crt": "cert"},
			}
			Expect(k8sClient.Create(ctx, slowSecret)).To(Succeed())
			Eventually(func() error {
				_, err := get(ctx, client.ObjectKeyFromObject(slowSecret))
				return err
			}).WithContext(ctx).Should(Succeed())

			reader, wait := c.reader, outdatedFetchWait
			DeferCleanup(func() { c.reader, outdatedFetchWait = reader, wait })
			released, release := releasedOnCleanup()
			blocked := &blockingReader{release: released}
			c.reader, outdatedFetchWait = blocked, 100*time.Millisecond
			slowSecret.Labels = map[string]string{"changed": "true"}
			Expect(k8sClient.Update(ctx, slowSecret)).To(Succeed())
			Eventually(func(g Gomega) {
				metadata := secretMetadata()
				g.Expect(c.Cache.Get(ctx, client.ObjectKeyFromObject(slowSecret), metadata)).To(Succeed())
				g.Expect(metadata.ResourceVersion).To(Equal(slowSecret.ResourceVersion))
			}).WithContext(ctx).Should(Succeed())

			start := time.Now()
			secret, err := get(ctx, client.ObjectKeyFromObject(slowSecret))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Labels).NotTo(HaveKey("changed"))
			Expect(time.Since(start)).To(BeNumerically("<", time.Second))

			blocked.secret = slowSecret.DeepCopy()
			release()
			// Wait for the slow read to store the Secret: a read before that
			// would start its own request.
			Eventually(func() *corev1.Secret {
				metadata := secretMetadata()
				if err := c.Cache.Get(ctx, client.ObjectKeyFromObject(slowSecret), metadata); err != nil {
					return nil
				}
				return c.stored(client.ObjectKeyFromObject(slowSecret), metadata, true)
			}).WithContext(ctx).WithTimeout(10*time.Second).ShouldNot(BeNil(), "the slow read must go on and store the Secret")
			secret, err = get(ctx, client.ObjectKeyFromObject(slowSecret))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Labels).To(HaveKey("changed"))
			Expect(blocked.calls.Load()).To(BeEquivalentTo(1), "the stored Secret must be served without another read")
		})

		It("does not read a Secret again when reading it failed slowly", SpecTimeout(time.Minute), func(ctx SpecContext) {
			throttled := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "throttled", Namespace: namespace}}
			Expect(k8sClient.Create(ctx, throttled)).To(Succeed())
			Eventually(func() error {
				return c.Cache.Get(ctx, client.ObjectKeyFromObject(throttled), secretMetadata())
			}).WithContext(ctx).Should(Succeed())

			reader, failedFastWithin := c.reader, fetchFailedFast
			DeferCleanup(func() { c.reader, fetchFailedFast = reader, failedFastWithin })
			slow := &slowReader{err: k8serrors.NewTooManyRequests("throttled", 1), delay: 100 * time.Millisecond}
			c.reader, fetchFailedFast = slow, 50*time.Millisecond
			_, err := get(ctx, client.ObjectKeyFromObject(throttled))
			Expect(k8serrors.IsTooManyRequests(err)).To(BeTrue(), "got %v", err)
			Expect(slow.calls.Load()).To(BeEquivalentTo(1), "client-go already retried throttling it was told to wait for")
		})

		It("times a failure by the shared read, not by how long a caller waited", SpecTimeout(time.Minute), func(ctx SpecContext) {
			joined := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "joined", Namespace: namespace}}
			Expect(k8sClient.Create(ctx, joined)).To(Succeed())
			Eventually(func() error {
				return c.Cache.Get(ctx, client.ObjectKeyFromObject(joined), secretMetadata())
			}).WithContext(ctx).Should(Succeed())

			reader, failedFastWithin := c.reader, fetchFailedFast
			DeferCleanup(func() { c.reader, fetchFailedFast = reader, failedFastWithin })
			released, release := releasedOnCleanup()
			slow := &blockingReader{release: released, err: k8serrors.NewServiceUnavailable("unavailable")}
			c.reader, fetchFailedFast = slow, time.Second
			results := make(chan error, 2)
			getJoined := func() {
				_, err := get(ctx, client.ObjectKeyFromObject(joined))
				results <- err
			}
			go getJoined()
			Eventually(slow.calls.Load).WithContext(ctx).Should(BeEquivalentTo(1))
			// The shared read takes longer than fetchFailedFast; the second
			// caller joins it after that and waits for it only briefly.
			time.Sleep(fetchFailedFast + 100*time.Millisecond)
			go getJoined()
			Eventually(func() int { return callersWaitingIn("(*secretCache).timedFetch") }).WithContext(ctx).WithTimeout(10 * time.Second).Should(Equal(2))
			release()
			for range 2 {
				Eventually(results).WithContext(ctx).WithTimeout(10 * time.Second).Should(Receive(Satisfy(k8serrors.IsServiceUnavailable)))
			}
			Expect(slow.calls.Load()).To(BeEquivalentTo(1), "a caller that joined a slow read late must not retry it")
		})

		It("does not read a Secret again when reading it timed out", SpecTimeout(time.Minute), func(ctx SpecContext) {
			hung := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "hung", Namespace: namespace}}
			Expect(k8sClient.Create(ctx, hung)).To(Succeed())
			Eventually(func() error {
				return c.Cache.Get(ctx, client.ObjectKeyFromObject(hung), secretMetadata())
			}).WithContext(ctx).Should(Succeed())

			reader, timeout := c.reader, fetchTimeout
			DeferCleanup(func() { c.reader, fetchTimeout = reader, timeout })
			blocked := &blockingReader{release: make(chan struct{})}
			c.reader, fetchTimeout = blocked, 100*time.Millisecond
			start := time.Now()
			_, err := get(ctx, client.ObjectKeyFromObject(hung))
			Expect(err).To(MatchError(context.DeadlineExceeded))
			Expect(IsSecretUnavailable(err)).To(BeTrue())
			Expect(blocked.calls.Load()).To(BeEquivalentTo(1))
			Expect(time.Since(start)).To(BeNumerically("<", time.Second))
		})

		It("reads Secrets created after the sync", SpecTimeout(time.Minute), func(ctx SpecContext) {
			created := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-ssl", Namespace: namespace},
				StringData: map[string]string{"tls.crt": "cert"},
			}
			_, err := get(ctx, client.ObjectKeyFromObject(created))
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())

			Expect(k8sClient.Create(ctx, created)).To(Succeed())
			Eventually(func(g Gomega) {
				secret, err := get(ctx, client.ObjectKeyFromObject(created))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.Data).To(HaveKeyWithValue("tls.crt", []byte("cert")))
			}).WithContext(ctx).Should(Succeed())
		})

		It("forgets deleted Secrets", SpecTimeout(time.Minute), func(ctx SpecContext) {
			key := client.ObjectKeyFromObject(userSecret)
			_, err := get(ctx, key)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, userSecret)).To(Succeed())
			Eventually(func(g Gomega) {
				_, err := get(ctx, key)
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				g.Expect(isStored(key)).To(BeFalse())
			}).WithContext(ctx).Should(Succeed())
		})

		It("reads its own writes", SpecTimeout(time.Minute), func(ctx SpecContext) {
			written := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-written", Namespace: namespace},
				Data:       map[string][]byte{"root": []byte("created")},
			}
			Expect(writer.Create(ctx, written)).To(Succeed())
			secret, err := get(ctx, client.ObjectKeyFromObject(written))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("root", []byte("created")))
			Expect(secret.GroupVersionKind()).To(Equal(secretGVK))
			Expect(requests.gets.Load()).To(BeZero(), "a Secret the operator created must be served without fetching it")

			// A later write can be fetched once if the informer is still behind
			// the earlier one, but never read stale.
			secret.Data["root"] = []byte("updated")
			Expect(writer.Update(ctx, secret)).To(Succeed())
			secret, err = get(ctx, client.ObjectKeyFromObject(written))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("root", []byte("updated")))

			patched := secret.DeepCopy()
			patched.Data["root"] = []byte("patched")
			Expect(writer.Patch(ctx, patched, client.MergeFrom(secret))).To(Succeed())
			secret, err = get(ctx, client.ObjectKeyFromObject(written))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("root", []byte("patched")))

			Expect(writer.Delete(ctx, secret)).To(Succeed())
			_, err = get(ctx, client.ObjectKeyFromObject(written))
			Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "a deleted Secret must be NotFound, got %v", err)
		})

		It("lists Secrets in the namespaces set for Secrets", func() {
			var secrets client.Object = &corev1.Secret{}
			informerCache, err := NewSecretCache()(cfg, cache.Options{
				Scheme:            scheme.Scheme,
				DefaultNamespaces: map[string]cache.Config{"a": {}, namespace: {}},
				ByObject:          map[client.Object]cache.ByObject{secrets: {Namespaces: map[string]cache.Config{namespace: {}}}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(informerCache.(*secretCache).namespaces).To(Equal([]string{namespace}))
		})

		It("drops Secrets written with server-side apply or deleted in bulk", SpecTimeout(time.Minute), func(ctx SpecContext) {
			applied := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-applied", Namespace: namespace, Labels: map[string]string{"bulk": "true"}},
				Data:       map[string][]byte{"root": []byte("created")},
			}
			Expect(writer.Create(ctx, applied)).To(Succeed())
			key := client.ObjectKeyFromObject(applied)
			_, err := get(ctx, key)
			Expect(err).NotTo(HaveOccurred())

			Expect(writer.Apply(ctx, corev1ac.Secret(applied.Name, applied.Namespace).
				WithData(map[string][]byte{"root": []byte("applied")}), client.FieldOwner("test"), client.ForceOwnership)).To(Succeed())
			Expect(isStored(key)).To(BeFalse(), "an applied Secret must not be served as stored before the apply")
			secret, err := get(ctx, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("root", []byte("applied")))

			unstructuredApply := &unstructured.Unstructured{}
			unstructuredApply.SetGroupVersionKind(secretGVK)
			unstructuredApply.SetName(applied.Name)
			unstructuredApply.SetNamespace(applied.Namespace)
			Expect(unstructured.SetNestedField(unstructuredApply.Object, "dW5zdHJ1Y3R1cmVk", "data", "root")).To(Succeed())
			Expect(writer.Apply(ctx, client.ApplyConfigurationFromUnstructured(unstructuredApply), client.FieldOwner("test"), client.ForceOwnership)).To(Succeed())
			Expect(isStored(key)).To(BeFalse(), "a Secret applied unstructured must not be served as stored before the apply")
			secret, err = get(ctx, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("root", []byte("unstructured")))

			Expect(writer.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(namespace), client.MatchingLabels{"bulk": "true"})).To(Succeed())
			Expect(isStored(key)).To(BeFalse(), "a Secret deleted in bulk must not be served as stored")
		})

		It("drops Secrets written unstructured", SpecTimeout(time.Minute), func(ctx SpecContext) {
			written := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-unstructured", Namespace: namespace},
				Data:       map[string][]byte{"root": []byte("created")},
			}
			Expect(writer.Create(ctx, written)).To(Succeed())
			key := client.ObjectKeyFromObject(written)
			Expect(isStored(key)).To(BeTrue())

			content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(written)
			Expect(err).NotTo(HaveOccurred())
			updated := &unstructured.Unstructured{Object: content}
			updated.SetGroupVersionKind(secretGVK)
			Expect(unstructured.SetNestedField(updated.Object, "dXBkYXRlZA==", "data", "root")).To(Succeed())
			Expect(writer.Update(ctx, updated)).To(Succeed())
			Expect(isStored(key)).To(BeFalse(), "a Secret written unstructured must not be served as stored before the write")
			secret, err := get(ctx, key)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).To(HaveKeyWithValue("root", []byte("updated")))
		})

		It("does not store dry-run writes", SpecTimeout(time.Minute), func(ctx SpecContext) {
			dryRun := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cluster1-dry-run", Namespace: namespace}}
			Expect(writer.Create(ctx, dryRun, client.DryRunAll)).To(Succeed())
			_, err := get(ctx, client.ObjectKeyFromObject(dryRun))
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("lists Secrets from the API server", SpecTimeout(time.Minute), func(ctx SpecContext) {
			secrets := &corev1.SecretList{}
			Expect(c.List(ctx, secrets, client.InNamespace(namespace))).To(Succeed())
			Expect(secrets.Items).NotTo(BeEmpty())

			unstructuredSecrets := &unstructured.UnstructuredList{}
			unstructuredSecrets.SetGroupVersionKind(secretListGVK)
			Expect(c.List(ctx, unstructuredSecrets, client.InNamespace(namespace))).To(Succeed())
			Expect(unstructuredSecrets.Items).To(HaveLen(len(secrets.Items)))
		})

		It("lists Secret metadata by label", SpecTimeout(time.Minute), func(ctx SpecContext) {
			metadata := &metav1.PartialObjectMetadataList{}
			metadata.SetGroupVersionKind(secretListGVK)
			Expect(c.List(ctx, metadata, client.InNamespace(namespace), client.MatchingLabels{"owner": "helm"})).To(Succeed())
			Expect(metadata.Items).To(HaveLen(1))
			Expect(metadata.Items[0].Name).To(Equal(helmSecret.Name))
		})

		It("refuses a Secret informer with data", SpecTimeout(time.Minute), func(ctx SpecContext) {
			_, err := c.GetInformer(ctx, &corev1.Secret{})
			Expect(err).To(MatchError(errFullSecretInformer))
			unstructuredSecret := &unstructured.Unstructured{}
			unstructuredSecret.SetGroupVersionKind(secretGVK)
			_, err = c.GetInformer(ctx, unstructuredSecret)
			Expect(err).To(MatchError(errFullSecretInformer))
			Expect(c.Get(ctx, client.ObjectKeyFromObject(helmSecret), unstructuredSecret)).To(MatchError(errFullSecretInformer))
			Expect(c.IndexField(ctx, &corev1.Secret{}, "type", func(client.Object) []string { return nil })).
				To(MatchError(errFullSecretInformer))
			_, err = c.GetInformerForKind(ctx, secretGVK)
			Expect(err).To(MatchError(errFullSecretInformer))
		})
	})
})
