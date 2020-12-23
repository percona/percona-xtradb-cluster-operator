package pxc

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var ErrNotAllPXCPodsRunning = errors.New("not all pxc pods are running")

var sequenceRegexp = regexp.MustCompile(`node with sequence number [(]seqno[)]: ([-]?\d+)`)

const crashBorder = `################################################################################################################################`

func (r *ReconcilePerconaXtraDBCluster) recoverFullClusterCrashIfNeeded(cr *v1.PerconaXtraDBCluster) error {

	err := r.checkIfPodsRunning(cr)
	if err != nil {
		if err == ErrNotAllPXCPodsRunning {
			return nil
		}
		return err
	}

	logLinesRequired := int64(7)
	logOpts := &corev1.PodLogOptions{
		Container: "pxc",
		TailLines: &logLinesRequired,
	}
	logs, err := r.clientcmd.PodLogs(cr.Namespace, cr.Name+"-pxc-0", logOpts)
	if err != nil {
		return errors.Wrap(err, "get logs from pxc 0 pod")
	}

	if strings.HasPrefix(logs, crashBorder+"\n") && strings.HasSuffix(logs, crashBorder+"\n") &&
		strings.Contains(logs, "You have the situation of a full PXC cluster crash.") {
		return r.doFullCrashRecovery(cr.Name, cr.Namespace, int(cr.Spec.PXC.Size))
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) doFullCrashRecovery(crName, namespace string, pxcSize int) error {
	maxSeq := int64(-100)
	maxSeqPod := ""
	logLinesRequired := int64(7)
	logOpts := &corev1.PodLogOptions{
		Container: "pxc",
		TailLines: &logLinesRequired,
	}

	for i := 0; i < pxcSize; i++ {
		podName := fmt.Sprintf("%s-pxc-%d", crName, i)
		logs, err := r.clientcmd.PodLogs(namespace, podName, logOpts)
		if err != nil {
			return errors.Wrapf(err, "get logs from %s pod", podName)
		}

		if !strings.HasPrefix(logs, crashBorder+"\n") || !strings.HasSuffix(logs, crashBorder+"\n") ||
			!strings.Contains(logs, "You have the situation of a full PXC cluster crash.") {
			return nil
		}

		seqStrSplit := sequenceRegexp.FindStringSubmatch(logs)
		if len(seqStrSplit) != 2 {
			return errors.Wrapf(err, "get sequence number from %s pod, seqSTR: %s", podName, seqStrSplit)
		}

		seq, err := strconv.ParseInt(strings.TrimSpace(seqStrSplit[1]), 10, 64)
		if err != nil {
			return errors.Wrapf(err, "parse sequence %s", seqStrSplit[1])
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
		return errors.New("Invalid exec command return " + stderrBuf.String())
	}

	// sleep there a little to start script and do not send
	// a lot of signals to the same pod
	time.Sleep(10 * time.Second)

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
