package storage

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

type Storage interface {
	GetObject(objectName string) (io.Reader, error)
	PutObject(name string, data io.Reader, size int64) error
	ListObjects(prefix string) ([]string, error)
}

// S3 is a type for working with S3 storages
type S3 struct {
	minioClient *minio.Client   // minio client for work with storage
	ctx         context.Context // context for client operations
	bucketName  string          // S3 bucket name where binlogs will be stored
	prefix      string          // prefix for S3 requests
}

// NewS3 return new Manager, useSSL using ssl for connection with storage
func NewS3(endpoint, accessKeyID, secretAccessKey, bucketName, prefix, region string, useSSL, verifyTLS bool) (*S3, error) {
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
		minioClient: minioClient,
		ctx:         context.TODO(),
		bucketName:  bucketName,
		prefix:      prefix,
	}, nil
}

func (s *S3) SetPrefix(prefix string) {
	s.prefix = prefix
}

// GetObject return content by given object name
func (s *S3) GetObject(objectName string) (io.Reader, error) {
	oldObj, err := s.minioClient.GetObject(s.ctx, s.bucketName, s.prefix+objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}

	return oldObj, nil
}

// PutObject puts new object to storage with given name and content
func (s *S3) PutObject(name string, data io.Reader, size int64) error {
	_, err := s.minioClient.PutObject(s.ctx, s.bucketName, s.prefix+name, data, size, minio.PutObjectOptions{})
	if err != nil {
		return errors.Wrap(err, "put object")
	}

	return nil
}

func (s *S3) ListObjects(prefix string) ([]string, error) {
	opts := minio.ListObjectsOptions{
		UseV1:  true,
		Prefix: s.prefix + prefix,
	}
	list := []string{}

	for object := range s.minioClient.ListObjects(s.ctx, s.bucketName, opts) {
		if object.Err != nil {
			return nil, errors.Wrapf(object.Err, "list object %s", object.Key)
		}
		list = append(list, strings.TrimPrefix(object.Key, s.prefix))
	}

	return list, nil
}
