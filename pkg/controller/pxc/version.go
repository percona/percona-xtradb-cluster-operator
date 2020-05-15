package pxc

import (
	"context"
	"fmt"
	"math/rand"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/robfig/cron"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

const jobName = "ensure-version"

func (r *ReconcilePerconaXtraDBCluster) deleteEnsureVersion(id int) {
	r.crons.crons.Remove(cron.EntryID(id))
	delete(r.crons.jobs, jobName)
}

func (r *ReconcilePerconaXtraDBCluster) ensurePXCVersion(cr *api.PerconaXtraDBCluster, vs VersionService, sfs *statefulset.Node) error {
	shedule, ok := r.crons.jobs[jobName]

	if cr.Spec.UpgradeOptions.Schedule == "" {
		r.deleteEnsureVersion(shedule.ID)
		return nil
	}

	if ok && shedule.CronShedule == cr.Spec.UpgradeOptions.Schedule {
		return nil
	}

	log.Info(fmt.Sprintf("remove job %s because of new %s", shedule.CronShedule, cr.Spec.UpgradeOptions.Schedule))
	r.deleteEnsureVersion(shedule.ID)

	log.Info(fmt.Sprintf("add new job: %s", cr.Spec.UpgradeOptions.Schedule))
	id, err := r.crons.crons.AddFunc(cr.Spec.UpgradeOptions.Schedule, func() {
		sfsLocal := appsv1.StatefulSet{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: sfs.StatefulSet().Name, Namespace: sfs.StatefulSet().Namespace}, &sfsLocal)
		if err != nil {
			log.Error(err, "failed to get stateful set")
			return
		}

		if sfsLocal.Status.ReadyReplicas < sfsLocal.Status.Replicas ||
			sfsLocal.Status.CurrentRevision != sfsLocal.Status.UpdateRevision {
			log.Info("cluster is not consistent")
			return
		}

		localCR := &api.PerconaXtraDBCluster{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, localCR)
		if err != nil {
			log.Error(err, "failed to get CR")
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
