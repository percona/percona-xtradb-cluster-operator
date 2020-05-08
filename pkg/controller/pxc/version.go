package pxc

import (
	"context"
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/robfig/cron"
)

func (r *ReconcilePerconaXtraDBCluster) ensureVersion(cr *api.PerconaXtraDBCluster, vs VersionService, sfs *statefulset.Node) error {
	if sfs.StatefulSet().Status.ReadyReplicas < sfs.StatefulSet().Status.Replicas ||
		sfs.StatefulSet().Status.CurrentRevision != sfs.StatefulSet().Status.UpdateRevision {
		return nil
	}

	jobName := "ensure-version"
	shedule, ok := r.crons.jobs[jobName]

	if ok {
		if shedule.CronShedule == cr.Spec.UpgradeOptions.Schedule {
			log.Info("same shedule")
			return nil
		}
		log.Info("remove job")
		r.crons.crons.Remove(cron.EntryID(shedule.ID))
		delete(r.crons.jobs, jobName)
	}

	var err error
	id, err := r.crons.crons.AddFunc(cr.Spec.UpgradeOptions.Schedule, func() {
		new := vs.CheckNew()
		if cr.Spec.PXC.Image != new {
			log.Info("update version")
			cr.Spec.PXC.Image = new
			err = r.client.Update(context.Background(), cr)
		} else {
			log.Info("same version")
		}
	})
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("add job: %s", cr.Spec.UpgradeOptions.Schedule))
	r.crons.jobs[jobName] = Shedule{
		ID:          int(id),
		CronShedule: cr.Spec.UpgradeOptions.Schedule,
	}

	return nil
}

type VersionService interface {
	CheckNew() string
}

type VersionServiceMock struct {
}

func (vs VersionServiceMock) CheckNew() string {
	return "perconalab/percona-xtradb-cluster-operator:master-pxc8.0"
}
