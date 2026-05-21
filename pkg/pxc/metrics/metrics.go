package metrics

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sretry "k8s.io/client-go/util/retry"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

// PVCUsage contains information about PVC disk usage
type PVCUsage struct {
	PVCName      string
	UsedBytes    int64
	TotalBytes   int64
	UsagePercent int
}

func GetPVCUsage(
	ctx context.Context,
	clientcmd clientcmd.Client,
	pod *corev1.Pod,
	pvcName string,
) (*PVCUsage, error) {
	if pod == nil {
		return nil, errors.New("pod is nil")
	}

	backoff := wait.Backoff{
		Steps:    5,
		Duration: 5 * time.Second,
		Factor:   2.0,
	}

	// Execute df command in the pxc container to get disk usage
	// df -B1 /var/lib/mysql outputs in bytes
	// Example output:
	// Filesystem       1B-blocks       Used   Available Use% Mounted on
	// /dev/sdb        3094126592  221798400  2855550976   8% /var/lib/mysql
	var stdout, stderr bytes.Buffer
	command := []string{"df", "-B1", "/var/lib/mysql"}

	err := k8sretry.OnError(backoff, func(err error) bool { return true }, func() error {
		stdout.Reset()
		stderr.Reset()

		err := clientcmd.Exec(pod, naming.ContainerNamePXC, command, nil, &stdout, &stderr, false)
		if err != nil {
			return errors.Wrapf(err, "failed to execute df in pod %s: %s", pod.Name, stderr.String())
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "wait for df execution")
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		return nil, errors.Errorf("unexpected df output format: %s", stdout.String())
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return nil, errors.Errorf("unexpected df output fields: %s", lines[1])
	}

	totalBytes, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse total bytes: %s", fields[1])
	}

	usedBytes, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse used bytes: %s", fields[2])
	}

	usagePercent := 0
	if totalBytes > 0 {
		usagePercent = int((usedBytes * 100) / totalBytes)
	}

	return &PVCUsage{
		PVCName:      pvcName,
		UsedBytes:    usedBytes,
		TotalBytes:   totalBytes,
		UsagePercent: usagePercent,
	}, nil
}
