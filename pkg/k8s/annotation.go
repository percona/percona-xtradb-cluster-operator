package k8s

import (
	"context"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AnnotateObject adds the specified annotations to the object
func AnnotateObject(ctx context.Context, c client.Client, obj client.Object, annotations map[string]string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_obj := obj.DeepCopyObject().(client.Object)
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), _obj)
		if err != nil {
			return err
		}

		a := _obj.GetAnnotations()
		if a == nil {
			a = make(map[string]string)
		}

		for k, v := range annotations {
			a[k] = v
		}
		_obj.SetAnnotations(a)

		return c.Patch(ctx, _obj, client.MergeFrom(obj))
	})
}

// DeannotateObject removes the specified annotation from the object
func DeannotateObject(ctx context.Context, c client.Client, obj client.Object, annotation string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_obj := obj.DeepCopyObject().(client.Object)
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), _obj)
		if err != nil {
			return err
		}

		a := _obj.GetAnnotations()
		if a == nil {
			a = make(map[string]string)
		}

		delete(a, annotation)
		_obj.SetAnnotations(a)

		return c.Patch(ctx, _obj, client.MergeFrom(obj))
	})
}
