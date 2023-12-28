package fake

import (
	"context"
	"io"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

func NewFakeClient(storage.Options) (storage.Storage, error) {
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
