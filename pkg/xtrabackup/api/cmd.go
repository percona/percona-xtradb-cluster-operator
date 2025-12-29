package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	goversion "github.com/hashicorp/go-version"
)

const (
	xtrabackupCmd = "xtrabackup"
	xbcloudCmd    = "xbcloud"
)

type XBCloudAction string

const (
	XBCloudActionPut    XBCloudAction = "put"
	XBCloudActionDelete XBCloudAction = "delete"
)

// NewXtrabackupCmd creates a new xtrabackup command
func (cfg *BackupConfig) NewXtrabackupCmd(
	ctx context.Context,
	user,
	password string,
	mysqlVersion *goversion.Version,
	withTablespaceEncryption bool) *exec.Cmd {
	cmd := exec.CommandContext(ctx, xtrabackupCmd, cfg.xtrabackupArgs(user, password, mysqlVersion, withTablespaceEncryption)...)
	cmd.Env = cfg.envs()
	return cmd
}

// NewXbcloudCmd creates a new xbcloud command
func (cfg *BackupConfig) NewXbcloudCmd(ctx context.Context, action XBCloudAction, in io.Reader) *exec.Cmd {
	cmd := exec.CommandContext(ctx, xbcloudCmd, cfg.xbcloudArgs(action)...)
	cmd.Env = cfg.envs()
	cmd.Stdin = in
	return cmd
}

func (cfg *BackupConfig) xbcloudArgs(action XBCloudAction) []string {
	args := []string{string(action), "--parallel=10", "--curl-retriable-errors=7"}

	if !cfg.VerifyTls {
		args = append(args, "--insecure")
	}

	if cfg.ContainerOptions != nil && cfg.ContainerOptions.Args != nil {
		args = append(args, cfg.ContainerOptions.Args.Xbcloud...)
	}

	switch cfg.Type {
	case BackupStorageType_GCS:
		args = append(
			args,
			[]string{
				"--md5",
				"--storage=google",
				fmt.Sprintf("--google-bucket=%s", cfg.Gcs.Bucket),
				fmt.Sprintf("--google-access-key=%s", cfg.Gcs.AccessKey),
				fmt.Sprintf("--google-secret-key=%s", cfg.Gcs.SecretKey),
			}...,
		)
		if len(cfg.Gcs.EndpointUrl) > 0 {
			args = append(args, fmt.Sprintf("--google-endpoint=%s", cfg.Gcs.EndpointUrl))
		}
	case BackupStorageType_S3:
		args = append(
			args,
			[]string{
				"--md5",
				"--storage=s3",
				fmt.Sprintf("--s3-bucket=%s", cfg.S3.Bucket),
				fmt.Sprintf("--s3-region=%s", cfg.S3.Region),
				fmt.Sprintf("--s3-access-key=%s", cfg.S3.AccessKey),
				fmt.Sprintf("--s3-secret-key=%s", cfg.S3.SecretKey),
			}...,
		)
		if len(cfg.S3.SessionToken) > 0 {
			args = append(args, fmt.Sprintf("--s3-session-token=%s", cfg.S3.SessionToken))
		}
		if len(cfg.S3.EndpointUrl) > 0 {
			args = append(args, fmt.Sprintf("--s3-endpoint=%s", cfg.S3.EndpointUrl))
		}
	case BackupStorageType_AZURE:
		args = append(
			args,
			[]string{
				"--storage=azure",
				fmt.Sprintf("--azure-storage-account=%s", cfg.Azure.StorageAccount),
				fmt.Sprintf("--azure-container-name=%s", cfg.Azure.ContainerName),
				fmt.Sprintf("--azure-access-key=%s", cfg.Azure.AccessKey),
			}...,
		)
		if len(cfg.Azure.EndpointUrl) > 0 {
			args = append(args, fmt.Sprintf("--azure-endpoint=%s", cfg.Azure.EndpointUrl))
		}
	}

	args = append(args, cfg.Destination)

	return args
}

func (cfg *BackupConfig) xtrabackupArgs(user, pass string, mysqlVersion *goversion.Version, withTablespaceEncryption bool) []string {
	args := []string{
		"--backup",
		"--stream=xbstream",
		"--safe-slave-backup",
		"--slave-info",
		"--target-dir=/backup/",
		"--socket=/tmp/mysql.sock",
		fmt.Sprintf("--user=%s", user),
		fmt.Sprintf("--password=%s", pass),
	}
	if withTablespaceEncryption {
		args = append(args, "--generate-transition-key")

		vaultConfigFlag := "--keyring-vault-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf"
		if mysqlVersion.Compare(goversion.Must(goversion.NewVersion("8.4.0"))) >= 0 {
			vaultConfigFlag = "--component-keyring-config=/etc/mysql/vault-keyring-secret/keyring_vault.conf"
		}
		args = append(args, vaultConfigFlag)
	}
	if cfg != nil && cfg.ContainerOptions != nil && cfg.ContainerOptions.Args != nil {
		args = append(args, cfg.ContainerOptions.Args.Xtrabackup...)
	}
	return args
}

func (cfg *BackupConfig) envs() []string {
	envs := os.Environ()
	if cfg.ContainerOptions != nil {
		for _, env := range cfg.ContainerOptions.Env {
			envs = append(envs, fmt.Sprintf("%s=%s", env.Key, env.Value))
		}
	}
	return envs
}
