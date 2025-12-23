package api

import (
	context "context"
	"fmt"
	"testing"

	goversion "github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
)

func TestNewXtrabackupCmd(t *testing.T) {
	testCases := []struct {
		backupConfig *BackupConfig
		expectedArgs []string
	}{
		{
			backupConfig: &BackupConfig{
				Destination: "s3://bucket/backup",
				Type:        BackupStorageType_S3,
				VerifyTls:   true,
				ContainerOptions: &ContainerOptions{
					Args: &BackupContainerArgs{
						Xtrabackup: []string{
							"--compress",
							"--compress-threads=4",
							"--parallel=4",
						},
					},
				},
			},
			expectedArgs: []string{
				"xtrabackup",
				"--backup",
				"--stream=xbstream",
				"--safe-slave-backup",
				"--slave-info",
				"--target-dir=/backup/",
				"--socket=/tmp/mysql.sock",
				"--user=root",
				"--password=password123",
				"--compress",
				"--compress-threads=4",
				"--parallel=4",
			},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			cmd := tc.backupConfig.NewXtrabackupCmd(
				context.Background(),
				"root", "password123", goversion.Must(goversion.NewVersion("8.0.0")), false)
			assert.Equal(t, tc.expectedArgs, cmd.Args)
		})
	}
}

func TestNewXbcloudCmd(t *testing.T) {
	testCases := []struct {
		name         string
		backupConfig *BackupConfig
		action       XBCloudAction
		expectedArgs []string
		expectedEnv  []string // Custom env vars that should be present (in addition to os.Environ())
	}{
		{
			name: "S3 storage with verify TLS and put action",
			backupConfig: &BackupConfig{
				Destination: "s3://bucket/backup-name",
				Type:        BackupStorageType_S3,
				VerifyTls:   true,
				S3: &S3Config{
					Bucket:      "test-bucket",
					Region:      "us-west-2",
					AccessKey:   "access-key-123",
					SecretKey:   "secret-key-456",
					EndpointUrl: "",
				},
			},
			action: XBCloudActionPut,
			expectedArgs: []string{
				"xbcloud",
				"put",
				"--parallel=10",
				"--curl-retriable-errors=7",
				"--md5",
				"--storage=s3",
				"--s3-bucket=test-bucket",
				"--s3-region=us-west-2",
				"--s3-access-key=access-key-123",
				"--s3-secret-key=secret-key-456",
				"s3://bucket/backup-name",
			},
			expectedEnv: []string{},
		},
		{
			name: "S3 storage without verify TLS and delete action",
			backupConfig: &BackupConfig{
				Destination: "s3://bucket/backup-name",
				Type:        BackupStorageType_S3,
				VerifyTls:   false,
				S3: &S3Config{
					Bucket:      "test-bucket",
					Region:      "eu-central-1",
					AccessKey:   "access-key-789",
					SecretKey:   "secret-key-012",
					EndpointUrl: "https://s3.custom.endpoint.com",
				},
			},
			action: XBCloudActionDelete,
			expectedArgs: []string{
				"xbcloud",
				"delete",
				"--parallel=10",
				"--curl-retriable-errors=7",
				"--insecure",
				"--md5",
				"--storage=s3",
				"--s3-bucket=test-bucket",
				"--s3-region=eu-central-1",
				"--s3-access-key=access-key-789",
				"--s3-secret-key=secret-key-012",
				"--s3-endpoint=https://s3.custom.endpoint.com",
				"s3://bucket/backup-name",
			},
			expectedEnv: []string{},
		},
		{
			name: "S3 storage with session token",
			backupConfig: &BackupConfig{
				Destination: "s3://bucket/backup-name",
				Type:        BackupStorageType_S3,
				VerifyTls:   true,
				S3: &S3Config{
					Bucket:       "test-bucket",
					Region:       "us-west-2",
					AccessKey:    "access-key-123",
					SecretKey:    "secret-key-456",
					SessionToken: "session-token-789",
					EndpointUrl:  "",
				},
			},
			action: XBCloudActionPut,
			expectedArgs: []string{
				"xbcloud",
				"put",
				"--parallel=10",
				"--curl-retriable-errors=7",
				"--md5",
				"--storage=s3",
				"--s3-bucket=test-bucket",
				"--s3-region=us-west-2",
				"--s3-access-key=access-key-123",
				"--s3-secret-key=secret-key-456",
				"--s3-session-token=session-token-789",
				"s3://bucket/backup-name",
			},
			expectedEnv: []string{},
		},
		{
			name: "S3 storage with container options and xbcloud args",
			backupConfig: &BackupConfig{
				Destination: "s3://bucket/backup-name",
				Type:        BackupStorageType_S3,
				VerifyTls:   true,
				S3: &S3Config{
					Bucket:      "test-bucket",
					Region:      "us-east-1",
					AccessKey:   "access-key",
					SecretKey:   "secret-key",
					EndpointUrl: "",
				},
				ContainerOptions: &ContainerOptions{
					Env: []*EnvVar{
						{Key: "AWS_PROFILE", Value: "production"},
						{Key: "CUSTOM_VAR", Value: "custom-value"},
					},
					Args: &BackupContainerArgs{
						Xbcloud: []string{
							"--xb-cloud-arg=some-vaule",
						},
					},
				},
			},
			action: XBCloudActionPut,
			expectedArgs: []string{
				"xbcloud",
				"put",
				"--parallel=10",
				"--curl-retriable-errors=7",
				"--xb-cloud-arg=some-vaule",
				"--md5",
				"--storage=s3",
				"--s3-bucket=test-bucket",
				"--s3-region=us-east-1",
				"--s3-access-key=access-key",
				"--s3-secret-key=secret-key",
				"s3://bucket/backup-name",
			},
			expectedEnv: []string{
				"AWS_PROFILE=production",
				"CUSTOM_VAR=custom-value",
			},
		},
		{
			name: "Azure storage with verify TLS and put action",
			backupConfig: &BackupConfig{
				Destination: "azure://container/backup-name",
				Type:        BackupStorageType_AZURE,
				VerifyTls:   true,
				Azure: &AzureConfig{
					StorageAccount: "storage-account-123",
					ContainerName:  "backup-container",
					AccessKey:      "azure-access-key",
					EndpointUrl:    "",
				},
			},
			action: XBCloudActionPut,
			expectedArgs: []string{
				"xbcloud",
				"put",
				"--parallel=10",
				"--curl-retriable-errors=7",
				"--storage=azure",
				"--azure-storage-account=storage-account-123",
				"--azure-container-name=backup-container",
				"--azure-access-key=azure-access-key",
				"azure://container/backup-name",
			},
			expectedEnv: []string{},
		},
		{
			name: "Azure storage without verify TLS and delete action",
			backupConfig: &BackupConfig{
				Destination: "azure://container/backup-name",
				Type:        BackupStorageType_AZURE,
				VerifyTls:   false,
				Azure: &AzureConfig{
					StorageAccount: "storage-account-456",
					ContainerName:  "backup-container",
					AccessKey:      "azure-access-key-789",
					EndpointUrl:    "https://custom.azure.endpoint.net",
				},
			},
			action: XBCloudActionDelete,
			expectedArgs: []string{
				"xbcloud",
				"delete",
				"--parallel=10",
				"--curl-retriable-errors=7",
				"--insecure",
				"--storage=azure",
				"--azure-storage-account=storage-account-456",
				"--azure-container-name=backup-container",
				"--azure-access-key=azure-access-key-789",
				"--azure-endpoint=https://custom.azure.endpoint.net",
				"azure://container/backup-name",
			},
			expectedEnv: []string{},
		},
		{
			name: "Azure storage with container options and env vars",
			backupConfig: &BackupConfig{
				Destination: "azure://container/backup-name",
				Type:        BackupStorageType_AZURE,
				VerifyTls:   true,
				Azure: &AzureConfig{
					StorageAccount: "storage-account",
					ContainerName:  "backup-container",
					AccessKey:      "azure-access-key",
					EndpointUrl:    "https://storage.azure.net",
				},
				ContainerOptions: &ContainerOptions{
					Env: []*EnvVar{
						{Key: "AZURE_STORAGE_CONNECTION_STRING", Value: "DefaultEndpointsProtocol=https"},
						{Key: "LOG_LEVEL", Value: "debug"},
					},
					Args: &BackupContainerArgs{
						Xbcloud: []string{
							"--xb-cloud-arg=some-vaule",
						},
					},
				},
			},
			action: XBCloudActionPut,
			expectedArgs: []string{
				"xbcloud",
				"put",
				"--parallel=10",
				"--curl-retriable-errors=7",
				"--xb-cloud-arg=some-vaule",
				"--storage=azure",
				"--azure-storage-account=storage-account",
				"--azure-container-name=backup-container",
				"--azure-access-key=azure-access-key",
				"--azure-endpoint=https://storage.azure.net",
				"azure://container/backup-name",
			},
			expectedEnv: []string{
				"AZURE_STORAGE_CONNECTION_STRING=DefaultEndpointsProtocol=https",
				"LOG_LEVEL=debug",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.backupConfig.NewXbcloudCmd(context.Background(), tc.action, nil)

			// Verify command arguments
			assert.Equal(t, tc.expectedArgs, cmd.Args)

			// Verify environment variables
			// The env will include os.Environ() plus custom env vars
			envMap := make(map[string]string)
			for _, env := range cmd.Env {
				parts := splitEnv(env)
				if len(parts) == 2 {
					envMap[parts[0]] = parts[1]
				}
			}

			// Check that all expected custom env vars are present
			for _, expectedEnv := range tc.expectedEnv {
				parts := splitEnv(expectedEnv)
				if len(parts) == 2 {
					assert.Contains(t, cmd.Env, expectedEnv, "Expected env var %s not found", expectedEnv)
					assert.Equal(t, parts[1], envMap[parts[0]], "Env var %s has wrong value", parts[0])
				}
			}
		})
	}
}

// Helper function to split env var string into key and value
func splitEnv(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}
