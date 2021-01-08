package pxc

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var (
	ErrNotAllPXCPodsRunning = errors.New("not all pxc pods are running")
	logLinesRequired        = int64(1)
)

const logPrefix = `#####################################################LAST_LINE`

func (r *ReconcilePerconaXtraDBCluster) recoverFullClusterCrashIfNeeded(cr *v1.PerconaXtraDBCluster) error {
	if cr.Spec.PXC.Size <= 0 {
		return nil
	}

	err := r.checkIfPodsRunning(cr)
	if err != nil {
		if err == ErrNotAllPXCPodsRunning {
			return nil
		}
		return err
	}

	logOpts := &corev1.PodLogOptions{
		Container: "pxc",
		TailLines: &logLinesRequired,
	}
	logs, err := r.clientcmd.PodLogs(cr.Namespace, cr.Name+"-pxc-0", logOpts)
	if err != nil {
		return errors.Wrap(err, "get logs from pxc 0 pod")
	}

	if len(logs) != 1 {
		return nil
	}

	if strings.HasPrefix(logs[0], logPrefix) {
		return r.doFullCrashRecovery(cr.Name, cr.Namespace, int(cr.Spec.PXC.Size))
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) isPodWaitingForRecovery(namespace, podName string) (bool, int64, error) {
	logOpts := &corev1.PodLogOptions{
		Container: "pxc",
		TailLines: &logLinesRequired,
	}
	logs, err := r.clientcmd.PodLogs(namespace, podName, logOpts)
	if err != nil {
		return false, -1, errors.Wrapf(err, "get logs from %s pod", podName)
	}

	if len(logs) != 1 {
		return false, -1, nil
	}

	if !strings.HasPrefix(logs[0], logPrefix) {
		return false, -1, nil
	}

	logsSplitted := strings.Split(logs[0], ":")
	if len(logsSplitted) != 4 {
		return false, -1, nil
	}

	seq, err := strconv.ParseInt(logsSplitted[2], 10, 64)
	if err != nil {
		return true, -1, errors.Wrapf(err, "parse sequence %s", logsSplitted[2])
	}

	return true, seq, nil
}

func (r *ReconcilePerconaXtraDBCluster) doFullCrashRecovery(crName, namespace string, pxcSize int) error {
	maxSeq := int64(-100)
	maxSeqPod := ""

	for i := 0; i < pxcSize; i++ {
		podName := fmt.Sprintf("%s-pxc-%d", crName, i)
		isPodWaitingForRecovery, seq, err := r.isPodWaitingForRecovery(namespace, podName)
		if err != nil {
			return errors.Wrapf(err, "parse %s pod logs", podName)
		}

		if !isPodWaitingForRecovery {
			return nil
		}

		if seq > maxSeq {
			maxSeq = seq
			maxSeqPod = podName
		}
	}
	log.Info("We are in full cluster crash, starting recovery")
	log.Info("Results of scanning sequences", "pod", maxSeqPod, "maxSeq", maxSeq)

	pod := &corev1.Pod{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      maxSeqPod,
	}, pod)
	if err != nil {
		return errors.Wrap(err, "get pods defenition")
	}

	stderrBuf := &bytes.Buffer{}
	err = r.clientcmd.Exec(pod, "pxc", []string{"/bin/sh", "-c", "kill -s USR1 1"}, nil, nil, stderrBuf, false)
	if err != nil {
		return errors.Wrap(err, "exec command in pod")
	}

	if stderrBuf.Len() != 0 {
		return errors.New("invalid exec command return: " + stderrBuf.String())
	}

	// sleep there a little to start script and do not send
	// a lot of signals to the same pod
	time.Sleep(30 * time.Second)

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) checkIfPodsRunning(cr *v1.PerconaXtraDBCluster) error {
	for i := 0; i < int(cr.Spec.PXC.Size); i++ {
		podName := fmt.Sprintf("%s-pxc-%d", cr.Name, i)
		ok, err := r.clientcmd.IsPodRunning(cr.Namespace, podName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return ErrNotAllPXCPodsRunning
			}
			return errors.Wrapf(err, "can't check pod %s state", podName)
		}
		if !ok {
			return ErrNotAllPXCPodsRunning
		}
	}
	return nil
}
