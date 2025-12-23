package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

var ErrObjectNotFound = errors.New("object not found")

type Storage interface {
	GetObject(ctx context.Context, objectName string) (io.ReadCloser, error)
	PutObject(ctx context.Context, name string, data io.Reader, size int64) error
	ListObjects(ctx context.Context, prefix string) ([]string, error)
	DeleteObject(ctx context.Context, objectName string) error
	SetPrefix(prefix string)
	GetPrefix() string
}

type NewClientFunc func(context.Context, Options) (Storage, error)

func NewClient(ctx context.Context, opts Options) (Storage, error) {
	switch opts.Type() {
	case api.BackupStorageS3:
		opts, ok := opts.(*S3Options)
		if !ok {
			return nil, errors.New("invalid options type")
		}
		return NewS3(ctx, opts.Endpoint, opts.AccessKeyID, opts.SecretAccessKey, opts.SessionToken, opts.BucketName, opts.Prefix, opts.Region, opts.VerifyTLS, opts.CABundle)
	case api.BackupStorageAzure:
		opts, ok := opts.(*AzureOptions)
		if !ok {
			return nil, errors.New("invalid options type")
		}
		return NewAzure(opts.StorageAccount, opts.AccessKey, opts.Endpoint, opts.Container, opts.Prefix, opts.BlockSize, opts.Concurrency)
	}
	return nil, errors.New("invalid storage type")
}

// S3 is a type for working with S3 storages
type S3 struct {
	client     *minio.Client // minio client for work with storage
	bucketName string        // S3 bucket name where binlogs will be stored
	prefix     string        // prefix for S3 requests
}

// NewS3 return new Manager, useSSL using ssl for connection with storage
func NewS3(
	ctx context.Context,
	endpoint,
	accessKeyID,
	secretAccessKey,
	sessionToken,
	bucketName,
	prefix,
	region string,
	verifyTLS bool,
	caBundle []byte,
) (Storage, error) {
	if endpoint == "" {
		endpoint = "https://s3.amazonaws.com"
		// We can't use default endpoint if region is not us-east-1
		// More info: https://docs.aws.amazon.com/general/latest/gr/s3.html
		if region != "" && region != "us-east-1" {
			endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", region)
		}
	}
	useSSL := strings.Contains(endpoint, "https")
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	transport := http.DefaultTransport
	transport.(*http.Transport).TLSClientConfig = &tls.Config{
		InsecureSkipVerify: !verifyTLS,
	}
	// if caBundle is provided, we use it for the TLS client config
	if len(caBundle) > 0 {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, errors.Wrap(err, "get system cert pool")
		}
		if ok := roots.AppendCertsFromPEM(caBundle); !ok {
			return nil, errors.New("failed to append certs from PEM")
		}
		transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	}
	minioClient, err := minio.New(strings.TrimRight(endpoint, "/"), &minio.Options{
		Creds:     credentials.NewStaticV4(accessKeyID, secretAccessKey, sessionToken),
		Secure:    useSSL,
		Region:    region,
		Transport: transport,
	})
	if err != nil {
		return nil, errors.Wrap(err, "new minio client")
	}

	bucketExists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		if merr, ok := err.(minio.ErrorResponse); ok && merr.Code == "301 Moved Permanently" {
			return nil, errors.Errorf("%s region: %s bucket: %s", merr.Code, merr.Region, merr.BucketName)
		}
		return nil, errors.Wrap(err, "failed to check if bucket exists")
	}
	if !bucketExists {
		return nil, errors.Errorf("bucket %s does not exist", bucketName)
	}

	return &S3{
		client:     minioClient,
		bucketName: bucketName,
		prefix:     prefix,
	}, nil
}

// GetObject return content by given object name
func (s *S3) GetObject(ctx context.Context, objectName string) (io.ReadCloser, error) {
	objPath := path.Join(s.prefix, objectName)
	oldObj, err := s.client.GetObject(ctx, s.bucketName, objPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "get object %s", objPath)
	}

	// minio client returns error only on Read() method, so we need to call it to see if object exists
	_, err = oldObj.Read([]byte{})
	if err != nil && err != io.EOF {
		if minio.ToErrorResponse(errors.Cause(err)).Code == "NoSuchKey" {
			return nil, ErrObjectNotFound
		}
		return nil, errors.Wrapf(err, "read object %s", objPath)
	}

	_, err = oldObj.Seek(0, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "seek object %s", objPath)
	}

	return oldObj, nil
}

// PutObject puts new object to storage with given name and content
func (s *S3) PutObject(ctx context.Context, name string, data io.Reader, size int64) error {
	objPath := path.Join(s.prefix, name)
	_, err := s.client.PutObject(ctx, s.bucketName, objPath, data, size, minio.PutObjectOptions{})
	if err != nil {
		return errors.Wrapf(err, "put object %s", objPath)
	}

	return nil
}

func (s *S3) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	opts := minio.ListObjectsOptions{
		UseV1:     true,
		Recursive: true,
		Prefix:    s.prefix + prefix,
	}
	list := []string{}

	var err error
	for object := range s.client.ListObjects(ctx, s.bucketName, opts) {
		// From `(c *Client) ListObjects` method docs:
		//  `caller must drain the channel entirely and wait until channel is closed before proceeding,
		//   without waiting on the channel to be closed completely you might leak goroutines`
		// So we should save the error and drain the channel.
		if err != nil {
			continue
		}
		if object.Err != nil {
			err = errors.Wrapf(object.Err, "list object %s", object.Key)
		}
		list = append(list, strings.TrimPrefix(object.Key, s.prefix))
	}
	if err != nil {
		return nil, err
	}

	return list, nil
}

func (s *S3) SetPrefix(prefix string) {
	s.prefix = prefix
}

func (s *S3) GetPrefix() string {
	return s.prefix
}

func (s *S3) DeleteObject(ctx context.Context, objectName string) error {
	log := logf.FromContext(ctx).WithValues("bucket", s.bucketName, "prefix", s.prefix)

	// minio sdk automatically URL-encodes the path
	p := path.Join(s.prefix, objectName)
	objPath, err := url.QueryUnescape(p)
	if err != nil {
		return errors.Wrapf(err, "failed to unescape object path %s", p)
	}

	log.V(1).Info("deleting object", "object", objPath)

	err = s.client.RemoveObject(ctx, s.bucketName, objPath, minio.RemoveObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(errors.Cause(err)).Code == "NoSuchKey" {
			return ErrObjectNotFound
		}
		return errors.Wrapf(err, "failed to remove object %s", objectName)
	}

	log.V(1).Info("object deleted", "object", objPath)

	return nil
}

// Azure is a type for working with Azure Blob storages
type Azure struct {
	client      *azblob.Client // azure client for work with storage
	container   string
	prefix      string
	blockSize   int64
	concurrency int
}

func NewAzure(storageAccount, accessKey, endpoint, container, prefix string, blockSize int64, concurrency int) (Storage, error) {
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
		client:      cli,
		container:   container,
		prefix:      prefix,
		blockSize:   blockSize,
		concurrency: concurrency,
	}, nil
}

func (a *Azure) GetObject(ctx context.Context, name string) (io.ReadCloser, error) {
	objPath := path.Join(a.prefix, name)
	resp, err := a.client.DownloadStream(ctx, a.container, objPath, &azblob.DownloadStreamOptions{})
	if err != nil {
		if bloberror.HasCode(errors.Cause(err), bloberror.BlobNotFound) {
			return nil, ErrObjectNotFound
		}
		return nil, errors.Wrapf(err, "download stream: %s", objPath)
	}
	return resp.Body, nil
}

func (a *Azure) PutObject(ctx context.Context, name string, data io.Reader, _ int64) error {
	objPath := path.Join(a.prefix, name)
	uploadOptions := azblob.UploadStreamOptions{
		BlockSize:   a.blockSize,
		Concurrency: a.concurrency,
	}
	_, err := a.client.UploadStream(ctx, a.container, objPath, data, &uploadOptions)
	if err != nil {
		return errors.Wrapf(err, "upload stream: %s", objPath)
	}
	return nil
}

func (a *Azure) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	listPrefix := path.Join(a.prefix, prefix)
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

func (a *Azure) DeleteObject(ctx context.Context, objectName string) error {
	log := logf.FromContext(ctx).WithValues("container", a.container, "prefix", a.prefix, "object", objectName)

	objPath := path.Join(a.prefix, objectName)
	log.V(1).Info("deleting object", "object", objPath)

	_, err := a.client.DeleteBlob(ctx, a.container, objPath, nil)
	if err != nil {
		if bloberror.HasCode(errors.Cause(err), bloberror.BlobNotFound) {
			return ErrObjectNotFound
		}
		return errors.Wrapf(err, "delete blob %s", objPath)
	}

	log.V(1).Info("object deleted", "object", objPath)

	return nil
}
