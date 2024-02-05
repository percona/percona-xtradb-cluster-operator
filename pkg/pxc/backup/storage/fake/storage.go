package fake

import (
	"context"
	"errors"
	"io"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

func NewFakeClient(ctx context.Context, opts storage.Options) (storage.Storage, error) {
	switch opts := opts.(type) {
	case *storage.S3Options:
		if opts.BucketName == "" {
			return nil, errors.New("bucket name is empty")
		}
	case *storage.AzureOptions:
		if opts.Container == "" {
			return nil, errors.New("container name is empty")
		}
	}
	return &FakeStorageClient{}, nil
}

type FakeStorageClient struct{}

func (c *FakeStorageClient) GetObject(ctx context.Context, objectName string) (io.ReadCloser, error) {
	return nil, nil
}

func (c *FakeStorageClient) PutObject(ctx context.Context, name string, data io.Reader, size int64) error {
	return nil
}

func (c *FakeStorageClient) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	return nil, nil
}
func (c *FakeStorageClient) DeleteObject(ctx context.Context, objectName string) error { return nil }
func (c *FakeStorageClient) SetPrefix(prefix string)                                   {}
func (c *FakeStorageClient) GetPrefix() string                                         { return "" }
