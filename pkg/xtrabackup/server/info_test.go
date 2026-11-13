package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
)

func TestParseBackupInfo(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedInfo *api.BackupInfo
		expectedErr  string
	}{
		{
			name: "plain file with all four LSN keys",
			input: strings.Join([]string{
				"from_lsn = 27655332",
				"to_lsn = 27660845",
				"last_lsn = 27660855",
				"flushed_lsn = 27660865",
				"",
			}, "\n"),
			expectedInfo: &api.BackupInfo{
				FromLsn:    "27655332",
				ToLsn:      "27660845",
				LastLsn:    "27660855",
				FlushedLsn: "27660865",
			},
		},
		{
			name: "file with extra unrelated keys",
			input: strings.Join([]string{
				"backup_type = incremental",
				"from_lsn = 27655332",
				"to_lsn = 27660845",
				"last_lsn = 27660855",
				"flushed_lsn = 27660865",
				"redo_memory = 0",
				"redo_frames = 0",
				"backup_size = 4096",
				"uncompressed_backup_size = 8192",
				"",
			}, "\n"),
			expectedInfo: &api.BackupInfo{
				FromLsn:    "27655332",
				ToLsn:      "27660845",
				LastLsn:    "27660855",
				FlushedLsn: "27660865",
			},
		},
		{
			name: "values with surrounding whitespace",
			input: strings.Join([]string{
				"  from_lsn   =   27655332  ",
				"\tto_lsn\t=\t27660845\t",
				"last_lsn =27660855",
				"flushed_lsn= 27660865 ",
				"",
			}, "\n"),
			expectedInfo: &api.BackupInfo{
				FromLsn:    "27655332",
				ToLsn:      "27660845",
				LastLsn:    "27660855",
				FlushedLsn: "27660865",
			},
		},
		{
			name: "LSN that would overflow int64 survives as an exact string",
			input: strings.Join([]string{
				"from_lsn = 18446744073709551614",
				"to_lsn = 18446744073709551615",
				"",
			}, "\n"),
			expectedInfo: &api.BackupInfo{
				FromLsn: "18446744073709551614",
				ToLsn:   "18446744073709551615",
			},
		},
		{
			name: "lines without a separator are ignored",
			input: strings.Join([]string{
				"this is not a key value line",
				"to_lsn = 27660845",
				"",
				"",
			}, "\n"),
			expectedInfo: &api.BackupInfo{
				ToLsn: "27660845",
			},
		},
		{
			name: "missing to_lsn is an error",
			input: strings.Join([]string{
				"backup_type = full-backuped",
				"from_lsn = 0",
				"last_lsn = 27660855",
				"",
			}, "\n"),
			expectedErr: "to_lsn is missing from xtrabackup_checkpoints",
		},
		{
			name:        "empty file is an error",
			input:       "",
			expectedErr: "to_lsn is missing from xtrabackup_checkpoints",
		},
		{
			name: "empty to_lsn value is an error",
			input: strings.Join([]string{
				"to_lsn = ",
				"",
			}, "\n"),
			expectedErr: "to_lsn is missing from xtrabackup_checkpoints",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseBackupInfo(strings.NewReader(tt.input))

			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, info)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.expectedInfo.FromLsn, info.FromLsn)
			assert.Equal(t, tt.expectedInfo.ToLsn, info.ToLsn)
			assert.Equal(t, tt.expectedInfo.LastLsn, info.LastLsn)
			assert.Equal(t, tt.expectedInfo.FlushedLsn, info.FlushedLsn)
		})
	}
}

func TestInfoFileCandidates(t *testing.T) {
	tests := []struct {
		name               string
		base               string
		expectedCandidates []string
	}{
		{
			name: "checkpoints file is probed plain, compressed, encrypted, then both",
			base: "xtrabackup_checkpoints",
			expectedCandidates: []string{
				"xtrabackup_checkpoints",
				"xtrabackup_checkpoints.zst",
				"xtrabackup_checkpoints.lz4",
				"xtrabackup_checkpoints.xbcrypt",
				"xtrabackup_checkpoints.zst.xbcrypt",
				"xtrabackup_checkpoints.lz4.xbcrypt",
			},
		},
		{
			name: "any base name follows the same ordering",
			base: "xtrabackup_info",
			expectedCandidates: []string{
				"xtrabackup_info",
				"xtrabackup_info.zst",
				"xtrabackup_info.lz4",
				"xtrabackup_info.xbcrypt",
				"xtrabackup_info.zst.xbcrypt",
				"xtrabackup_info.lz4.xbcrypt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedCandidates, infoFileCandidates(tt.base))
		})
	}
}

func TestExtractedFileName(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		expectedName string
	}{
		{
			name:         "plain file is unchanged",
			file:         "xtrabackup_checkpoints",
			expectedName: "xtrabackup_checkpoints",
		},
		{
			name:         "zstd suffix is stripped",
			file:         "xtrabackup_checkpoints.zst",
			expectedName: "xtrabackup_checkpoints",
		},
		{
			name:         "lz4 suffix is stripped",
			file:         "xtrabackup_checkpoints.lz4",
			expectedName: "xtrabackup_checkpoints",
		},
		{
			name:         "xbcrypt suffix is stripped",
			file:         "xtrabackup_checkpoints.xbcrypt",
			expectedName: "xtrabackup_checkpoints",
		},
		{
			name:         "compressed and encrypted suffixes are both stripped",
			file:         "xtrabackup_checkpoints.zst.xbcrypt",
			expectedName: "xtrabackup_checkpoints",
		},
		{
			name:         "lz4 and encrypted suffixes are both stripped",
			file:         "xtrabackup_checkpoints.lz4.xbcrypt",
			expectedName: "xtrabackup_checkpoints",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedName, extractedFileName(tt.file))
		})
	}
}

func TestXbstreamExtractArgs(t *testing.T) {
	tests := []struct {
		name         string
		backupConfig *api.BackupConfig
		expectedArgs []string
	}{
		{
			name:         "no container options",
			backupConfig: &api.BackupConfig{},
			expectedArgs: []string{"-x", "--decompress", "-C", "/tmp/dir"},
		},
		{
			name: "compression only adds no decrypt flags",
			backupConfig: &api.BackupConfig{
				ContainerOptions: &api.ContainerOptions{
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{"--compress"},
					},
				},
			},
			expectedArgs: []string{"-x", "--decompress", "-C", "/tmp/dir"},
		},
		{
			name: "encryption with a key file",
			backupConfig: &api.BackupConfig{
				ContainerOptions: &api.ContainerOptions{
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{"--compress", "--encrypt=AES256", "--encrypt-key-file=/etc/key"},
					},
				},
			},
			expectedArgs: []string{
				"-x", "--decompress",
				"--decrypt=AES256",
				"--encrypt-key-file=/etc/key",
				"-C", "/tmp/dir",
			},
		},
		{
			name: "encryption with an inline key",
			backupConfig: &api.BackupConfig{
				ContainerOptions: &api.ContainerOptions{
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{"--encrypt=AES128", "--encrypt-key=secret"},
					},
				},
			},
			expectedArgs: []string{
				"-x", "--decompress",
				"--decrypt=AES128",
				"--encrypt-key=secret",
				"-C", "/tmp/dir",
			},
		},
		{
			name: "a key without an algorithm is ignored",
			backupConfig: &api.BackupConfig{
				ContainerOptions: &api.ContainerOptions{
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{"--encrypt-key-file=/etc/key"},
					},
				},
			},
			expectedArgs: []string{"-x", "--decompress", "-C", "/tmp/dir"},
		},
		{
			// The form a user gets by copying a command line into the spec as
			// separate list items rather than joining each flag to its value.
			name: "encryption configured with separate arguments",
			backupConfig: &api.BackupConfig{
				ContainerOptions: &api.ContainerOptions{
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{"--compress", "--encrypt", "AES256", "--encrypt-key-file", "/etc/key"},
					},
				},
			},
			expectedArgs: []string{
				"-x", "--decompress",
				"--decrypt=AES256",
				"--encrypt-key-file=/etc/key",
				"-C", "/tmp/dir",
			},
		},
		{
			name: "encryption configured with the two forms mixed",
			backupConfig: &api.BackupConfig{
				ContainerOptions: &api.ContainerOptions{
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{"--encrypt=AES256", "--encrypt-key-file", "/etc/key"},
					},
				},
			},
			expectedArgs: []string{
				"-x", "--decompress",
				"--decrypt=AES256",
				"--encrypt-key-file=/etc/key",
				"-C", "/tmp/dir",
			},
		},
		{
			name: "a repeated algorithm takes the last occurrence",
			backupConfig: &api.BackupConfig{
				ContainerOptions: &api.ContainerOptions{
					Args: &api.BackupContainerArgs{
						Xtrabackup: []string{"--encrypt=AES128", "--encrypt=AES256", "--encrypt-key-file=/etc/key"},
					},
				},
			},
			expectedArgs: []string{
				"-x", "--decompress",
				"--decrypt=AES256",
				"--encrypt-key-file=/etc/key",
				"-C", "/tmp/dir",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedArgs, xbstreamExtractArgs(tt.backupConfig, "/tmp/dir"))
		})
	}
}

func TestFlagValue(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		flag          string
		expectedValue string
	}{
		{
			name:          "missing flag returns empty",
			args:          []string{"--compress"},
			flag:          "--encrypt",
			expectedValue: "",
		},
		{
			name:          "flag without a value returns empty",
			args:          []string{"--encrypt"},
			flag:          "--encrypt",
			expectedValue: "",
		},
		{
			name:          "flag with a value",
			args:          []string{"--compress", "--encrypt=AES256"},
			flag:          "--encrypt",
			expectedValue: "AES256",
		},
		{
			name:          "longer flag with the same prefix is not matched",
			args:          []string{"--encrypt-key-file=/etc/key"},
			flag:          "--encrypt",
			expectedValue: "",
		},
		{
			name:          "last occurrence wins",
			args:          []string{"--encrypt=AES128", "--encrypt=AES256"},
			flag:          "--encrypt",
			expectedValue: "AES256",
		},
		{
			name:          "value containing an equals sign is preserved",
			args:          []string{"--encrypt-key=a=b=c"},
			flag:          "--encrypt-key",
			expectedValue: "a=b=c",
		},
		{
			name:          "value given as the next argument",
			args:          []string{"--compress", "--encrypt", "AES256"},
			flag:          "--encrypt",
			expectedValue: "AES256",
		},
		{
			name:          "key file path given as the next argument",
			args:          []string{"--encrypt=AES256", "--encrypt-key-file", "/etc/key"},
			flag:          "--encrypt-key-file",
			expectedValue: "/etc/key",
		},
		{
			// --encrypt-key-file is not --encrypt, so the path after it must
			// not be read as the algorithm.
			name:          "a longer flag and its separate value are not matched",
			args:          []string{"--encrypt-key-file", "/etc/key"},
			flag:          "--encrypt",
			expectedValue: "",
		},
		{
			name:          "a trailing flag has no value to take",
			args:          []string{"--compress", "--encrypt"},
			flag:          "--encrypt",
			expectedValue: "",
		},
		{
			// Malformed input: --encrypt needs a value. Taking the next flag
			// would hand XtraBackup an algorithm named --encrypt-key-file.
			name:          "the following flag is not taken as a value",
			args:          []string{"--encrypt", "--encrypt-key-file=/etc/key"},
			flag:          "--encrypt",
			expectedValue: "",
		},
		{
			name:          "last occurrence wins across both forms",
			args:          []string{"--encrypt=AES128", "--encrypt", "AES256"},
			flag:          "--encrypt",
			expectedValue: "AES256",
		},
		{
			name:          "a separate value is overridden by a later joined one",
			args:          []string{"--encrypt", "AES128", "--encrypt=AES256"},
			flag:          "--encrypt",
			expectedValue: "AES256",
		},
		{
			name:          "a value that is not a flag is not mistaken for one",
			args:          []string{"--encrypt-key", "-dash-leading-key"},
			flag:          "--encrypt-key",
			expectedValue: "-dash-leading-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedValue, flagValue(tt.args, tt.flag))
		})
	}
}
