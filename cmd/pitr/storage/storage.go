package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

type Storage interface {
	GetObject(ctx context.Context, objectName string) (io.ReadCloser, error)
	PutObject(ctx context.Context, name string, data io.Reader, size int64) error
	ListObjects(ctx context.Context, prefix string) ([]string, error)
	SetPrefix(prefix string)
	GetPrefix() string
}

// S3 is a type for working with S3 storages
type S3 struct {
	client     *minio.Client // minio client for work with storage
	bucketName string        // S3 bucket name where binlogs will be stored
	prefix     string        // prefix for S3 requests
}

// NewS3 return new Manager, useSSL using ssl for connection with storage
func NewS3(endpoint, accessKeyID, secretAccessKey, bucketName, prefix, region string, verifyTLS bool) (*S3, error) {
	useSSL := strings.Contains(endpoint, "https")
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	transport := http.DefaultTransport
	transport.(*http.Transport).TLSClientConfig = &tls.Config{
		InsecureSkipVerify: !verifyTLS,
	}
	minioClient, err := minio.New(strings.TrimRight(endpoint, "/"), &minio.Options{
		Creds:     credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure:    useSSL,
		Region:    region,
		Transport: transport,
	})
	if err != nil {
		return nil, errors.Wrap(err, "new minio client")
	}

	return &S3{
		client:     minioClient,
		bucketName: bucketName,
		prefix:     prefix,
	}, nil
}

// GetObject return content by given object name
func (s *S3) GetObject(ctx context.Context, objectName string) (io.ReadCloser, error) {
	oldObj, err := s.client.GetObject(ctx, s.bucketName, s.prefix+objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "get object %s", s.prefix+objectName)
	}

	return oldObj, nil
}

// PutObject puts new object to storage with given name and content
func (s *S3) PutObject(ctx context.Context, name string, data io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucketName, s.prefix+name, data, size, minio.PutObjectOptions{})
	if err != nil {
		return errors.Wrapf(err, "put object %s", s.prefix+name)
	}

	return nil
}

func (s *S3) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	opts := minio.ListObjectsOptions{
		UseV1:  true,
		Prefix: s.prefix + prefix,
	}
	list := []string{}

	for object := range s.client.ListObjects(ctx, s.bucketName, opts) {
		if object.Err != nil {
			return nil, errors.Wrapf(object.Err, "list object %s", object.Key)
		}
		list = append(list, strings.TrimPrefix(object.Key, s.prefix))
	}

	return list, nil
}

func (s *S3) SetPrefix(prefix string) {
	s.prefix = prefix
}

func (s *S3) GetPrefix() string {
	return s.prefix
}

// Azure is a type for working with Azure Blob storages
type Azure struct {
	client    *azblob.Client // azure client for work with storage
	container string
	prefix    string
}

func NewAzure(storageAccount, accessKey, endpoint, container, prefix string) (*Azure, error) {
	credential, err := azblob.NewSharedKeyCredential(storageAccount, accessKey)
	if err != nil {
		return nil, errors.Wrap(err, "new credentials")
	}
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccount)
	}
	cli, err := azblob.NewClientWithSharedKeyCredential(endpoint, credential, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new client")
	}

	return &Azure{
		client:    cli,
		container: container,
		prefix:    prefix,
	}, nil
}

func (a *Azure) GetObject(ctx context.Context, name string) (io.ReadCloser, error) {
	resp, err := a.client.DownloadStream(ctx, a.container, url.QueryEscape(a.prefix+name), &azblob.DownloadStreamOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "download stream: %s", a.prefix+name)
	}
	return resp.Body, nil
}

func (a *Azure) PutObject(ctx context.Context, name string, data io.Reader, _ int64) error {
	_, err := a.client.UploadStream(ctx, a.container, url.QueryEscape(a.prefix+name), data, nil)
	if err != nil {
		return errors.Wrapf(err, "upload stream: %s", a.prefix+name)
	}
	return nil
}

func (a *Azure) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	listPrefix := a.prefix + prefix
	pg := a.client.NewListBlobsFlatPager(a.container, &container.ListBlobsFlatOptions{
		Prefix: &listPrefix,
	})
	var blobs []string
	for pg.More() {
		resp, err := pg.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "next page: %s", prefix)
		}
		if resp.Segment != nil {
			for _, item := range resp.Segment.BlobItems {
				if item != nil && item.Name != nil {
					name := strings.TrimPrefix(*item.Name, a.prefix)
					blobs = append(blobs, name)
				}
			}
		}
	}
	return blobs, nil
}

func (a *Azure) SetPrefix(prefix string) {
	a.prefix = prefix
}

func (a *Azure) GetPrefix() string {
	return a.prefix
}
