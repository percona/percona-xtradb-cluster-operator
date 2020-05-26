package pxc

import (
	"context"
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
	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType ||
		cr.Spec.UpgradeOptions.Schedule == "" ||
		cr.Spec.UpgradeOptions.Apply == never ||
		cr.Spec.UpgradeOptions.Apply == disabled {
		return nil
	}

	shedule, ok := r.crons.jobs[jobName]
	if ok && (cr.Spec.UpgradeOptions.Schedule == "" ||
		cr.Spec.UpgradeOptions.Apply == never ||
		cr.Spec.UpgradeOptions.Apply == disabled) {
		r.deleteEnsureVersion(shedule.ID)
		return nil
	}
	if ok && shedule.CronShedule == cr.Spec.UpgradeOptions.Schedule {
		return nil
	}
	if ok {
		log.Info(fmt.Sprintf("remove job %s because of new %s", shedule.CronShedule, cr.Spec.UpgradeOptions.Schedule))
		r.deleteEnsureVersion(shedule.ID)
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

		r.ensurePXCVersion(localCr, vs)
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

func (r *ReconcilePerconaXtraDBCluster) ensurePXCVersion(cr *api.PerconaXtraDBCluster, vs VersionService) {
	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType ||
		cr.Spec.UpgradeOptions.Schedule == "" ||
		cr.Spec.UpgradeOptions.Apply == never ||
		cr.Spec.UpgradeOptions.Apply == disabled {
		return
	}

	if cr.Status.Status != v1.AppStateReady && cr.Status.PXC.Version != "" {
		log.Info("cluster is not ready")
		return
	}

	new := vs.CheckNew()

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
		log.Info(fmt.Sprintf("update PMM version to %v", new.PMMVersion))
		cr.Spec.ProxySQL.Image = new.ProxySqlImage
		cr.Status.ProxySQL.Version = new.ProxySqlVersion
	}

	err := r.client.Update(context.Background(), cr)
	if err != nil {
		log.Error(err, "failed to update CR")
		return
	}
}

type VersionService interface {
	CheckNew() VersionResponse
}

type VersionServiceMock struct {
}

func (vs VersionServiceMock) CheckNew() VersionResponse {
	vr := VersionResponse{
		PXCImage:      "percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0",
		PXCVersion:    "8.0.18-9.3",
		BackupImage:   "perconalab/percona-xtradb-cluster-operator:master-pxc8.0",
		PMMImage:      "perconalab/percona-xtradb-cluster-operator:master-pmm",
		ProxySqlImage: "perconalab/percona-xtradb-cluster-operator:master-proxysql",
	}

	if rand.Int()%2 == 0 {
		vr.PXCImage = "perconalab/percona-xtradb-cluster-operator:master-pxc8.0"
		vr.PXCVersion = "8.0.18-9.4"
	}

	return vr
}

type VersionResponse struct {
	PXCImage        string `json:"pxcImage,omitempty"`
	PXCVersion      string `json:"pxcVersion,omitempty"`
	BackupImage     string `json:"backupImage,omitempty"`
	BackupVersion   string `json:"backupVersion,omitempty"`
	ProxySqlImage   string `json:"proxySqlImage,omitempty"`
	ProxySqlVersion string `json:"proxySqlVersion,omitempty"`
	PMMImage        string `json:"pmmImage,omitempty"`
	PMMVersion      string `json:"pmmVersion,omitempty"`
}
