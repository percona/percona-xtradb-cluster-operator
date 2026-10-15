package k8s

import (
	"context"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// excludedSecretTypesCache explains cache misses caused by
// PXCO_SECRET_CACHE_EXCLUDED_TYPES. Next to the main cache it keeps one
// metadata-only cache per excluded type. When the main cache has no Secret
// for a key and the metadata cache of an excluded type has one, Get returns
// an error naming the setting instead of NotFound, without an API request.
type excludedSecretTypesCache struct {
	cache.Cache
	excluded map[corev1.SecretType]cache.Cache
}

// NewExcludedSecretTypesCache returns a NewCacheFunc for a manager whose
// Secret cache leaves out excludedTypes. See excludedSecretTypesCache.
func NewExcludedSecretTypesCache(excludedTypes []corev1.SecretType) cache.NewCacheFunc {
	return func(config *rest.Config, options cache.Options) (cache.Cache, error) {
		mainCache, err := cache.New(config, options)
		if err != nil {
			return nil, errors.Wrap(err, "create cache")
		}

		excluded := make(map[corev1.SecretType]cache.Cache, len(excludedTypes))
		for _, secretType := range excludedTypes {
			metadataCache, err := cache.New(config, cache.Options{
				HTTPClient:           options.HTTPClient,
				Scheme:               options.Scheme,
				Mapper:               options.Mapper,
				DefaultNamespaces:    options.DefaultNamespaces,
				DefaultFieldSelector: fields.OneTermEqualSelector("type", string(secretType)),
				DefaultTransform:     keepNameOnly,
			})
			if err != nil {
				return nil, errors.Wrapf(err, "create metadata cache for Secret type %q", secretType)
			}
			// Register the informer now so that WaitForCacheSync waits for it.
			if _, err := metadataCache.GetInformer(context.Background(), secretMetadata(), cache.BlockUntilSynced(false)); err != nil {
				return nil, errors.Wrapf(err, "create metadata informer for Secret type %q", secretType)
			}
			excluded[secretType] = metadataCache
		}
		return &excludedSecretTypesCache{Cache: mainCache, excluded: excluded}, nil
	}
}

// keepNameOnly drops the metadata the existence check does not need.
// Annotations can be large: kubectl's last-applied-configuration holds the
// whole Secret, data included.
func keepNameOnly(in any) (any, error) {
	if obj, err := meta.Accessor(in); err == nil {
		obj.SetAnnotations(nil)
		obj.SetLabels(nil)
		obj.SetManagedFields(nil)
		obj.SetOwnerReferences(nil)
		obj.SetFinalizers(nil)
	}
	return in, nil
}

func secretMetadata() *metav1.PartialObjectMetadata {
	secret := &metav1.PartialObjectMetadata{}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	return secret
}

func (c *excludedSecretTypesCache) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return c.Cache.Start(ctx) })
	for _, metadataCache := range c.excluded {
		g.Go(func() error { return metadataCache.Start(ctx) })
	}
	return errors.Wrap(g.Wait(), "run Secret caches")
}

func (c *excludedSecretTypesCache) WaitForCacheSync(ctx context.Context) bool {
	for _, metadataCache := range c.excluded {
		if !metadataCache.WaitForCacheSync(ctx) {
			return false
		}
	}
	return c.Cache.WaitForCacheSync(ctx)
}

func (c *excludedSecretTypesCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	err := c.Cache.Get(ctx, key, obj, opts...)
	if _, isSecret := obj.(*corev1.Secret); !isSecret || !k8serrors.IsNotFound(err) {
		return err
	}

	for secretType, metadataCache := range c.excluded {
		metadataErr := metadataCache.Get(ctx, key, secretMetadata())
		switch {
		case metadataErr == nil:
			return errors.Errorf("secret %s has type %q, which PXCO_SECRET_CACHE_EXCLUDED_TYPES keeps out of the operator's cache: remove the type from the list", key, secretType)
		case !k8serrors.IsNotFound(metadataErr):
			return errors.Wrapf(metadataErr, "check whether secret %s has type %q", key, secretType)
		}
	}
	return err
}
