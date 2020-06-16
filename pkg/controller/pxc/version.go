package pxc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

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

	new, err := vs.CheckNew()
	if err != nil {
		return fmt.Errorf("failed to check version: %v", err)
	}

	if cr.Status.PXC.Version != new.Versions.Matrix.Pxc["8.0.9.9"].Version {
		log.Info(fmt.Sprintf("update PXC version to %v", new.Versions.Matrix.Pxc["8.0.9.9"].Version))
		cr.Spec.PXC.Image = new.Versions.Matrix.Pxc["8.0.9.9"].Imagepath
		cr.Status.PXC.Version = new.Versions.Matrix.Pxc["8.0.9.9"].Version
	}
	if cr.Status.Backup.Version != new.Versions.Matrix.Backup["master"].Version {
		log.Info(fmt.Sprintf("update Backup version to %v", new.Versions.Matrix.Backup["master"].Version))
		cr.Spec.Backup.Image = new.Versions.Matrix.Backup["master"].Imagepath
		cr.Status.Backup.Version = new.Versions.Matrix.Backup["master"].Version
	}
	if cr.Status.PMM.Version != new.Versions.Matrix.Pmm["master"].Version {
		log.Info(fmt.Sprintf("update PMM version to %v", new.Versions.Matrix.Pmm["master"].Version))
		cr.Spec.PMM.Image = new.Versions.Matrix.Pmm["master"].Imagepath
		cr.Status.PMM.Version = new.Versions.Matrix.Pmm["master"].Version
	}
	if cr.Status.ProxySQL.Version != new.Versions.Matrix.Proxysql["master"].Version {
		log.Info(fmt.Sprintf("update PMM version to %v", new.Versions.Matrix.Proxysql["master"].Version))
		cr.Spec.ProxySQL.Image = new.Versions.Matrix.Proxysql["master"].Imagepath
		cr.Status.ProxySQL.Version = new.Versions.Matrix.Proxysql["master"].Version
	}

	err = r.client.Update(context.Background(), cr)
	if err != nil {
		return fmt.Errorf("failed to update CR: %v", err)
	}

	return nil
}

type VersionService interface {
	CheckNew() (VersionResponse, error)
}

type VersionServiceMock struct {
}

type Version struct {
	Version   string `json:"version"`
	Imagepath string `json:"imagepath"`
	Imagehash string `json:"imagehash"`
	Status    string `json:"status"`
	Critilal  bool   `json:"critilal"`
}

type VersionMatrix struct {
	Pxc      map[string]Version `json:"pxc"`
	Pmm      map[string]Version `json:"pmm"`
	Proxysql map[string]Version `json:"proxysql"`
	Backup   map[string]Version `json:"backup"`
}

type OperatorVersion struct {
	Operator string        `json:"operator"`
	Database string        `json:"database"`
	Matrix   VersionMatrix `json:"matrix"`
}

type VersionResponse struct {
	Versions OperatorVersion `json:"versions"`
}

func (vs VersionServiceMock) CheckNew() (VersionResponse, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://0.0.0.0:11000/api/versions/v1/pxc/1.5.0/8.0.9.9", nil)
	if err != nil {
		return VersionResponse{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return VersionResponse{}, err
	}

	defer resp.Body.Close()

	r := VersionResponse{}
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return VersionResponse{}, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return r, nil
}
