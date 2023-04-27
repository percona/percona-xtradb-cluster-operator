package pxc

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	apiv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/percona/percona-xtradb-cluster-operator/version"
)

var versionNotReadyErr = errors.New("not ready to fetch version")

func (r *ReconcilePerconaXtraDBCluster) deleteEnsureVersion(jobName string) {
	r.crons.crons.Remove(cron.EntryID(r.crons.ensureVersionJobs[jobName].ID))
	delete(r.crons.ensureVersionJobs, jobName)
}

func (r *ReconcilePerconaXtraDBCluster) scheduleEnsurePXCVersion(ctx context.Context, cr *apiv1.PerconaXtraDBCluster, vs VersionService) error {
	log := logf.FromContext(ctx)

	jn := jobName(cr)
	schedule, ok := r.crons.ensureVersionJobs[jn]
	if cr.Spec.UpgradeOptions.Schedule == "" || !(versionUpgradeEnabled(cr) || telemetryEnabled()) {
		if ok {
			r.deleteEnsureVersion(jn)
		}
		return nil
	}

	if ok && schedule.CronSchedule == cr.Spec.UpgradeOptions.Schedule {
		return nil
	}

	if ok {
		log.Info("remove job because of new", "old", schedule.CronSchedule, "new", cr.Spec.UpgradeOptions.Schedule)
		r.deleteEnsureVersion(jn)
	}

	nn := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}

	l := r.lockers.LoadOrCreate(nn.String())

	log.Info("add new job", "schedule", cr.Spec.UpgradeOptions.Schedule)
	id, err := r.crons.AddFuncWithSeconds(cr.Spec.UpgradeOptions.Schedule, func() {
		l.statusMutex.Lock()
		defer l.statusMutex.Unlock()

		if !atomic.CompareAndSwapInt32(l.updateSync, updateDone, updateWait) {
			return
		}

		localCr := &apiv1.PerconaXtraDBCluster{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCr)
		if k8serrors.IsNotFound(err) {
			log.Info("cluster is not found, deleting the job",
				"job name", jobName, "cluster", cr.Name, "namespace", cr.Namespace)
			r.deleteEnsureVersion(jn)
			return
		}
		if err != nil {
			log.Error(err, "failed to get CR")
			return
		}

		if localCr.Status.Status != apiv1.AppStateReady {
			log.Info("cluster is not ready")
			return
		}

		err = localCr.CheckNSetDefaults(r.serverVersion, log)
		if err != nil {
			log.Error(err, "failed to set defaults for CR")
			return
		}

		err = r.ensurePXCVersion(ctx, localCr, vs)
		if err != nil {
			log.Error(err, "failed to ensure version")
		}
	})
	if err != nil {
		return err
	}

	log.Info("add new job", "name", jn, "schedule", cr.Spec.UpgradeOptions.Schedule)

	r.crons.ensureVersionJobs[jn] = Schedule{
		ID:           int(id),
		CronSchedule: cr.Spec.UpgradeOptions.Schedule,
	}

	return nil
}

func jobName(cr *apiv1.PerconaXtraDBCluster) string {
	jobName := "ensure-version"
	nn := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}
	return fmt.Sprintf("%s/%s", jobName, nn.String())
}

func (r *ReconcilePerconaXtraDBCluster) ensurePXCVersion(ctx context.Context, cr *apiv1.PerconaXtraDBCluster, vs VersionService) error {
	log := logf.FromContext(ctx)

	if !(versionUpgradeEnabled(cr) || telemetryEnabled()) {
		return nil
	}

	if cr.Status.Status != apiv1.AppStateReady && cr.Status.PXC.Version != "" {
		return errors.New("cluster is not ready")
	}

	watchNs, err := k8s.GetWatchNamespace()
	if err != nil {
		return errors.Wrap(err, "get WATCH_NAMESPACE env variable")
	}

	vm := versionMeta{
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
		ClusterWideEnabled:  watchNs == "",
	}

	if telemetryEnabled() && (!versionUpgradeEnabled(cr) || cr.Spec.UpgradeOptions.VersionServiceEndpoint != apiv1.GetDefaultVersionServiceEndpoint()) {
		_, err := vs.GetExactVersion(cr, apiv1.GetDefaultVersionServiceEndpoint(), vm)
		if err != nil {
			log.Error(err, "failed to send telemetry to "+apiv1.GetDefaultVersionServiceEndpoint())
		}
	}

	if !versionUpgradeEnabled(cr) {
		return nil
	}

	newVersion, err := vs.GetExactVersion(cr, cr.Spec.UpgradeOptions.VersionServiceEndpoint, vm)
	if err != nil {
		return errors.Wrap(err, "failed to check version")
	}

	patch := client.MergeFrom(cr.DeepCopy())

	if cr.Spec.PXC != nil && cr.Spec.PXC.Image != newVersion.PXCImage {
		if cr.Status.PXC.Version == "" {
			log.Info("set PXC version to " + newVersion.PXCVersion)
		} else {
			log.Info("update PXC version", "old version", cr.Status.PXC.Version, "new version", newVersion.PXCVersion)
		}
		cr.Spec.PXC.Image = newVersion.PXCImage
	}

	if cr.Spec.Backup != nil && cr.Spec.Backup.Image != newVersion.BackupImage {
		if cr.Status.Backup.Version == "" {
			log.Info("set Backup version to " + newVersion.BackupVersion)
		} else {
			log.Info("update Backup version", "old version", cr.Status.Backup.Version, "new version", newVersion.BackupVersion)
		}
		cr.Spec.Backup.Image = newVersion.BackupImage
	}

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled && cr.Spec.PMM.Image != newVersion.PMMImage {
		if cr.Status.PMM.Version == "" {
			log.Info("set PMM version to " + newVersion.PMMVersion)
		} else {
			log.Info("update PMM version", "old version", cr.Status.PMM.Version, "new version", newVersion.PMMVersion)
		}
		cr.Spec.PMM.Image = newVersion.PMMImage
	}

	if cr.Spec.ProxySQLEnabled() && cr.Spec.ProxySQL.Image != newVersion.ProxySqlImage {
		if cr.Status.ProxySQL.Version == "" {
			log.Info("set ProxySQL version to " + newVersion.ProxySqlVersion)
		} else {
			log.Info("update ProxySQL version", "old version", cr.Status.ProxySQL.Version, "new version", newVersion.ProxySqlVersion)
		}
		cr.Spec.ProxySQL.Image = newVersion.ProxySqlImage
	}

	if cr.Spec.HAProxyEnabled() && cr.Spec.HAProxy.Image != newVersion.HAProxyImage {
		if cr.Status.HAProxy.Version == "" {
			log.Info("set HAProxy version to " + newVersion.HAProxyVersion)
		} else {
			log.Info("update HAProxy version", "old version", cr.Status.HAProxy.Version, "new version", newVersion.HAProxyVersion)
		}
		cr.Spec.HAProxy.Image = newVersion.HAProxyImage
	}

	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Enabled && cr.Spec.LogCollector.Image != newVersion.LogCollectorImage {
		if cr.Status.LogCollector.Version == "" {
			log.Info("set LogCollector version to " + newVersion.LogCollectorVersion)
		} else {
			log.Info("update LogCollector version", "old version", cr.Status.LogCollector.Version, "new version", newVersion.LogCollectorVersion)
		}
		cr.Spec.LogCollector.Image = newVersion.LogCollectorImage
	}

	err = r.client.Patch(context.Background(), cr.DeepCopy(), patch)
	if err != nil {
		return errors.Wrap(err, "failed to update CR")
	}

	cr.Status.ProxySQL.Version = newVersion.ProxySqlVersion
	cr.Status.HAProxy.Version = newVersion.HAProxyVersion
	cr.Status.PMM.Version = newVersion.PMMVersion
	cr.Status.Backup.Version = newVersion.BackupVersion
	cr.Status.PXC.Version = newVersion.PXCVersion
	cr.Status.PXC.Image = newVersion.PXCImage
	cr.Status.LogCollector.Version = newVersion.LogCollectorVersion

	err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		localCr := &apiv1.PerconaXtraDBCluster{}

		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCr)
		if err != nil {
			return err
		}

		localCr.Status = cr.Status

		return r.client.Status().Update(context.TODO(), localCr)
	})
	if err != nil {
		return errors.Wrap(err, "failed to update CR status")
	}

	time.Sleep(1 * time.Second)

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) mysqlVersion(ctx context.Context, cr *apiv1.PerconaXtraDBCluster, sfs apiv1.StatefulApp) (string, error) {
	log := logf.FromContext(ctx)

	if cr.Status.PXC.Ready < 1 {
		return "", versionNotReadyErr
	}

	if cr.Status.ObservedGeneration != cr.ObjectMeta.Generation {
		return "", versionNotReadyErr
	}

	if cr.Status.PXC.Image == cr.Spec.PXC.Image {
		return "", versionNotReadyErr
	}

	upgradeInProgress, err := r.upgradeInProgress(cr, "pxc")
	if err != nil {
		return "", errors.Wrap(err, "check pxc upgrade progress")
	}
	if upgradeInProgress {
		return "", versionNotReadyErr
	}

	list := corev1.PodList{}
	if err := r.client.List(context.TODO(),
		&list,
		&client.ListOptions{
			Namespace:     sfs.StatefulSet().Namespace,
			LabelSelector: labels.SelectorFromSet(sfs.Labels()),
		},
	); err != nil {
		return "", errors.Wrap(err, "get pod list")
	}

	user := "root"
	port := int32(3306)
	secrets := cr.Spec.SecretsName
	if cr.CompareVersionWith("1.6.0") >= 0 {
		port = int32(33062)
		secrets = "internal-" + cr.Name
	}

	for _, pod := range list.Items {
		if !isPodReady(pod) {
			continue
		}

		database, err := queries.New(r.client, cr.Namespace, secrets, user, pod.Name+"."+cr.Name+"-pxc."+cr.Namespace, port, cr.Spec.PXC.ReadinessProbes.TimeoutSeconds)
		if err != nil {
			log.Error(err, "failed to create db instance")
			continue
		}

		defer database.Close()

		version, err := database.Version()
		if err != nil {
			log.Error(err, "failed to get pxc version")
			continue
		}

		return version, nil
	}

	return "", errors.New("failed to reach any pod")
}

func (r *ReconcilePerconaXtraDBCluster) fetchVersionFromPXC(ctx context.Context, cr *apiv1.PerconaXtraDBCluster, sfs apiv1.StatefulApp) error {
	log := logf.FromContext(ctx)

	if cr.Status.PXC.Status != apiv1.AppStateReady {
		return nil
	}

	version, err := r.mysqlVersion(ctx, cr, sfs)
	if err != nil {
		if errors.Is(err, versionNotReadyErr) {
			return nil
		}

		return err
	}

	cr.Status.PXC.Version = version
	cr.Status.PXC.Image = cr.Spec.PXC.Image

	log.Info("update PXC version (fetched from db)", "new version", version)
	err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		localCr := &apiv1.PerconaXtraDBCluster{}

		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCr)
		if err != nil {
			return err
		}

		localCr.Status = cr.Status

		return r.client.Status().Update(context.TODO(), localCr)
	})
	if err != nil {
		return errors.Wrap(err, "failed to update CR")
	}
	return nil
}

func telemetryEnabled() bool {
	value, ok := os.LookupEnv("DISABLE_TELEMETRY")
	if ok {
		return value != "true"
	}
	return true
}

func versionUpgradeEnabled(cr *apiv1.PerconaXtraDBCluster) bool {
	return strings.ToLower(cr.Spec.UpgradeOptions.Apply) != apiv1.UpgradeStrategyNever &&
		strings.ToLower(cr.Spec.UpgradeOptions.Apply) != apiv1.UpgradeStrategyDisabled
}

// setCRVersion sets operator version of PerconaXtraDBCluster.
// The new (semver-matching) version is determined by the CR's crVersion field.
// If the crVersion is an empty string, it sets the current operator version.
func (r *ReconcilePerconaXtraDBCluster) setCRVersion(ctx context.Context, cr *apiv1.PerconaXtraDBCluster) error {
	if len(cr.Spec.CRVersion) > 0 {
		return nil
	}

	orig := cr.DeepCopy()
	cr.Spec.CRVersion = version.Version

	if err := r.client.Patch(ctx, cr, client.MergeFrom(orig)); err != nil {
		return errors.Wrap(err, "patch CR")
	}

	logf.FromContext(ctx).Info("Set CR version", "version", cr.Spec.CRVersion)

	return nil
}
