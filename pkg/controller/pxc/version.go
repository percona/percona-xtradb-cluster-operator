package pxc

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/types"
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
	version := []string{"8.0.1.1", "8.0.1.2", "8.0.1.3"}[rand.Intn(3)]
	new, err := vs.Apply(version)
	if err != nil {
		return fmt.Errorf("failed to check version: %v", err)
	}

	if cr.Status.PXC.Version != new.PXCVersion {
		log.Info(fmt.Sprintf("update PXC version to %v", new.PXCVersion))
		cr.Spec.PXC.Image = new.PXCImage
		cr.Status.PXC.Version = new.PXCVersion
	}
	if cr.Status.Backup.Version != new.BackupVersion {
		log.Info(fmt.Sprintf("update Backup version to %v", new.BackupVersion))
		cr.Spec.Backup.Image = new.BackupImage
		cr.Status.Backup.Version = new.BackupVersion
	}
	if cr.Status.PMM.Version != new.PMMVersion {
		log.Info(fmt.Sprintf("update PMM version to %v", new.PMMVersion))
		cr.Spec.PMM.Image = new.PMMImage
		cr.Status.PMM.Version = new.PMMVersion
	}
	if cr.Status.ProxySQL.Version != new.ProxySqlVersion {
		log.Info(fmt.Sprintf("update ProxySQL version to %v", new.ProxySqlVersion))
		cr.Spec.ProxySQL.Image = new.ProxySqlImage
		cr.Status.ProxySQL.Version = new.ProxySqlVersion
	}

	err = r.client.Update(context.Background(), cr)
	if err != nil {
		return fmt.Errorf("failed to update CR: %v", err)
	}

	return nil
}
