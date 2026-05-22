package storage

import (
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestMinioPutObjectOptions(t *testing.T) {
	tests := map[string]struct {
		size      int64
		algorithm api.S3ChecksumAlgorithmType
		expected  minio.PutObjectOptions
	}{
		"default": {
			expected: minio.PutObjectOptions{},
		},
		"sha256": {
			algorithm: api.S3ChecksumAlgorithmSHA256,
			expected:  minio.PutObjectOptions{AutoChecksum: minio.ChecksumSHA256},
		},
		"sha1": {
			algorithm: api.S3ChecksumAlgorithmSHA1,
			expected:  minio.PutObjectOptions{AutoChecksum: minio.ChecksumSHA1},
		},
		"crc32": {
			algorithm: api.S3ChecksumAlgorithmCRC32,
			expected:  minio.PutObjectOptions{AutoChecksum: minio.ChecksumCRC32},
		},
		"crc32c": {
			algorithm: api.S3ChecksumAlgorithmCRC32C,
			expected:  minio.PutObjectOptions{AutoChecksum: minio.ChecksumCRC32C},
		},
		"crc64nvme": {
			algorithm: api.S3ChecksumAlgorithmCRC64NVME,
			expected:  minio.PutObjectOptions{AutoChecksum: minio.ChecksumCRC64NVME},
		},
		"md5": {
			algorithm: api.S3ChecksumAlgorithmMD5,
			expected:  minio.PutObjectOptions{SendContentMd5: true},
		},
		"streaming upload disables checksum": {
			size:      -1,
			algorithm: api.S3ChecksumAlgorithmSHA256,
			expected:  minio.PutObjectOptions{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expected, minioPutObjectOptions(tt.size, tt.algorithm))
		})
	}
}
