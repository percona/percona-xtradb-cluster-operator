package pxc

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/queries"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const jobName = "ensure-version"
const never = "Never"
const disabled = "Disabled"

func (r *ReconcilePerconaXtraDBCluster) deleteEnsureVersion(id int) {
	r.crons.crons.Remove(cron.EntryID(id))
	delete(r.crons.jobs, jobName)
}

func (r *ReconcilePerconaXtraDBCluster) sheduleEnsurePXCVersion(cr *api.PerconaXtraDBCluster, vs VersionService) error {
	schedule, ok := r.crons.jobs[jobName]
	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType ||
		cr.Spec.UpgradeOptions.Schedule == "" ||
		cr.Spec.UpgradeOptions.Apply == never ||
		cr.Spec.UpgradeOptions.Apply == disabled {
		if ok {
			r.deleteEnsureVersion(schedule.ID)
		}
		return nil
	}

	if ok && schedule.CronShedule == cr.Spec.UpgradeOptions.Schedule {
		return nil
	}

	if ok {
		log.Info(fmt.Sprintf("remove job %s because of new %s", schedule.CronShedule, cr.Spec.UpgradeOptions.Schedule))
		r.deleteEnsureVersion(schedule.ID)
	}

	log.Info(fmt.Sprintf("add new job: %s", cr.Spec.UpgradeOptions.Schedule))
	id, err := r.crons.crons.AddFunc(cr.Spec.UpgradeOptions.Schedule, func() {
		r.statusMutex.Lock()
		defer r.statusMutex.Unlock()

		if !atomic.CompareAndSwapInt32(&r.updateSync, updateDone, updateWait) {
			return
		}

		localCr := &api.PerconaXtraDBCluster{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCr)
		if err != nil {
			log.Error(err, "failed to get CR")
			return
		}

		if localCr.Status.Status != v1.AppStateReady {
			log.Info("cluster is not ready")
			return
		}

		err = r.ensurePXCVersion(localCr, vs)
		if err != nil {
			log.Error(err, "failed to ensure version")
		}
	})
	if err != nil {
		return err
	}

	r.crons.jobs[jobName] = Shedule{
		ID:          int(id),
		CronShedule: cr.Spec.UpgradeOptions.Schedule,
	}

	return nil
}

func (r *ReconcilePerconaXtraDBCluster) ensurePXCVersion(cr *api.PerconaXtraDBCluster, vs VersionService) error {
	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType ||
		cr.Spec.UpgradeOptions.Schedule == "" ||
		cr.Spec.UpgradeOptions.Apply == never ||
		cr.Spec.UpgradeOptions.Apply == disabled {
		return nil
	}

	if cr.Status.Status != v1.AppStateReady && cr.Status.PXC.Version != "" {
		return errors.New("cluster is not ready")
	}

	new, err := vs.GetExactVersion(cr.Spec.UpgradeOptions.Apply, cr.Status.PXC.Version)
	if err != nil {
		return fmt.Errorf("failed to check version: %v", err)
	}

	if cr.Status.PXC.Version != new.PXCVersion {
		log.Info(fmt.Sprintf("update PXC version to %v", new.PXCVersion))
		cr.Spec.PXC.Image = new.PXCImage
	}
	if cr.Status.Backup.Version != new.BackupVersion {
		log.Info(fmt.Sprintf("update Backup version to %v", new.BackupVersion))
		cr.Spec.Backup.Image = new.BackupImage
	}
	if cr.Status.PMM.Version != new.PMMVersion {
		log.Info(fmt.Sprintf("update PMM version to %v", new.PMMVersion))
		cr.Spec.PMM.Image = new.PMMImage
	}
	if cr.Status.ProxySQL.Version != new.ProxySqlVersion {
		log.Info(fmt.Sprintf("update ProxySQL version to %v", new.ProxySqlVersion))
		cr.Spec.ProxySQL.Image = new.ProxySqlImage
	}

	err = r.client.Update(context.Background(), cr)
	if err != nil {
		return fmt.Errorf("failed to update CR: %v", err)
	}

	time.Sleep(1 * time.Second) // based on experiments operator just need it.

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cr)
	if err != nil {
		log.Error(err, "failed to get CR")
	}

	cr.Status.ProxySQL.Version = new.ProxySqlVersion
	cr.Status.PMM.Version = new.PMMVersion
	cr.Status.Backup.Version = new.BackupVersion
	cr.Status.PXC.Version = new.PXCVersion
	cr.Status.PXC.Image = new.PXCImage

	err = r.client.Status().Update(context.Background(), cr)
	if err != nil {
		return fmt.Errorf("failed to update CR status: %v", err)
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

	if cr.Status.PXC.Version != "" &&
		cr.Status.PXC.Image == cr.Spec.PXC.Image {
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
		return fmt.Errorf("get pod list: %v", err)
	}

	user := "root"
	for _, pod := range list.Items {
		database, err := queries.New(r.client, cr.Namespace, cr.Spec.SecretsName, user, pod.Status.PodIP, 3306)
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

		log.Info(fmt.Sprintf("update PXC version to %v (fetched from db)", version))
		cr.Status.PXC.Version = version
		cr.Status.PXC.Image = cr.Spec.PXC.Image
		err = r.client.Status().Update(context.Background(), cr)
		if err != nil {
			return fmt.Errorf("failed to update CR: %v", err)
		}

		return nil
	}

	return fmt.Errorf("failed to reach any pod")
}
