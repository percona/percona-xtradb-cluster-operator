package pxc

import (
	"context"
	"fmt"
	"math/rand"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/robfig/cron"
	"k8s.io/apimachinery/pkg/types"
)

const jobName = "ensure-version"

func (r *ReconcilePerconaXtraDBCluster) deleteEnsureVersion(id int) {
	r.crons.crons.Remove(cron.EntryID(id))
	delete(r.crons.jobs, jobName)
}

func (r *ReconcilePerconaXtraDBCluster) ensurePXCVersion(cr *api.PerconaXtraDBCluster, vs VersionService) error {
	if cr.Spec.UpdateStrategy != v1.SmartUpdateStatefulSetStrategyType {
		return nil
	}

	shedule, ok := r.crons.jobs[jobName]
	if ok && cr.Spec.UpgradeOptions.Schedule == "" {
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
		localCR := &api.PerconaXtraDBCluster{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCR)
		if err != nil {
			log.Error(err, "failed to get CR")
			return
		}

		if localCR.Status.Status != v1.AppStateReady {
			log.Info("cluster is not ready")
			return
		}

		new := vs.CheckNew()
		if localCR.Spec.PXC.Image != new {
			log.Info(fmt.Sprintf("update version to %s", new))
			localCR.Spec.PXC.Image = new
			err = r.client.Update(context.Background(), localCR)
			if err != nil {
				log.Error(err, "failed to update CR")
				return
			}
		} else {
			log.Info(fmt.Sprintf("same version %s", new))
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

type VersionService interface {
	CheckNew() string
}

type VersionServiceMock struct {
}

func (vs VersionServiceMock) CheckNew() string {
	if rand.Int()%2 == 0 {
		return "perconalab/percona-xtradb-cluster-operator:master-pxc8.0"
	}
	return "percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0"
}
