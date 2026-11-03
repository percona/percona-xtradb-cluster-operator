package server

import (
	"bufio"
	"bytes"
	"context"
	stderrors "errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/xtrabackup/api"
)

const (
	xbstreamCmd     = "xbstream"
	checkpointsFile = "xtrabackup_checkpoints"
)

// compressExtensions lists the file extensions produced by the compression
// algorithms xtrabackup supports.
var compressExtensions = []string{".zst", ".lz4"}

// GetBackupInfo reads the xtrabackup_checkpoints file of an already uploaded
// backup and returns its LSN values.
func (s *appServer) GetBackupInfo(ctx context.Context, req *api.GetBackupInfoRequest) (*api.BackupInfo, error) {
	log := s.log.WithName("xtrabackup-server").WithName("GetBackupInfo")

	cfg := req.GetBackupConfig()
	if cfg == nil {
		return nil, status.Error(codes.InvalidArgument, "backup_config is required")
	}

	log = log.WithValues("namespace", s.namespace, "destination", cfg.Destination, "storage", cfg.Type)

	// With compression or encryption enabled the object is stored under a
	// suffixed name, so every variant has to be probed.
	candidates := infoFileCandidates(checkpointsFile)
	probeErrs := make([]error, 0, len(candidates))
	for _, candidate := range candidates {
		info, err := s.fetchBackupInfoFile(ctx, log, cfg, candidate)
		if err != nil {
			log.V(1).Info("Failed to read backup info candidate", "file", candidate, "error", err.Error())
			probeErrs = append(probeErrs, errors.Wrapf(err, "file %s", candidate))
			continue
		}

		log.Info("Read backup info", "file", candidate, "toLSN", info.ToLsn)
		return info, nil
	}

	return nil, errors.Wrapf(stderrors.Join(probeErrs...), "read %s from %s", checkpointsFile, cfg.Destination)
}

func (s *appServer) fetchBackupInfoFile(
	ctx context.Context,
	log logr.Logger,
	cfg *api.BackupConfig,
	file string,
) (*api.BackupInfo, error) {
	tmpDir, err := os.MkdirTemp("", "xb-info-*")
	if err != nil {
		return nil, errors.Wrap(err, "create temp dir")
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	xbcloud := cfg.NewXbcloudCmd(ctx, api.XBCloudActionGet, nil, file)
	xbstream := exec.CommandContext(ctx, xbstreamCmd, xbstreamExtractArgs(cfg, tmpDir)...)

	var xbcloudErr, xbstreamErr bytes.Buffer
	xbcloud.Stderr = &xbcloudErr
	xbstream.Stderr = &xbstreamErr

	xbcloudOut, err := xbcloud.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "xbcloud stdout pipe failed")
	}
	xbstream.Stdin = xbcloudOut

	log.V(1).Info(
		"Fetching backup info file",
		"file", file,
		"xbcloudCmd", sanitizeCmd(xbcloud),
		"xbstreamCmd", sanitizeCmd(xbstream),
	)

	if err := xbcloud.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start xbcloud")
	}
	if err := xbstream.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start xbstream")
	}

	if err := xbstream.Wait(); err != nil {
		_ = xbcloud.Wait()
		return nil, errors.Wrapf(err, "xbstream failed: %s", xbstreamErr.String())
	}
	if err := xbcloud.Wait(); err != nil {
		return nil, errors.Wrapf(err, "xbcloud failed: %s", xbcloudErr.String())
	}

	f, err := os.Open(filepath.Join(tmpDir, extractedFileName(file)))
	if err != nil {
		return nil, errors.Wrap(err, "open extracted file")
	}
	defer f.Close() //nolint:errcheck

	info, err := parseBackupInfo(f)
	if err != nil {
		return nil, errors.Wrapf(err, "parse %s", file)
	}
	return info, nil
}

func infoFileCandidates(base string) []string {
	candidates := []string{base}
	for _, ext := range compressExtensions {
		candidates = append(candidates, base+ext)
	}
	candidates = append(candidates, base+".xbcrypt")
	for _, ext := range compressExtensions {
		candidates = append(candidates, base+ext+".xbcrypt")
	}
	return candidates
}

func extractedFileName(file string) string {
	file = strings.TrimSuffix(file, ".xbcrypt")
	for _, ext := range compressExtensions {
		file = strings.TrimSuffix(file, ext)
	}
	return file
}

func xbstreamExtractArgs(cfg *api.BackupConfig, dir string) []string {
	args := []string{"-x", "--decompress"}
	args = append(args, xbstreamDecryptArgs(cfg)...)
	return append(args, "-C", dir)
}

func xbstreamDecryptArgs(cfg *api.BackupConfig) []string {
	xtrabackupArgs := cfg.GetContainerOptions().GetArgs().GetXtrabackup()

	algorithm := flagValue(xtrabackupArgs, "--encrypt")
	if algorithm == "" {
		return nil
	}

	args := []string{"--decrypt=" + algorithm}
	for _, flag := range []string{"--encrypt-key-file", "--encrypt-key"} {
		if v := flagValue(xtrabackupArgs, flag); v != "" {
			args = append(args, flag+"="+v)
		}
	}
	return args
}

func flagValue(args []string, flag string) string {
	prefix := flag + "="
	value := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			value = strings.TrimPrefix(arg, prefix)
		}
	}
	return value
}

func parseBackupInfo(r io.Reader) (*api.BackupInfo, error) {
	info := new(api.BackupInfo)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "from_lsn":
			info.FromLsn = value
		case "to_lsn":
			info.ToLsn = value
		case "last_lsn":
			info.LastLsn = value
		case "flushed_lsn":
			info.FlushedLsn = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "scan checkpoints file")
	}

	// An empty to_lsn would be passed on as `--incremental-lsn=`, which silently
	// produces an unusable backup, so refuse it here instead.
	if info.ToLsn == "" {
		return nil, errors.Errorf("to_lsn is missing from %s", checkpointsFile)
	}

	return info, nil
}
