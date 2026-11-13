package k8s

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/resourceversion"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// secretIdleTimeout is how long a fetched Secret stays stored without
	// being read. Reconciles read the Secrets they reference every few
	// seconds, so only Secrets nothing references any more go idle.
	secretIdleTimeout = 10 * time.Minute

	// writeVisibilityTimeout is how long a Secret the operator wrote is
	// served while the metadata informer has not shown the write yet. The
	// informer normally catches up in well under a second; past this the
	// cache stops trusting the write and goes by the informer again.
	writeVisibilityTimeout = 30 * time.Second

	// createVisibilityTimeout replaces writeVisibilityTimeout while the
	// informer shows no Secret, as before a create or after the operator
	// replaced a Secret. The cache cannot tell a Secret not seen yet from
	// one deleted while the watch was down, so such a write is trusted
	// briefly: past it, reads return NotFound until the informer shows the
	// Secret, as the stock cache does from the start.
	createVisibilityTimeout = 5 * time.Second

	// evictInterval is how often Secrets idle for secretIdleTimeout are
	// dropped, so that none stays stored much longer than that.
	evictInterval = time.Minute
)

var (
	// fetchTimeout bounds a shared fetch, which runs apart from the
	// contexts of the callers waiting for it.
	fetchTimeout = 30 * time.Second

	// fetchRetryDelays are the waits before retrying a fetch that failed
	// within fetchFailedFast to reach the API server, for a Secret with no
	// stored copy to serve instead. A slower failure, such as a timeout or
	// throttling client-go already retried, is not retried.
	fetchRetryDelays = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second}
	fetchFailedFast  = time.Second

	// outdatedFetchWait is how long a read waits for a fetch before serving
	// the stored copy of an earlier version, if there is one. The fetch goes
	// on and stores the Secret for later reads.
	outdatedFetchWait = 2 * time.Second
)

var errFullSecretInformer = errors.New("the cache holds Secret metadata only: watch Secrets with WatchesMetadata")

// secretCache keeps Secret data out of the informer cache. It watches the
// metadata of every Secret, which is what controllers watch Secrets with,
// and reads Secret data from the API server, keeping each Secret it reads
// until its resourceVersion changes, it is deleted or it goes unread for
// secretIdleTimeout. Memory then grows with the Secrets the operator reads
// instead of with every Secret in the cluster, such as Helm releases.
//
// A Secret missing from the metadata cache is NotFound without an API
// request, so optional Secrets that do not exist stay cheap to look up.
//
// When reading a changed Secret fails with a server error, throttling, a
// timeout or no answer from the API server, or takes longer than
// outdatedFetchWait, the Secret as last read is returned: older than the
// version the metadata cache shows, where the stock cache would return that
// version. It is returned with no age limit for as long as reading the
// change fails, so the change is acted on once a read succeeds. A Secret
// with no stored copy is read again a few times over
// about two seconds before the error is returned, unless the read failed
// slowly or timed out.
//
// Secret metadata read from the cache has no annotations, managed fields or
// finalizers.
//
// Secrets the operator writes through the client from NewSecretClient are
// stored as written, so reads see the operator's own writes before the
// informer does.
type secretCache struct {
	cache.Cache
	reader client.Reader
	scheme *runtime.Scheme
	// namespaces are the namespaces the cache is restricted to, or none
	// when it watches all of them.
	namespaces []string
	now        func() time.Time
	fetches    singleflight.Group

	mu      sync.Mutex
	entries map[client.ObjectKey]*secretEntry
	// dropped counts the times stored Secrets were dropped while they may
	// still exist. A fetch that started before a drop does not store what
	// it got where nothing is stored, or where a Secret with another UID is:
	// a newer version, or the Secret that replaced it, may have been read in
	// between, and nothing is left to keep the cache from going back.
	// The count is shared by all Secrets, so a drop also costs fetches of
	// other unstored Secrets their store; the operator rarely drops any.
	dropped uint64
}

type secretEntry struct {
	// metadataVersion is the resourceVersion the metadata cache shows when
	// secret is current. For a fetched Secret it is the one the metadata
	// cache had when the fetch started: the fetch can return a newer one,
	// and comparing with that would fetch again until the metadata caught
	// up. For a Secret the operator wrote it is the version the write made.
	metadataVersion string
	secret          *corev1.Secret
	lastRead        time.Time

	// write is set for a Secret the operator wrote until the metadata
	// cache shows the write.
	write *pendingWrite
}

// pendingWrite is what the metadata cache showed just before the operator
// wrote a Secret. While it still shows that, the informer is behind the
// write and the written Secret is the newest one.
type pendingWrite struct {
	baseFound   bool
	baseVersion string
	baseUID     types.UID
	at          time.Time
}

// informerBehind reports whether the metadata cache, showing metadata, has
// not reached the write of written yet.
func (w *pendingWrite) informerBehind(metadata *metav1.PartialObjectMetadata, found bool, written *corev1.Secret) bool {
	if !found {
		// Either the Secret is new, or the operator replaced it under the
		// same name and the informer shows the old one deleted.
		return !w.baseFound || w.baseUID != written.UID
	}
	// A fallback for resourceVersions that cannot be compared: otherwise
	// showsReplaced or showsEarlierWrite covers it.
	showsBase := w.baseFound && metadata.ResourceVersion == w.baseVersion
	showsReplaced := w.baseFound && w.baseUID != written.UID && metadata.UID == w.baseUID
	// An earlier version of the written Secret, such as one from another of
	// the operator's writes.
	showsEarlierWrite := metadata.UID == written.UID && olderVersion(metadata.ResourceVersion, written.ResourceVersion)
	return showsBase || showsReplaced || showsEarlierWrite
}

func visibilityTimeout(found bool) time.Duration {
	if !found {
		return createVisibilityTimeout
	}
	return writeVisibilityTimeout
}

// olderVersion reports whether resourceVersion a is older than b. It is
// false when either cannot be compared.
func olderVersion(a, b string) bool {
	cmp, err := resourceversion.CompareResourceVersion(a, b)
	return err == nil && cmp < 0
}

// newerSecret reports whether a is a newer version of Secret b.
func newerSecret(a, b *corev1.Secret) bool {
	return a.UID == b.UID && olderVersion(b.ResourceVersion, a.ResourceVersion)
}

// NewSecretCache returns a NewCacheFunc for a manager that caches Secret
// metadata only. See secretCache.
//
// Controllers must watch Secrets with WatchesMetadata or
// builder.OnlyMetadata. controller-runtime retries a refused Secret informer
// until the controller's CacheSyncTimeout, so a typed Owns or Watches of
// Secrets makes the manager fail to start after that timeout.
//
// Secrets cannot be selected by label or field: List reads them from the API
// server, which would not apply the selectors the informer does.
func NewSecretCache() cache.NewCacheFunc {
	return func(config *rest.Config, options cache.Options) (cache.Cache, error) {
		if options.Scheme == nil {
			options.Scheme = scheme.Scheme
		}
		if err := rejectSecretSelectors(options); err != nil {
			return nil, err
		}
		byObject, err := withSecretTransform(options.ByObject, options.DefaultTransform, options.Scheme)
		if err != nil {
			return nil, err
		}
		options.ByObject = byObject

		informerCache, err := cache.New(config, options)
		if err != nil {
			return nil, errors.Wrap(err, "create cache")
		}
		reader, err := client.New(config, client.Options{
			HTTPClient: options.HTTPClient,
			Scheme:     options.Scheme,
			Mapper:     options.Mapper,
		})
		if err != nil {
			return nil, errors.Wrap(err, "create Secret reader")
		}

		c := &secretCache{
			Cache:   informerCache,
			reader:  reader,
			scheme:  options.Scheme,
			now:     time.Now,
			entries: make(map[client.ObjectKey]*secretEntry),
		}
		if c.namespaces, err = secretNamespaces(options); err != nil {
			return nil, err
		}
		// Register the informer now so that WaitForCacheSync waits for it.
		informer, err := informerCache.GetInformer(context.Background(), secretMetadata(), cache.BlockUntilSynced(false))
		if err != nil {
			return nil, errors.Wrap(err, "create Secret metadata informer")
		}
		if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{DeleteFunc: c.forget}); err != nil {
			return nil, errors.Wrap(err, "handle Secret deletions")
		}
		return c, nil
	}
}

// withSecretTransform returns a copy of byObject in which Secrets are
// transformed with dropBulkyMetadata, after the transform set for them or,
// without one, defaultTransform. A transform set in DefaultNamespaces
// takes precedence over it and skips dropBulkyMetadata.
func withSecretTransform(byObject map[client.Object]cache.ByObject, defaultTransform toolscache.TransformFunc, s *runtime.Scheme) (map[client.Object]cache.ByObject, error) {
	byObject = maps.Clone(byObject)
	if byObject == nil {
		byObject = make(map[client.Object]cache.ByObject, 1)
	}
	key, err := secretKey(byObject, s)
	if err != nil {
		return nil, err
	}
	options := byObject[key]
	transform := options.Transform
	if transform == nil {
		transform = defaultTransform
	}
	options.Transform = func(in any) (any, error) {
		if transform != nil {
			var err error
			if in, err = transform(in); err != nil {
				return nil, err
			}
		}
		return dropBulkyMetadata(in)
	}
	byObject[key] = options
	return byObject, nil
}

// secretNamespaces returns the namespaces the Secret informer is restricted
// to, or none when it watches all of them. A Secret entry in ByObject with
// Namespaces set takes precedence over DefaultNamespaces.
func secretNamespaces(options cache.Options) ([]string, error) {
	key, err := secretKey(options.ByObject, options.Scheme)
	if err != nil {
		return nil, err
	}
	namespaces := options.DefaultNamespaces
	if byObject := options.ByObject[key].Namespaces; byObject != nil {
		namespaces = byObject
	}
	if _, all := namespaces[metav1.NamespaceAll]; all {
		return nil, nil
	}
	return slices.Sorted(maps.Keys(namespaces)), nil
}

// rejectSecretSelectors returns an error if options select Secrets by label
// or field.
func rejectSecretSelectors(options cache.Options) error {
	key, err := secretKey(options.ByObject, options.Scheme)
	if err != nil {
		return err
	}
	byObject := options.ByObject[key]
	configs := []cache.Config{
		{LabelSelector: options.DefaultLabelSelector, FieldSelector: options.DefaultFieldSelector},
		{LabelSelector: byObject.Label, FieldSelector: byObject.Field},
	}
	configs = slices.AppendSeq(configs, maps.Values(options.DefaultNamespaces))
	configs = slices.AppendSeq(configs, maps.Values(byObject.Namespaces))
	for _, config := range configs {
		if (config.LabelSelector != nil && !config.LabelSelector.Empty()) || (config.FieldSelector != nil && !config.FieldSelector.Empty()) {
			return errors.New("Secrets cannot be selected by label or field: they are listed from the API server")
		}
	}
	return nil
}

// secretKey returns the key of the Secret entry in byObject, or a new key if
// there is none. Keys are pointers, so the entry is found by its kind.
func secretKey(byObject map[client.Object]cache.ByObject, s *runtime.Scheme) (client.Object, error) {
	for obj := range byObject {
		gvk, err := apiutil.GVKForObject(obj, s)
		if err != nil {
			return nil, errors.Wrapf(err, "get kind of %T", obj)
		}
		if gvk == secretGVK {
			return obj, nil
		}
	}
	return &corev1.Secret{}, nil
}

// dropBulkyMetadata drops the metadata that can be large and that neither
// the cache nor event handlers need. kubectl's last-applied-configuration
// annotation holds the whole Secret, data included.
func dropBulkyMetadata(in any) (any, error) {
	if obj, err := meta.Accessor(in); err == nil {
		obj.SetAnnotations(nil)
		obj.SetManagedFields(nil)
		obj.SetFinalizers(nil)
	}
	return in, nil
}

var (
	secretGVK     = corev1.SchemeGroupVersion.WithKind("Secret")
	secretListGVK = corev1.SchemeGroupVersion.WithKind("SecretList")
)

func secretMetadata() *metav1.PartialObjectMetadata {
	secret := &metav1.PartialObjectMetadata{}
	secret.SetGroupVersionKind(secretGVK)
	return secret
}

// IsSecretCache reports whether c was built by NewSecretCache.
func IsSecretCache(c cache.Cache) bool {
	_, ok := c.(*secretCache)
	return ok
}

func (c *secretCache) Start(ctx context.Context) error {
	go wait.UntilWithContext(ctx, func(context.Context) { c.evictIdle() }, evictInterval)
	return c.Cache.Start(ctx)
}

func (c *secretCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	secret, isSecret := obj.(*corev1.Secret)
	if !isSecret {
		if c.holdsSecretData(obj) {
			return errFullSecretInformer
		}
		return c.Cache.Get(ctx, key, obj, opts...)
	}

	metadata := secretMetadata()
	err := c.Cache.Get(ctx, key, metadata)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	found := err == nil
	if stored := c.stored(key, metadata, found); stored != nil {
		stored.DeepCopyInto(secret)
		return nil
	}
	if !found {
		return err
	}

	fetchCtx := ctx
	if c.outdated(key, metadata) != nil {
		var cancel context.CancelFunc
		fetchCtx, cancel = context.WithTimeout(ctx, outdatedFetchWait)
		defer cancel()
	}
	fetched, took, err := c.timedFetch(fetchCtx, key, metadata.ResourceVersion)
	// The caller's own context ending is not a failure to reach the API
	// server.
	if err != nil && transientError(err) && ctx.Err() == nil {
		if outdated := c.outdated(key, metadata); outdated != nil {
			logf.FromContext(ctx).V(1).Info("Serving the Secret as last read", "secret", key, "error", err.Error())
			outdated.DeepCopyInto(secret)
			return nil
		}
		if failedFast(err, took) {
			fetched, err = c.retryFetch(ctx, key, metadata.ResourceVersion, err)
		}
		if err != nil && transientError(err) && ctx.Err() == nil {
			return unavailableError{err}
		}
	}
	if err != nil {
		return err
	}
	fetched.DeepCopyInto(secret)
	return nil
}

// unavailableError is a failure to read a Secret from the API server.
type unavailableError struct{ error }

func (e unavailableError) Unwrap() error { return e.error }

func (unavailableError) SecretUnavailable() bool { return true }

// IsSecretUnavailable reports whether err is a failure of the Secret
// metadata cache to read a Secret from the API server, which a later read
// can get past: an error with a SecretUnavailable method returning true.
// The stock cache never fails this way. When the caller's own context ends
// first, Get returns the context's error instead, which the caller must
// handle itself.
func IsSecretUnavailable(err error) bool {
	var unavailable interface{ SecretUnavailable() bool }
	return errors.As(err, &unavailable) && unavailable.SecretUnavailable()
}

// retryFetch retries a fetch that failed with err to reach the API server.
func (c *secretCache) retryFetch(ctx context.Context, key client.ObjectKey, metadataVersion string, err error) (*corev1.Secret, error) {
	for _, delay := range fetchRetryDelays {
		select {
		case <-ctx.Done():
			return nil, errors.Wrapf(ctx.Err(), "retry after %v", err)
		case <-time.After(delay):
		}
		fetched, took, fetchErr := c.timedFetch(ctx, key, metadataVersion)
		if err = fetchErr; err == nil || !transientError(err) || !failedFast(err, took) {
			return fetched, err
		}
	}
	return nil, err
}

// failedFast reports whether a fetch that failed with err after took is
// worth retrying.
func failedFast(err error, took time.Duration) bool {
	return took < fetchFailedFast && !errors.Is(err, context.DeadlineExceeded)
}

// transientError reports whether err is a failure to reach the API server
// rather than an answer about the Secret: anything but an API status other
// than throttling or a server error. Client-side errors without a status,
// such as decoding or TLS failures, count too: telling them apart from the
// network errors client-go wraps is not reliable, and a caller that retries
// them gives up after its own timeout.
func transientError(err error) bool {
	var status k8serrors.APIStatus
	if !errors.As(err, &status) {
		return true
	}
	code := status.Status().Code
	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

// stored returns the stored copy of the Secret if it is current, given what
// the metadata cache shows for it.
func (c *secretCache) stored(key client.ObjectKey, metadata *metav1.PartialObjectMetadata, found bool) *corev1.Secret {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil
	}
	switch {
	case found && entry.metadataVersion == metadata.ResourceVersion:
		entry.write = nil
	case entry.write != nil && entry.write.informerBehind(metadata, found, entry.secret) &&
		c.now().Sub(entry.write.at) < visibilityTimeout(found):
		// The informer has not shown the operator's write yet.
	case found && entry.secret.UID == metadata.UID:
		// Outdated, but kept to serve if reading the new version fails.
		entry.write = nil
		return nil
	default:
		delete(c.entries, key)
		return nil
	}
	entry.lastRead = c.now()
	return entry.secret
}

// outdated returns the stored copy of the Secret the metadata cache shows,
// whatever its version.
func (c *secretCache) outdated(key client.ObjectKey, metadata *metav1.PartialObjectMetadata) *corev1.Secret {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok && entry.secret.UID == metadata.UID {
		entry.lastRead = c.now()
		return entry.secret
	}
	return nil
}

// timedFetch reads the Secret from the API server and stores it, and
// returns how long the request took, which can be longer than the caller
// waited for it. Concurrent fetches of the same version share one request,
// which a caller giving up does not cancel for the others.
func (c *secretCache) timedFetch(ctx context.Context, key client.ObjectKey, metadataVersion string) (*corev1.Secret, time.Duration, error) {
	c.mu.Lock()
	dropped := c.dropped
	c.mu.Unlock()
	// A read that starts after a drop does not share an earlier request,
	// which can return the Secret as it was before the operator's write.
	fetches := c.fetches.DoChan(fmt.Sprintf("%s@%s#%d", key, metadataVersion, dropped), func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()
		start := time.Now()
		secret := &corev1.Secret{}
		if err := c.reader.Get(fetchCtx, key, secret); err != nil {
			return fetchResult{took: time.Since(start)}, errors.Wrapf(err, "get Secret %s", key)
		}
		// Like the informer cache, return Secrets with their kind set.
		secret.SetGroupVersionKind(secretGVK)
		c.mu.Lock()
		defer c.mu.Unlock()
		// Keep a write the operator made while the fetch was in flight, and
		// a newer Secret another fetch stored first, unless the fetch got a
		// newer one. Of two fetches that got the same version, keep the one
		// tagged with the newer metadata.
		entry, ok := c.entries[key]
		switch {
		case c.dropped != dropped && (!ok || entry.secret.UID != secret.UID):
		case ok && entry.write != nil && newerSecret(secret, entry.secret):
			// A newer version of the Secret the operator wrote: the informer is
			// still compared with what it showed before the write.
			c.entries[key] = &secretEntry{metadataVersion: secret.ResourceVersion, secret: secret, lastRead: c.now(), write: entry.write}
		case !ok || newerSecret(secret, entry.secret) ||
			entry.write == nil && !newerSecret(entry.secret, secret) && !olderVersion(metadataVersion, entry.metadataVersion):
			c.entries[key] = &secretEntry{metadataVersion: metadataVersion, secret: secret, lastRead: c.now()}
		}
		return fetchResult{secret: secret, took: time.Since(start)}, nil
	})
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case fetched := <-fetches:
		result := fetched.Val.(fetchResult)
		return result.secret, result.took, fetched.Err
	}
}

type fetchResult struct {
	secret *corev1.Secret
	took   time.Duration
}

// List reads Secrets from the API server: the cache has no Secret data.
func (c *secretCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.holdsSecretData(list) {
		if err := c.listSecrets(ctx, list, opts...); err != nil {
			return err
		}
		// The API server leaves the kind out of typed items, which the
		// stock cache sets.
		return meta.EachListItem(list, func(obj runtime.Object) error {
			if secret, ok := obj.(*corev1.Secret); ok {
				secret.SetGroupVersionKind(secretGVK)
			}
			return nil
		})
	}
	return c.Cache.List(ctx, list, opts...)
}

// listSecrets lists Secrets from the API server in the namespaces the cache
// is restricted to, as the cache would: a list without a namespace covers
// all of them, and a list in another namespace fails. Unlike the stock
// cache, a list without a namespace across several namespaces cannot be
// limited or continued.
func (c *secretCache) listSecrets(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if len(c.namespaces) == 0 {
		return errors.Wrap(c.reader.List(ctx, list, opts...), "list Secrets")
	}
	listOpts := (&client.ListOptions{}).ApplyOptions(opts)
	if listOpts.Namespace != "" {
		if !slices.Contains(c.namespaces, listOpts.Namespace) {
			return errors.Errorf("list Secrets in namespace %s: the cache is restricted to namespaces %v", listOpts.Namespace, c.namespaces)
		}
		return errors.Wrapf(c.reader.List(ctx, list, opts...), "list Secrets in namespace %s", listOpts.Namespace)
	}
	if listOpts.Limit > 0 || listOpts.Continue != "" {
		return errors.New("list Secrets across namespaces: pagination is not supported")
	}

	var items []runtime.Object
	for _, namespace := range c.namespaces {
		namespaceList, ok := list.DeepCopyObject().(client.ObjectList)
		if !ok {
			return errors.Errorf("copy %T", list)
		}
		if err := c.reader.List(ctx, namespaceList, append(slices.Clone(opts), client.InNamespace(namespace))...); err != nil {
			return errors.Wrapf(err, "list Secrets in namespace %s", namespace)
		}
		namespaceItems, err := meta.ExtractList(namespaceList)
		if err != nil {
			return errors.Wrap(err, "extract Secrets")
		}
		items = append(items, namespaceItems...)
	}
	// The lists have their own resourceVersions, none of which is the
	// merged list's.
	list.SetResourceVersion("")
	list.SetContinue("")
	list.SetRemainingItemCount(nil)
	return meta.SetList(list, items)
}

// GetInformer refuses a Secret informer: it would cache every Secret with
// its data, which is what secretCache exists to prevent.
func (c *secretCache) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if c.holdsSecretData(obj) {
		return nil, errFullSecretInformer
	}
	return c.Cache.GetInformer(ctx, obj, opts...)
}

// IndexField refuses to index Secrets: the index would need a Secret
// informer with data.
func (c *secretCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	if c.holdsSecretData(obj) {
		return errFullSecretInformer
	}
	return c.Cache.IndexField(ctx, obj, field, extractValue)
}

// holdsSecretData reports whether obj is a Secret or Secret list with data,
// typed or unstructured, as opposed to Secret metadata.
func (c *secretCache) holdsSecretData(obj runtime.Object) bool {
	switch obj.(type) {
	case *metav1.PartialObjectMetadata, *metav1.PartialObjectMetadataList:
		return false
	}
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	return err == nil && (gvk == secretGVK || gvk == secretListGVK)
}

func (c *secretCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	if gvk == secretGVK {
		return nil, errFullSecretInformer
	}
	return c.Cache.GetInformerForKind(ctx, gvk, opts...)
}

// wrote stores a Secret the operator has written, as the API server
// returned it. metadata and found are what the metadata cache showed just
// before the write.
//
// A Delete of the Secret that finishes while the write is in flight is not
// seen here: the written Secret stays stored until the informer shows the
// deletion, or for at most visibilityTimeout.
func (c *secretCache) wrote(secret *corev1.Secret, metadata *metav1.PartialObjectMetadata, found bool) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	key := client.ObjectKeyFromObject(secret)
	// Another write to the Secret that finished first stored a newer version.
	if entry, ok := c.entries[key]; ok && newerSecret(entry.secret, secret) {
		return
	}
	stored := secret.DeepCopy()
	stored.SetGroupVersionKind(secretGVK)
	c.entries[key] = &secretEntry{
		metadataVersion: secret.ResourceVersion,
		secret:          stored,
		lastRead:        now,
		write: &pendingWrite{
			baseFound:   found,
			baseVersion: metadata.ResourceVersion,
			baseUID:     metadata.UID,
			at:          now,
		},
	}
}

func (c *secretCache) forget(obj any) {
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	key := client.ObjectKey{Namespace: accessor.GetNamespace(), Name: accessor.GetName()}
	c.mu.Lock()
	defer c.mu.Unlock()
	// A Secret recreated under the same name is a different Secret.
	if entry, ok := c.entries[key]; ok && entry.secret.UID != accessor.GetUID() {
		return
	}
	delete(c.entries, key)
}

func (c *secretCache) forgetKey(key client.ObjectKey) {
	c.mu.Lock()
	delete(c.entries, key)
	c.dropped++
	c.mu.Unlock()
}

// forgetNamespace drops the Secrets stored for namespace, or for all
// namespaces if it is empty.
func (c *secretCache) forgetNamespace(namespace string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.DeleteFunc(c.entries, func(key client.ObjectKey, _ *secretEntry) bool {
		return namespace == "" || key.Namespace == namespace
	})
	c.dropped++
}

func (c *secretCache) evictIdle() {
	idleSince := c.now().Add(-secretIdleTimeout)
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.DeleteFunc(c.entries, func(_ client.ObjectKey, entry *secretEntry) bool {
		return entry.lastRead.Before(idleSince)
	})
}

// NewSecretClient is a NewClientFunc for a manager whose cache comes from
// NewSecretCache. Secrets written through the client are stored in the
// cache as written, so that the operator reads its own writes.
func NewSecretClient(config *rest.Config, options client.Options) (client.Client, error) {
	cl, err := client.New(config, options)
	if err != nil {
		return nil, errors.Wrap(err, "create client")
	}
	if options.Cache == nil {
		return cl, nil
	}
	secrets, ok := options.Cache.Reader.(*secretCache)
	if !ok {
		return cl, nil
	}
	return &secretWritingClient{Client: cl, secrets: secrets}, nil
}

type secretWritingClient struct {
	client.Client
	secrets *secretCache
}

func (c *secretWritingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	dryRun := len((&client.CreateOptions{}).ApplyOptions(opts).DryRun) > 0
	return c.write(ctx, obj, dryRun, func() error { return c.Client.Create(ctx, obj, opts...) })
}

func (c *secretWritingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	dryRun := len((&client.UpdateOptions{}).ApplyOptions(opts).DryRun) > 0
	return c.write(ctx, obj, dryRun, func() error { return c.Client.Update(ctx, obj, opts...) })
}

func (c *secretWritingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	dryRun := len((&client.PatchOptions{}).ApplyOptions(opts).DryRun) > 0
	return c.write(ctx, obj, dryRun, func() error { return c.Client.Patch(ctx, obj, patch, opts...) })
}

// Delete drops the stored Secret even when the request fails: the Secret
// may be gone, and dropping it only costs a read. A read already in flight
// can still return and store the Secret until the informer shows the
// deletion, as the stock cache would return it until the informer does.
func (c *secretWritingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := c.Client.Delete(ctx, obj, opts...)
	dryRun := len((&client.DeleteOptions{}).ApplyOptions(opts).DryRun) > 0
	if c.isSecret(obj) && !dryRun {
		c.secrets.forgetKey(client.ObjectKeyFromObject(obj))
	}
	return err
}

// Apply drops the stored Secret: the apply configuration is not the
// Secret the API server stored.
func (c *secretWritingClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	if err := c.Client.Apply(ctx, obj, opts...); err != nil {
		return err
	}
	if key, isSecret := appliedSecret(obj); isSecret {
		c.secrets.forgetKey(key)
	}
	return nil
}

// appliedSecret returns the key of the Secret an apply configuration
// writes, typed or made with client.ApplyConfigurationFromUnstructured.
func appliedSecret(obj runtime.ApplyConfiguration) (client.ObjectKey, bool) {
	switch ac := obj.(type) {
	case interface {
		GetAPIVersion() *string
		GetKind() *string
		GetNamespace() *string
		GetName() *string
	}:
		gvk := schema.FromAPIVersionAndKind(ptr.Deref(ac.GetAPIVersion(), ""), ptr.Deref(ac.GetKind(), ""))
		return client.ObjectKey{Namespace: ptr.Deref(ac.GetNamespace(), ""), Name: ptr.Deref(ac.GetName(), "")}, gvk == secretGVK
	case interface {
		GetObjectKind() schema.ObjectKind
		GetNamespace() string
		GetName() string
	}:
		return client.ObjectKey{Namespace: ac.GetNamespace(), Name: ac.GetName()}, ac.GetObjectKind().GroupVersionKind() == secretGVK
	}
	return client.ObjectKey{}, false
}

func (c *secretWritingClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if err := c.Client.DeleteAllOf(ctx, obj, opts...); err != nil {
		return err
	}
	if c.isSecret(obj) {
		c.secrets.forgetNamespace((&client.DeleteAllOfOptions{}).ApplyOptions(opts).Namespace)
	}
	return nil
}

// isSecret reports whether obj is a Secret, typed, unstructured or as
// metadata.
func (c *secretWritingClient) isSecret(obj client.Object) bool {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	return err == nil && gvk == secretGVK
}

func (c *secretWritingClient) write(ctx context.Context, obj client.Object, dryRun bool, write func() error) error {
	secret, isSecret := obj.(*corev1.Secret)
	if !isSecret || dryRun {
		if !dryRun && c.isSecret(obj) {
			// Only a typed Secret can be stored as written.
			defer c.secrets.forgetKey(client.ObjectKeyFromObject(obj))
		}
		return write()
	}
	// Read the metadata before writing: read after, it could already show
	// a newer write by someone else, which the stored Secret would hide.
	metadata := secretMetadata()
	err := c.secrets.Cache.Get(ctx, client.ObjectKeyFromObject(secret), metadata)
	if err != nil && !k8serrors.IsNotFound(err) {
		// Without the metadata from before the write, the write cannot be
		// told apart from later ones: leave the Secret to the informer.
		defer c.secrets.forgetKey(client.ObjectKeyFromObject(secret))
		return write()
	}
	found := err == nil
	if err := write(); err != nil {
		return err
	}
	c.secrets.wrote(secret, metadata, found)
	return nil
}
