package storage

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestGetS3Options(t *testing.T) {
	ctx := context.Background()

	const ns = "my-ns"

	const storageName = "my-storage"
	const secretName = "my-secret"
	const accessKeyID = "some-access-key"
	const secretAccessKey = "some-secret-key"

	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name            string
		destination     string
		bucket          string
		accessKeyID     string
		secretAccessKey string
		endpoint        string
		region          string
		verifyTLS       *bool
		storage         *api.BackupStorageSpec

		expected    *S3Options
		expectedErr string
	}{
		{
			name:     "no secret",
			bucket:   "somebucket",
			endpoint: "some-endpoint",
			region:   "some-region",
			expected: &S3Options{
				Endpoint:   "some-endpoint",
				BucketName: "somebucket",
				Region:     "some-region",
				VerifyTLS:  true,
			},
		},
		{
			name:            "with secret",
			bucket:          "somebucket",
			accessKeyID:     accessKeyID,
			secretAccessKey: secretAccessKey,
			expected: &S3Options{
				BucketName:      "somebucket",
				AccessKeyID:     accessKeyID,
				SecretAccessKey: secretAccessKey,
				VerifyTLS:       true,
				Region:          "us-east-1",
			},
		},
		{
			name:   "bucket without prefix",
			bucket: "my-bucket",
			expected: &S3Options{
				BucketName: "my-bucket",
				VerifyTLS:  true,
				Region:     "us-east-1",
			},
		},
		{
			name:   "bucket with prefix",
			bucket: "my-bucket/prefix",
			expected: &S3Options{
				BucketName: "my-bucket",
				Prefix:     "prefix/",
				VerifyTLS:  true,
				Region:     "us-east-1",
			},
		},
		{
			name:        "destination with bucket",
			destination: "s3://invalid-bucket/prefix/backup-name",
			bucket:      "my-bucket",
			expected: &S3Options{
				BucketName: "my-bucket",
				VerifyTLS:  true,
				Region:     "us-east-1",
			},
		},
		{
			name:        "destination without prefix",
			destination: "s3://destination-bucket/backup-name",
			expected: &S3Options{
				BucketName: "destination-bucket",
				VerifyTLS:  true,
				Region:     "us-east-1",
			},
		},
		{
			name:        "destination with prefix",
			destination: "s3://destination-bucket/prefix/backup-name",
			expected: &S3Options{
				BucketName: "destination-bucket",
				Prefix:     "prefix/",
				VerifyTLS:  true,
				Region:     "us-east-1",
			},
		},
		{
			name:        "no destination",
			expectedErr: "bucket name is not set",
		},
		{
			name:      "verifyTLS in backup",
			bucket:    "somebucket",
			verifyTLS: boolPtr(false),
			expected: &S3Options{
				BucketName: "somebucket",
				VerifyTLS:  false,
				Region:     "us-east-1",
			},
		},
		{
			name:      "verifyTLS in backup and cluster",
			bucket:    "somebucket",
			verifyTLS: boolPtr(true),
			storage: &api.BackupStorageSpec{
				VerifyTLS: boolPtr(false),
			},
			expected: &S3Options{
				BucketName: "somebucket",
				VerifyTLS:  false,
				Region:     "us-east-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backup := testBackup(ns, storageName, tt.destination, tt.verifyTLS, &api.BackupStorageS3Spec{
				Bucket:            tt.bucket,
				CredentialsSecret: secretName,
				Region:            tt.region,
				EndpointURL:       tt.endpoint,
			}, nil)

			var cluster *api.PerconaXtraDBCluster
			if tt.storage != nil {
				cluster = &api.PerconaXtraDBCluster{
					Spec: api.PerconaXtraDBClusterSpec{
						Backup: &api.BackupSpec{
							Storages: map[string]*api.BackupStorageSpec{
								storageName: tt.storage,
							},
						},
					},
				}
			}

			objs := []runtime.Object{}
			if tt.accessKeyID != "" || tt.secretAccessKey != "" {
				objs = append(objs, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: ns,
					},
					Data: map[string][]byte{
						"AWS_ACCESS_KEY_ID":     []byte(tt.accessKeyID),
						"AWS_SECRET_ACCESS_KEY": []byte(tt.secretAccessKey),
					},
				})
			}
			cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

			opts, err := getS3OptionsFromBackup(ctx, cl, cluster, backup)
			if err != nil && tt.expectedErr != err.Error() {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(opts, tt.expected) {
				t.Fatalf("expected: %+v, got: %+v", tt.expected, opts)
			}
		})
	}
}

func TestGetAzureOptions(t *testing.T) {
	ctx := context.Background()

	const ns = "my-ns"

	const storageName = "my-storage"
	const secretName = "my-secret"
	const accountName = "some-access-key"
	const accountKey = "some-secret-key"

	tests := []struct {
		name        string
		destination string
		container   string
		accountName string
		accountKey  string
		endpoint    string

		expected    *AzureOptions
		expectedErr string
	}{
		{
			name:        "no secret",
			container:   "some-container",
			endpoint:    "some-endpoint",
			expectedErr: `failed to get secret: secrets "my-secret" not found`,
		},
		{
			name:        "container without prefix",
			container:   "my-container",
			accountName: accountName,
			accountKey:  accountKey,
			endpoint:    "some-endpoint",
			expected: &AzureOptions{
				StorageAccount: accountName,
				AccessKey:      accountKey,
				Container:      "my-container",
				Endpoint:       "some-endpoint",
			},
		},
		{
			name:        "container with prefix",
			container:   "my-container/prefix",
			accountName: accountName,
			accountKey:  accountKey,
			expected: &AzureOptions{
				StorageAccount: accountName,
				AccessKey:      accountKey,
				Container:      "my-container",
				Prefix:         "prefix/",
			},
		},
		{
			name:        "destination with container",
			destination: "azure://invalid-container/prefix/backup-name",
			container:   "my-container",
			accountName: accountName,
			accountKey:  accountKey,
			expected: &AzureOptions{
				StorageAccount: accountName,
				AccessKey:      accountKey,
				Container:      "my-container",
			},
		},
		{
			name:        "destination without prefix",
			destination: "azure://destination-container/backup-name",
			accountName: accountName,
			accountKey:  accountKey,
			expected: &AzureOptions{
				StorageAccount: accountName,
				AccessKey:      accountKey,
				Container:      "destination-container",
			},
		},
		{
			name:        "destination with prefix",
			destination: "azure://destination-container/prefix/backup-name",
			accountName: accountName,
			accountKey:  accountKey,
			expected: &AzureOptions{
				StorageAccount: accountName,
				AccessKey:      accountKey,
				Container:      "destination-container",
				Prefix:         "prefix/",
			},
		},
		{
			name:        "no destination",
			accountName: accountName,
			accountKey:  accountKey,
			expectedErr: "container name is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backup := testBackup(ns, storageName, tt.destination, nil, nil, &api.BackupStorageAzureSpec{
				ContainerPath:     tt.container,
				CredentialsSecret: secretName,
				Endpoint:          tt.endpoint,
			})

			objs := []runtime.Object{}
			if tt.accountName != "" || tt.accountKey != "" {
				objs = append(objs, testSecret(ns, secretName, map[string][]byte{
					"AZURE_STORAGE_ACCOUNT_NAME": []byte(tt.accountName),
					"AZURE_STORAGE_ACCOUNT_KEY":  []byte(tt.accountKey),
				}))
			}
			cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

			opts, err := getAzureOptionsFromBackup(ctx, cl, backup)
			if err != nil && tt.expectedErr != err.Error() {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(opts, tt.expected) {
				t.Fatalf("expected: %+v, got: %+v", tt.expected, opts)
			}
		})
	}
}

func testSecret(ns string, name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: data,
	}
}

func testBackup(ns string, storageName string, destination string, verifyTLS *bool, s3 *api.BackupStorageS3Spec, azure *api.BackupStorageAzureSpec) *api.PerconaXtraDBClusterBackup {
	return &api.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-backup",
			Namespace: ns,
		},
		Spec: api.PXCBackupSpec{
			StorageName: storageName,
		},
		Status: api.PXCBackupStatus{
			Destination: api.PXCBackupDestination(destination),
			S3:          s3,
			Azure:       azure,
			VerifyTLS:   verifyTLS,
		},
	}
}
