package pxc

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const never = "never"
const disabled = "disabled"

func (r *ReconcilePerconaXtraDBCluster) deleteEnsureVersion(jobName string) {
	r.crons.crons.Remove(cron.EntryID(r.crons.jobs[jobName].ID))
	delete(r.crons.jobs, jobName)
}

func (r *ReconcilePerconaXtraDBCluster) sheduleEnsurePXCVersion(cr *api.PerconaXtraDBCluster, vs VersionService) error {
	jn := jobName(cr)
	schedule, ok := r.crons.jobs[jn]
	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType ||
		cr.Spec.UpgradeOptions.Schedule == "" ||
		strings.ToLower(cr.Spec.UpgradeOptions.Apply) == never ||
		strings.ToLower(cr.Spec.UpgradeOptions.Apply) == disabled {
		if ok {
			r.deleteEnsureVersion(jn)
		}
		return nil
	}

	if ok && schedule.CronShedule == cr.Spec.UpgradeOptions.Schedule {
		return nil
	}

	logger := r.logger(cr.Name, cr.Namespace)

	if ok {
		logger.Info("remove job because of new", "old", schedule.CronShedule, "new", cr.Spec.UpgradeOptions.Schedule)
		r.deleteEnsureVersion(jn)
	}

	nn := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}

	l := r.lockers.LoadOrCreate(nn.String())

	logger.Info("add new job", "schedule", cr.Spec.UpgradeOptions.Schedule)
	id, err := r.crons.crons.AddFunc(cr.Spec.UpgradeOptions.Schedule, func() {
		l.statusMutex.Lock()
		defer l.statusMutex.Unlock()

		if !atomic.CompareAndSwapInt32(l.updateSync, updateDone, updateWait) {
			return
		}

		localCr := &api.PerconaXtraDBCluster{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCr)
		if k8serrors.IsNotFound(err) {
			logger.Info("cluster is not found, deleting the job",
				"job name", jobName, "cluster", cr.Name, "namespace", cr.Namespace)
			r.deleteEnsureVersion(jn)
			return
		}
		if err != nil {
			logger.Error(err, "failed to get CR")
			return
		}

		if localCr.Status.Status != v1.AppStateReady {
			logger.Info("cluster is not ready")
			return
		}

		_, err = localCr.CheckNSetDefaults(r.serverVersion)
		if err != nil {
			logger.Error(err, "failed to set defaults for CR")
			return
		}

		err = r.ensurePXCVersion(localCr, vs)
		if err != nil {
			logger.Error(err, "failed to ensure version")
		}
	})
	if err != nil {
		return err
	}

	logger.Info("add new job", "name", jn, "schedule", cr.Spec.UpgradeOptions.Schedule)

	r.crons.jobs[jn] = Shedule{
		ID:          int(id),
		CronShedule: cr.Spec.UpgradeOptions.Schedule,
	}

	return nil
}

func jobName(cr *api.PerconaXtraDBCluster) string {
	jobName := "ensure-version"
	nn := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}
	return fmt.Sprintf("%s/%s", jobName, nn.String())
}

func (r *ReconcilePerconaXtraDBCluster) ensurePXCVersion(cr *api.PerconaXtraDBCluster, vs VersionService) error {
	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType ||
		cr.Spec.UpgradeOptions.Schedule == "" ||
		strings.ToLower(cr.Spec.UpgradeOptions.Apply) == never ||
		strings.ToLower(cr.Spec.UpgradeOptions.Apply) == disabled {
		return nil
	}

	if cr.Status.Status != v1.AppStateReady && cr.Status.PXC.Version != "" {
		return errors.New("cluster is not ready")
	}

	newVersion, err := vs.GetExactVersion(cr, cr.Spec.UpgradeOptions.VersionServiceEndpoint, versionMeta{
		Apply:               cr.Spec.UpgradeOptions.Apply,
		Platform:            string(cr.Spec.Platform),
		KubeVersion:         r.serverVersion.Info.GitVersion,
		PXCVersion:          cr.Status.PXC.Version,
		PMMVersion:          cr.Status.PMM.Version,
		HAProxyVersion:      cr.Status.HAProxy.Version,
		ProxySQLVersion:     cr.Status.ProxySQL.Version,
		BackupVersion:       cr.Status.Backup.Version,
		LogCollectorVersion: cr.Status.LogCollector.Version,
		CRUID:               string(cr.GetUID()),
	})
	if err != nil {
		return errors.Wrap(err, "failed to check version")
	}

	logger := r.logger(cr.Name, cr.Namespace)

	if cr.Spec.PXC != nil && cr.Spec.PXC.Image != newVersion.PXCImage {
		if cr.Status.PXC.Version == "" {
			logger.Info("set PXC version to " + newVersion.PXCVersion)
		} else {
			logger.Info("update PXC version", "old version", cr.Status.PXC.Version, "new version", newVersion.PXCVersion)
		}
		cr.Spec.PXC.Image = newVersion.PXCImage
	}

	if cr.Spec.Backup != nil && cr.Spec.Backup.Image != newVersion.BackupImage {
		if cr.Status.Backup.Version == "" {
			logger.Info("set Backup version to " + newVersion.BackupVersion)
		} else {
			logger.Info("update Backup version", "old version", cr.Status.Backup.Version, "new version", newVersion.BackupVersion)
		}
		cr.Spec.Backup.Image = newVersion.BackupImage
	}

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled && cr.Spec.PMM.Image != newVersion.PMMImage {
		if cr.Status.PMM.Version == "" {
			logger.Info("set PMM version to " + newVersion.PMMVersion)
		} else {
			logger.Info("update PMM version", "old version", cr.Status.PMM.Version, "new version", newVersion.PMMVersion)
		}
		cr.Spec.PMM.Image = newVersion.PMMImage
	}

	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled && cr.Spec.ProxySQL.Image != newVersion.ProxySqlImage {
		if cr.Status.ProxySQL.Version == "" {
			logger.Info("set ProxySQL version to " + newVersion.ProxySqlVersion)
		} else {
			logger.Info("update ProxySQL version", "old version", cr.Status.ProxySQL.Version, "new version", newVersion.ProxySqlVersion)
		}
		cr.Spec.ProxySQL.Image = newVersion.ProxySqlImage
	}

	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled && cr.Spec.HAProxy.Image != newVersion.HAProxyImage {
		if cr.Status.HAProxy.Version == "" {
			logger.Info("set HAProxy version to " + newVersion.HAProxyVersion)
		} else {
			logger.Info("update HAProxy version", "old version", cr.Status.HAProxy.Version, "new version", newVersion.HAProxyVersion)
		}
		cr.Spec.HAProxy.Image = newVersion.HAProxyImage
	}

	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Enabled && cr.Spec.LogCollector.Image != newVersion.LogCollectorImage {
		if cr.Status.LogCollector.Version == "" {
			logger.Info("set LogCollector version to " + newVersion.LogCollectorVersion)
		} else {
			logger.Info("update LogCollector version", "old version", cr.Status.LogCollector.Version, "new version", newVersion.LogCollectorVersion)
		}
		cr.Spec.LogCollector.Image = newVersion.LogCollectorImage
	}

	err = r.client.Update(context.Background(), cr)
	if err != nil {
		return errors.Wrap(err, "failed to update CR")
	}

	time.Sleep(1 * time.Second) // based on experiments operator just need it.

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cr)
	if err != nil {
		logger.Error(err, "failed to get CR")
	}

	cr.Status.ProxySQL.Version = newVersion.ProxySqlVersion
	cr.Status.HAProxy.Version = newVersion.HAProxyVersion
	cr.Status.PMM.Version = newVersion.PMMVersion
	cr.Status.Backup.Version = newVersion.BackupVersion
	cr.Status.PXC.Version = newVersion.PXCVersion
	cr.Status.PXC.Image = newVersion.PXCImage
	cr.Status.LogCollector.Version = newVersion.LogCollectorVersion

	err = r.client.Status().Update(context.Background(), cr)
	if err != nil {
		return errors.Wrap(err, "failed to update CR status")
	}

	time.Sleep(1 * time.Second)

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) fetchVersionFromPXC(cr *api.PerconaXtraDBCluster, sfs api.StatefulApp) error {
	if cr.Status.PXC.Status != api.AppStateReady {
		return nil
	}

	if cr.Status.ObservedGeneration != cr.ObjectMeta.Generation {
		return nil
	}

	if cr.Status.PXC.Image == cr.Spec.PXC.Image {
		return nil
	}

	upgradeInProgress, err := r.upgradeInProgress(cr, "pxc")
	if err != nil {
		return errors.Wrap(err, "check pxc upgrade progress")
	}
	if upgradeInProgress {
		return nil
	}

	list := corev1.PodList{}
	if err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     sfs.StatefulSet().Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
	); err != nil {
		return errors.Wrap(err, "get pod list")
	}

	user := "root"
	port := int32(3306)
	if cr.CompareVersionWith("1.6.0") >= 0 {
		port = int32(33062)
	}

	logger := r.logger(cr.Name, cr.Namespace)

	for _, pod := range list.Items {
		database, err := queries.New(r.client, cr.Namespace, cr.Spec.SecretsName, user, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, port)
		if err != nil {
			logger.Error(err, "failed to create db instance")
			continue
		}

		defer database.Close()

		version, err := database.Version()
		if err != nil {
			logger.Error(err, "failed to get pxc version")
			continue
		}

		logger.Info("update PXC version (fetched from db)", "new version", version)
		cr.Status.PXC.Version = version
		cr.Status.PXC.Image = cr.Spec.PXC.Image
		err = r.client.Status().Update(context.Background(), cr)
		if err != nil {
			return errors.Wrap(err, "failed to update CR")
		}

		return nil
	}

	return errors.New("failed to reach any pod")
}
