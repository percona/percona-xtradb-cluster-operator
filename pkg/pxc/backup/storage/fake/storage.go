package fake

import (
	"context"
	"errors"
	"io"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
)

func NewStorage(_ context.Context, opts storage.Options) (storage.Storage, error) {
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
	return &Storage{}, nil
}

// Storage is a mock implementation of the storage.Storage interface
// used for testing purposes without performing real storage operations.
type Storage struct{}

func (c *Storage) GetObject(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (c *Storage) PutObject(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}

func (c *Storage) ListObjects(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (c *Storage) DeleteObject(_ context.Context, _ string) error {
	return nil
}

func (c *Storage) SetPrefix(_ string) {}

func (c *Storage) GetPrefix() string { return "" }
