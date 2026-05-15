package pxc

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

var (
	ErrNotAllPXCPodsRunning = errors.New("not all pxc pods are running")
	logLinesRequired        = int64(9)
)

const logPrefix = `#####################################################LAST_LINE`

// recoverFullClusterCrashIfNeeded detects a full PXC cluster crash and
// triggers a Galera bootstrap from the pod with the highest recovered seqno.
//
// The flow is split between the PXC entrypoint (build/pxc-entrypoint.sh) and
// this controller. The entrypoint stalls each pod waiting for either a Primary
// to appear or a SIGUSR1 from the operator; the operator picks the bootstrap
// pod from the logs and signals it.
//
//	pod side (pxc-entrypoint.sh)
//	  1. mysqld --wsrep_recover, or read grastate.dat directly
//	       -> uuid:seqno  (00000000-...:-1 if grastate has neither)
//	  2. peer-list: any peer already Primary?
//	       yes -> exec mysqld normally, done
//	       no  -> install SIGUSR1 trap (node_recovery:
//	                  rewrite wsrep_cluster_address to gcomm://,
//	                  flip safe_to_bootstrap=1, exec mysqld)
//	              log "...LAST_LINE:NODE:UUID:SEQNO:..." marker
//	              busy-wait for a Primary or SIGUSR1
//	                            |
//	                            | pod stdout
//	                            v
//	operator side (this file)
//	  recoverFullClusterCrashIfNeeded
//	    all pxc pods Running?           no  -> return
//	    pod-0 stuck on LAST_LINE?       no  -> return
//	                                    yes -> doFullCrashRecovery
//	  doFullCrashRecovery
//	    parse LAST_LINE on every pod (parseRecoveredPosition)
//	    any pod no longer waiting?     yes -> abort
//	    recoveryPod = pod with max seqno
//	    isAutomaticRecoverySafe(cr, uuid, seq)?
//	      no  ->  return an error and wait for human intervention
//	    exec "kill -s USR1 1" in recoveryPod -> entrypoint's node_recovery
//	    persist RecoveryStatus{uuid, seqno, pod, time}
//	    sleep 30s so the next reconcile doesn't re-signal the same pod
func (r *ReconcilePerconaXtraDBCluster) recoverFullClusterCrashIfNeeded(ctx context.Context, cr *pxcv1.PerconaXtraDBCluster) error {
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

	isWaiting, _, _, err := r.isPodWaitingForRecovery(cr.Namespace, cr.Name+"-pxc-0")
	if err != nil {
		return errors.Wrap(err, "failed to check if pxc pod 0 is waiting for recovery")
	}

	if isWaiting {
		return r.doFullCrashRecovery(ctx, cr)
	}

	return nil
}

const (
	invalidSeqno = -1
	// invalidUUID is the parser's "I could not determine the cluster UUID"
	// sentinel — used for legacy log lines (which had no UUID field) and parse
	// failures. It is intentionally distinct from the all-zeros UUID the
	// entrypoint emits when grastate.dat has no UUID, so the two cases can be
	// told apart.
	invalidUUID = ""
	// uninitializedUUID is what the PXC entrypoint writes in the LAST_LINE
	// marker when grastate.dat contains no UUID (fresh, never-bootstrapped
	// node). It is a real, expected value — not a parse error.
	uninitializedUUID = "00000000-0000-0000-0000-000000000000"
)

// isKnownUUID returns true when uuid identifies a specific cluster.
// "" (parser couldn't determine) and the all-zeros entrypoint sentinel
// (uninitialized grastate) are both treated as unknown.
func isKnownUUID(uuid string) bool {
	return uuid != invalidUUID && uuid != uninitializedUUID
}

func (r *ReconcilePerconaXtraDBCluster) isPodWaitingForRecovery(namespace, podName string) (bool, string, int64, error) {
	logOpts := &corev1.PodLogOptions{
		Container: "pxc",
		TailLines: &logLinesRequired,
	}
	logLines, err := r.clientcmd.PodLogs(namespace, podName, logOpts)
	if err != nil {
		return false, invalidUUID, invalidSeqno, errors.Wrapf(err, "get logs from %s pod", podName)
	}

	for i := len(logLines) - 1; i >= 0; i-- {
		if strings.HasPrefix(logLines[i], logPrefix) {
			uuid, seq, err := parseRecoveredPosition(logLines[i])
			return true, uuid, seq, err
		}
	}

	return false, invalidUUID, invalidSeqno, nil
}

// parseRecoveredPosition parses the full cluster crash recovery log in PXC container
// to get wsrep recovered position and return uuid:seqno.
//
// Two formats are accepted for backward compatibility with PXC images that
// predate the UUID being added to the marker:
//   - LAST_LINE:<node>:<seqno>:        (4 parts after split on ':')
//   - LAST_LINE:<node>:<uuid>:<seqno>: (5 parts after split on ':')
func parseRecoveredPosition(log string) (string, int64, error) {
	logsSplitted := strings.Split(log, ":")

	var uuid, seqStr string
	switch len(logsSplitted) {
	case 4:
		uuid, seqStr = invalidUUID, logsSplitted[2]
	case 5:
		uuid, seqStr = logsSplitted[2], logsSplitted[3]
	default:
		return invalidUUID, invalidSeqno, errors.New("invalid log format. Log: " + log)
	}

	seq, err := strconv.ParseInt(seqStr, 10, 64)
	if err != nil {
		return uuid, invalidSeqno, errors.Wrapf(err, "parse sequence %s", seqStr)
	}

	return uuid, seq, nil
}

type podRecoveryInfo struct {
	uuid string
	seq  int64
}

func (i podRecoveryInfo) String() string {
	return fmt.Sprintf("%s:%d", i.uuid, i.seq)
}

func (r *ReconcilePerconaXtraDBCluster) doFullCrashRecovery(ctx context.Context, cr *pxcv1.PerconaXtraDBCluster) error {
	maxSeq := int64(math.MinInt64)
	recoveryPod := ""
	podInfos := make(map[string]podRecoveryInfo, int(cr.Spec.PXC.Size))

	for i := range cr.Spec.PXC.Size {
		podName := fmt.Sprintf("%s-pxc-%d", cr.Name, i)
		isPodWaitingForRecovery, uuid, seq, err := r.isPodWaitingForRecovery(cr.Namespace, podName)
		if err != nil {
			return errors.Wrapf(err, "parse %s pod logs", podName)
		}

		if !isPodWaitingForRecovery {
			return nil
		}

		podInfos[podName] = podRecoveryInfo{uuid: uuid, seq: seq}

		if seq > maxSeq {
			maxSeq = seq
			recoveryPod = podName
		}
	}

	recoveryInfo := podInfos[recoveryPod]
	log := logf.FromContext(ctx).WithName("CrashRecovery")
	log.Info("We are in full cluster crash, starting recovery")
	log.Info("Results of scanning sequences", "pod", recoveryPod, "clusterUUID", recoveryInfo.uuid, "maxSeq", recoveryInfo.seq)
	r.recorder.Event(cr, corev1.EventTypeWarning, "FullClusterCrashDetected", "We are in full cluster crash")

	if !isAutomaticRecoverySafe(cr, recoveryInfo.uuid, recoveryInfo.seq) {
		lastRecoveryInfo := cr.Status.Recovery
		msg := fmt.Sprintf(
			"automatic recovery refused: cluster UUID or seqno regressed since last recovery (last uuid=%s, last seqno=%d; current uuid=%s, current seqno=%d). Follow the documented manual bootstrap/recovery procedure",
			lastRecoveryInfo.ClusterUUID,
			lastRecoveryInfo.LastRecoverySeqNo,
			recoveryInfo.uuid,
			recoveryInfo.seq,
		)
		r.recorder.Event(cr, corev1.EventTypeWarning, "AutomaticRecoveryRefused", msg)
		return errors.New(msg)
	}

	if recoveryInfo.uuid == invalidUUID {
		log.Info("Recovering from a pod that did not report a cluster UUID; future safety checks will rely on seqno only", "pod", recoveryPod)
	}

	pod := &corev1.Pod{}
	err := r.client.Get(ctx, types.NamespacedName{Name: recoveryPod, Namespace: cr.Namespace}, pod)
	if err != nil {
		return errors.Wrap(err, "get recovery pod")
	}

	stderrBuf := &bytes.Buffer{}
	err = r.clientcmd.Exec(pod, "pxc", []string{"/bin/sh", "-c", "kill -s USR1 1"}, nil, nil, stderrBuf, false)
	if err != nil {
		return errors.Wrap(err, "exec command in pod")
	}

	if stderrBuf.Len() != 0 {
		return errors.New("invalid exec command return: " + stderrBuf.String())
	}

	log.Info("Recovery started", "pod", pod.Name, "position", recoveryInfo)
	r.recorder.Event(cr, corev1.EventTypeNormal, "RecoveryStarted", fmt.Sprintf("Recovery started in %s, position %s", pod.Name, recoveryInfo))

	if err := updateRecoveryStatus(ctx, r.client, cr, recoveryInfo, recoveryPod); err != nil {
		log.Error(err, "update recovery status")
		// we already signalled the pod
		// not returning here so we can sleep
	}

	// sleep here a little to start recovery
	// and not send a lot of signals to the same pod
	time.Sleep(30 * time.Second)

	return nil
}

func isAutomaticRecoverySafe(cr *pxcv1.PerconaXtraDBCluster, uuid string, seqno int64) bool {
	// first recovery
	if cr.Status.Recovery == nil {
		return true
	}
	last := cr.Status.Recovery

	// When both sides have an identifiable UUID, require them to match —
	// auto-recovery across different clusters would risk data loss.
	// When either side is unknown (legacy log format, parse failure, or a
	// fresh/uninitialized cluster), fall back to the seqno regression check.
	if isKnownUUID(uuid) && isKnownUUID(last.ClusterUUID) && uuid != last.ClusterUUID {
		return false
	}

	// Equal seqno means no transactions committed since the last recovery —
	// recovering from the same point is identical to last time and does not
	// lose data.
	return last.LastRecoverySeqNo <= seqno
}

func (r *ReconcilePerconaXtraDBCluster) checkIfPodsRunning(cr *pxcv1.PerconaXtraDBCluster) error {
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

func updateRecoveryStatus(ctx context.Context, cl client.Client, cr *pxcv1.PerconaXtraDBCluster, info podRecoveryInfo, recoveryPod string) error {
	orig := cr.DeepCopy()

	cr.Status.Recovery = &pxcv1.RecoveryStatus{
		ClusterUUID:       info.uuid,
		LastRecoverySeqNo: info.seq,
		LastRecoveryPod:   recoveryPod,
		LastRecoveryTime:  metav1.Now(),
	}

	return cl.Status().Patch(ctx, cr, client.MergeFrom(orig))
}
