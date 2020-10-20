package storage

import (
	"context"
	"io"
	"io/ioutil"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

type Client interface {
	GetObject(objectName string) ([]byte, error)
	PutObject(name string, data io.Reader) error
}

// S3 is a type for working with S3 storages
type S3 struct {
	minioClient *minio.Client   // minio client for work with storage
	ctx         context.Context // context for client operations
	bucketName  string          // S3 bucket name where binlogs will be stored
}

// NewS3 return new Manager, useSSL using ssl for connection with storage
func NewS3(endpoint, accessKeyID, secretAccessKey, bucketName string, useSSL bool) (*S3, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "new minio client")
	}

	return &S3{
		minioClient: minioClient,
		ctx:         context.Background(),
		bucketName:  bucketName,
	}, nil
}

// GetObject return content by given object name
func (s *S3) GetObject(objectName string) ([]byte, error) {
	oldObj, err := s.minioClient.GetObject(s.ctx, s.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}

	return ioutil.ReadAll(oldObj)
}

type reader struct {
	r io.Reader
}

func (r *reader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

// PutObject puts new object to storage with given name and content
func (s *S3) PutObject(name string, data io.Reader) error {
	r := reader{data}

	_, err := s.minioClient.PutObject(s.ctx, s.bucketName, name, &r, -1, minio.PutObjectOptions{ContentType: "application/text"})
	if err != nil {
		return errors.Wrap(err, "put object")
	}

	return nil
}
