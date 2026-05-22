package metrics

import (
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

// mockClientCmd is a mock implementation of the ClientCmd interface for testing
type mockClientCmd struct {
	execFunc func(pod *corev1.Pod, containerName string, command []string, stdin io.Reader, stdout, stderr io.Writer, tty bool) error
}

func (m *mockClientCmd) Exec(pod *corev1.Pod, containerName string, command []string, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	if m.execFunc != nil {
		return m.execFunc(pod, containerName, command, stdin, stdout, stderr, tty)
	}
	return nil
}

func (m *mockClientCmd) IsPodRunning(namespace, podName string) (bool, error) {
	return true, nil
}

func (m *mockClientCmd) PodLogs(namespace, podName string, opts *corev1.PodLogOptions) ([]string, error) {
	return nil, nil
}

func (m *mockClientCmd) REST() restclient.Interface {
	return nil
}

func TestGetPVCUsageFromMetrics(t *testing.T) {
	tests := map[string]struct {
		pvcName     string
		dfOutput    string
		dfError     error
		expectedErr bool
		expected    *PVCUsage
	}{
		"successful df output parsing": {
			pvcName: "datadir-test-cluster-pxc-0",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb        3094126592  221798400  2855550976   8% /var/lib/mysql`,
			expected: &PVCUsage{
				PVCName:      "datadir-test-cluster-pxc-0",
				UsedBytes:    221798400,
				TotalBytes:   3094126592,
				UsagePercent: 7, // (221798400 * 100) / 3094126592 = 7.16... = 7
			},
		},
		"high usage percentage": {
			pvcName: "datadir-test-cluster-pxc-1",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb       10737418240 9663676416  1073741824  90% /var/lib/mysql`,
			expected: &PVCUsage{
				PVCName:      "datadir-test-cluster-pxc-1",
				UsedBytes:    9663676416,
				TotalBytes:   10737418240,
				UsagePercent: 90,
			},
		},
		"zero total bytes": {
			pvcName: "datadir-test-cluster-pxc-2",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb                 0          0           0   0% /var/lib/mysql`,
			expected: &PVCUsage{
				PVCName:      "datadir-test-cluster-pxc-2",
				UsedBytes:    0,
				TotalBytes:   0,
				UsagePercent: 0,
			},
		},
		"100% usage": {
			pvcName: "datadir-test-cluster-pxc-3",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb        1073741824 1073741824           0 100% /var/lib/mysql`,
			expected: &PVCUsage{
				PVCName:      "datadir-test-cluster-pxc-3",
				UsedBytes:    1073741824,
				TotalBytes:   1073741824,
				UsagePercent: 100,
			},
		},
		"exec command fails": {
			pvcName:     "datadir-test-cluster-pxc-4",
			dfOutput:    "",
			dfError:     errors.New("connection refused"),
			expectedErr: true,
		},
		"invalid df output - less than 2 lines": {
			pvcName:     "datadir-test-cluster-pxc-5",
			dfOutput:    `Filesystem       1B-blocks       Used   Available Use% Mounted on`,
			expectedErr: true,
		},
		"invalid df output - less than 6 fields": {
			pvcName: "datadir-test-cluster-pxc-6",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb        3094126592  221798400`,
			expectedErr: true,
		},
		"invalid total bytes format": {
			pvcName: "datadir-test-cluster-pxc-7",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb        invalid  221798400  2855550976   8% /var/lib/mysql`,
			expectedErr: true,
		},
		"invalid used bytes format": {
			pvcName: "datadir-test-cluster-pxc-8",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb        3094126592  invalid  2855550976   8% /var/lib/mysql`,
			expectedErr: true,
		},
		"large volume with fractional percentage": {
			pvcName: "datadir-test-cluster-pxc-9",
			dfOutput: `Filesystem       1B-blocks       Used   Available Use% Mounted on
/dev/sdb       107374182400 5368709120 102005473280   5% /var/lib/mysql`,
			expected: &PVCUsage{
				PVCName:      "datadir-test-cluster-pxc-9",
				UsedBytes:    5368709120,
				TotalBytes:   107374182400,
				UsagePercent: 5,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mockCmd := &mockClientCmd{
				execFunc: func(pod *corev1.Pod, containerName string, command []string, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
					if tt.dfError != nil {
						if stderr != nil {
							_, _ = stderr.Write([]byte("error executing df command"))
						}
						return tt.dfError
					}
					if stdout != nil {
						_, _ = stdout.Write([]byte(tt.dfOutput))
					}
					return nil
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-0",
					Namespace: "test-namespace",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: naming.ComponentPXC,
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: naming.ComponentPXC,
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			}

			result, err := GetPVCUsage(t.Context(), mockCmd, pod, tt.pvcName)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.PVCName, result.PVCName)
				assert.Equal(t, tt.expected.UsedBytes, result.UsedBytes)
				assert.Equal(t, tt.expected.TotalBytes, result.TotalBytes)
				assert.Equal(t, tt.expected.UsagePercent, result.UsagePercent)
			}
		})
	}
}
