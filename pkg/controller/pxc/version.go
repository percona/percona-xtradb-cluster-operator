package pxc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
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
	version := []string{"8.0.1.1", "8.0.1.2", "8.0.1.3"}[rand.Intn(3)]
	new, err := vs.Apply(version)
	if err != nil {
		return fmt.Errorf("failed to check version: %v", err)
	}

	if cr.Status.PXC.Version != new.Versions[0].Matrix.PXC[version].Version {
		log.Info(fmt.Sprintf("update PXC version to %v", new.Versions[0].Matrix.PXC[version].Version))
		cr.Spec.PXC.Image = new.Versions[0].Matrix.PXC[version].Imagepath
		cr.Status.PXC.Version = new.Versions[0].Matrix.PXC[version].Version
	}
	if cr.Status.Backup.Version != new.Versions[0].Matrix.Backup["master"].Version {
		log.Info(fmt.Sprintf("update Backup version to %v", new.Versions[0].Matrix.Backup["master"].Version))
		cr.Spec.Backup.Image = new.Versions[0].Matrix.Backup["master"].Imagepath
		cr.Status.Backup.Version = new.Versions[0].Matrix.Backup["master"].Version
	}
	if cr.Status.PMM.Version != new.Versions[0].Matrix.PMM["master"].Version {
		log.Info(fmt.Sprintf("update PMM version to %v", new.Versions[0].Matrix.PMM["master"].Version))
		cr.Spec.PMM.Image = new.Versions[0].Matrix.PMM["master"].Imagepath
		cr.Status.PMM.Version = new.Versions[0].Matrix.PMM["master"].Version
	}
	if cr.Status.ProxySQL.Version != new.Versions[0].Matrix.ProxySQL["master"].Version {
		log.Info(fmt.Sprintf("update PMM version to %v", new.Versions[0].Matrix.ProxySQL["master"].Version))
		cr.Spec.ProxySQL.Image = new.Versions[0].Matrix.ProxySQL["master"].Imagepath
		cr.Status.ProxySQL.Version = new.Versions[0].Matrix.ProxySQL["master"].Version
	}

	err = r.client.Update(context.Background(), cr)
	if err != nil {
		return fmt.Errorf("failed to update CR: %v", err)
	}

	return nil
}

type VersionService interface {
	Apply(string) (VersionResponse, error)
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
	PXC      map[string]Version `json:"pxc"`
	PMM      map[string]Version `json:"pmm"`
	ProxySQL map[string]Version `json:"proxysql"`
	Backup   map[string]Version `json:"backup"`
}

type OperatorVersion struct {
	Operator string        `json:"operator"`
	Database string        `json:"database"`
	Matrix   VersionMatrix `json:"matrix"`
}

type VersionResponse struct {
	Versions []OperatorVersion `json:"versions"`
}

func (vs VersionServiceMock) Apply(version string) (VersionResponse, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", "http://9a46fb98feeb.ngrok.io/api/versions/v1/pxc/1.5.0/"+version, nil)
	if err != nil {
		return VersionResponse{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return VersionResponse{}, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return VersionResponse{}, fmt.Errorf("received bad status code %s", resp.Status)
	}

	r := VersionResponse{}
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return VersionResponse{}, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return r, nil
}
