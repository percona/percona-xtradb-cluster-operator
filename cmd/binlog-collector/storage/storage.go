package storage

import (
	"bytes"
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

// Manager is a type for working with S3 storages
type Manager struct {
	minioClient       *minio.Client   // minio client for work with storage
	ctx               context.Context // context for client operations
	useSSL            bool            // using ssl for connection with S3 storage
	bucketName        string          // S3 bucket name where binlogs will be stored
	LastSetObjectName string          // name for object where the last binlog set will stored
}

// NewManager return new Manager
func NewManager(endpoint, accessKeyID, secretAccessKey, bucketName, lastSetObjectName string, useSSL bool) (Manager, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return Manager{}, errors.Wrap(err, "new minio client")
	}

	return Manager{
		minioClient:       minioClient,
		ctx:               context.Background(),
		useSSL:            useSSL,
		bucketName:        bucketName,
		LastSetObjectName: lastSetObjectName,
	}, nil
}

// GetObjectContent return content by given object name
func (m *Manager) GetObjectContent(objectName string) ([]byte, error) {
	oldObj, err := m.minioClient.GetObject(m.ctx, m.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}

	var objCont bytes.Buffer
	_, err = io.Copy(&objCont, oldObj)
	if err != nil {
		return nil, errors.Wrap(err, "copy content")
	}

	return objCont.Bytes(), nil
}

type reader struct {
	r io.Reader
}

func (r *reader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

// PutObject puts new object to storage with given name and content
func (m *Manager) PutObject(name string, data io.Reader) error {
	r := reader{data}

	_, err := m.minioClient.PutObject(m.ctx, m.bucketName, name, &r, -1, minio.PutObjectOptions{ContentType: "application/text"})
	if err != nil {
		return errors.Wrap(err, "put object")
	}

	return nil
}
