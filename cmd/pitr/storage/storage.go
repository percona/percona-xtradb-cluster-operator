package storage

import (
	"context"
	"io"
	"io/ioutil"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

// S3 is a type for working with S3 storages
type S3 struct {
	minioClient *minio.Client   // minio client for work with storage
	ctx         context.Context // context for client operations
	bucketName  string          // S3 bucket name where binlogs will be stored
}

// NewS3 return new Manager, useSSL using ssl for connection with storage
func NewS3(endpoint, accessKeyID, secretAccessKey, bucketName, region string, useSSL bool) (*S3, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV2(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, errors.Wrap(err, "new minio client")
	}

	return &S3{
		minioClient: minioClient,
		ctx:         context.TODO(),
		bucketName:  bucketName,
	}, nil
}

// GetObject return content by given object name
func (s *S3) GetObject(objectName string) ([]byte, error) {
	oldObj, err := s.minioClient.GetObject(s.ctx, s.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}
	out, err := ioutil.ReadAll(oldObj)
	if err != nil && minio.ToErrorResponse(err).Code != "NoSuchKey" {
		return nil, errors.Wrap(err, "read object")
	}
	//TODO: this method should return io.Reader
	return out, nil
}

// PutObject puts new object to storage with given name and content
func (s *S3) PutObject(name string, data io.Reader) error {
	_, err := s.minioClient.PutObject(s.ctx, s.bucketName, name, data, -1, minio.PutObjectOptions{})
	if err != nil {
		return errors.Wrap(err, "put object")
	}

	return nil
}

func (s *S3) ListObjects(prefix string) []string {
	opts := minio.ListObjectsOptions{
		UseV1:  true,
		Prefix: prefix,
	}
	list := []string{}

	for object := range s.minioClient.ListObjects(s.ctx, s.bucketName, opts) {
		list = append(list, object.Key)
	}

	return list
}
