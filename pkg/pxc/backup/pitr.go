package backup

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/deployment"
)

func CheckPITRErrors(ctx context.Context, cl client.Client, clcmd *clientcmd.Client, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	if cr.Spec.Backup == nil || !cr.Spec.Backup.PITR.Enabled {
		return nil
	}

	backup, err := getLatestSuccessfulBackup(ctx, cl, cr)
	if err != nil {
		if errors.Is(err, ErrNoBackups) {
			return nil
		}
		return errors.Wrap(err, "get latest successful backup")
	}

	if cond := meta.FindStatusCondition(backup.Status.Conditions, api.BackupConditionPITRReady); cond != nil {
		if cond.Status == metav1.ConditionFalse {
			return nil
		}
	}

	err = cl.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: deployment.GetBinlogCollectorDeploymentName(cr)}, new(appsv1.Deployment))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "get binlog collector deployment")
	}

	collectorPod, err := deployment.GetBinlogCollectorPod(ctx, cl, cr)
	if err != nil {
		return errors.Wrap(err, "get binlog collector pod")
	}

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	err = clcmd.Exec(collectorPod, "pitr", []string{"/bin/bash", "-c", "cat /tmp/gap-detected"}, nil, stdoutBuf, stderrBuf, false)
	if err != nil {
		if strings.Contains(stderrBuf.String(), "No such file or directory") {
			return nil
		}
		return errors.Wrapf(err, "check binlog gaps in pod %s", collectorPod.Name)
	}

	if stdoutBuf.Len() == 0 {
		log.Info("Gap detected but GTID set is empty", "collector", collectorPod.Name)
		return nil
	}

	missingGTIDSet := stdoutBuf.String()
	log.Info("Gap detected in binary logs", "collector", collectorPod.Name, "missingGTIDSet", missingGTIDSet)

	condition := metav1.Condition{
		Type:               api.BackupConditionPITRReady,
		Status:             metav1.ConditionFalse,
		Reason:             "BinlogGapDetected",
		Message:            fmt.Sprintf("Binlog with GTID set %s not found", missingGTIDSet),
		LastTransitionTime: metav1.Now(),
	}
	meta.SetStatusCondition(&backup.Status.Conditions, condition)

	if err := cl.Status().Update(ctx, backup); err != nil {
		return errors.Wrap(err, "update backup status")
	}

	if err := deployment.RemoveGapFile(ctx, clcmd, collectorPod); err != nil {
		if !errors.Is(err, deployment.GapFileNotFound) {
			return errors.Wrap(err, "remove gap file")
		}
	}

	return nil
}

func UpdatePITRTimeline(ctx context.Context, cl client.Client, clcmd *clientcmd.Client, cr *api.PerconaXtraDBCluster) error {
	log := logf.FromContext(ctx)

	if cr.Spec.Backup == nil || !cr.Spec.Backup.PITR.Enabled {
		return nil
	}

	backup, err := getLatestSuccessfulBackup(ctx, cl, cr)
	if err != nil {
		if errors.Is(err, ErrNoBackups) {
			return nil
		}
		return errors.Wrap(err, "get latest successful backup")
	}

	err = cl.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: deployment.GetBinlogCollectorDeploymentName(cr)}, new(appsv1.Deployment))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "get binlog collector deployment")
	}

	collectorPod, err := deployment.GetBinlogCollectorPod(ctx, cl, cr)
	if err != nil {
		return errors.Wrap(err, "get binlog collector pod")
	}

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	err = clcmd.Exec(collectorPod, "pitr", []string{"/bin/bash", "-c", "cat /tmp/pitr-timeline"}, nil, stdoutBuf, stderrBuf, false)
	if err != nil {
		if strings.Contains(stderrBuf.String(), "No such file or directory") {
			return nil
		}
		return errors.Wrapf(err, "check binlog gaps in pod %s", collectorPod.Name)
	}

	timelines := strings.Split(stdoutBuf.String(), "\n")

	latest, err := strconv.ParseInt(timelines[1], 10, 64)
	if err != nil {
		return errors.Wrapf(err, "parse latest timeline %s", timelines[1])
	}
	latestTm := time.Unix(latest, 0)

	if backup.Status.LatestRestorableTime != nil && backup.Status.LatestRestorableTime.Time.Equal(latestTm) {
		return nil
	}

	backup.Status.LatestRestorableTime = &metav1.Time{Time: latestTm}

	if err := cl.Status().Update(ctx, backup); err != nil {
		return errors.Wrap(err, "update backup status")
	}

	log.Info("Updated PITR timelines", "latest", backup.Status.LatestRestorableTime, "lastBackup", backup.Name)

	return nil
}

var ErrNoBackups = errors.New("No backups found")

func getLatestSuccessfulBackup(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster) (*api.PerconaXtraDBClusterBackup, error) {
	bcpList := api.PerconaXtraDBClusterBackupList{}
	if err := cl.List(ctx, &bcpList, &client.ListOptions{Namespace: cr.Namespace}); err != nil {
		return nil, errors.Wrap(err, "get backup objects")
	}

	if len(bcpList.Items) == 0 {
		return nil, ErrNoBackups
	}

	latest := bcpList.Items[0]
	for _, bcp := range bcpList.Items {
		if bcp.Spec.PXCCluster != cr.Name || bcp.Status.State != api.BackupSucceeded {
			continue
		}

		if latest.ObjectMeta.CreationTimestamp.Before(&bcp.ObjectMeta.CreationTimestamp) {
			latest = bcp
		}
	}

	// if there are no successful backups, don't blindly return the first item
	if latest.Status.State != api.BackupSucceeded {
		return nil, ErrNoBackups
	}

	return &latest, nil
}
