package naming

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util"
)

const (
	appKuberenetesPrefix = "app.kubernetes.io/"
	perconaPrefix        = "percona.com/"
)

const (
	LabelAppKubernetesInstance  = appKuberenetesPrefix + "instance"
	LabelAppKubernetesName      = appKuberenetesPrefix + "name"
	LabelAppKubernetesComponent = appKuberenetesPrefix + "component"
	LabelAppKubernetesManagedBy = appKuberenetesPrefix + "managed-by"
	LabelAppKubernetesPartOf    = appKuberenetesPrefix + "part-of"

	LabelOperatorVersion = appKuberenetesPrefix + "version"
)

const (
	LabelPerconaClusterName = perconaPrefix + "cluster"

	LabelPerconaBackupType         = perconaPrefix + "backup-type"
	LabelPerconaBackupName         = perconaPrefix + "backup-name"
	LabelPerconaBackupJobName      = perconaPrefix + "backup-job-name"
	LabelPerconaBackupAncestorName = perconaPrefix + "backup-ancestor"

	LabelPerconaRestoreServiceName = perconaPrefix + "restore-svc-name"
	LabelPerconaRestoreJobName     = perconaPrefix + "restore-job-name"
)

func GetLabelBackupType(cr *api.PerconaXtraDBCluster) string {
	if cr.CompareVersionWith("1.16.0") < 0 {
		return "type"
	}

	return LabelPerconaBackupType
}

func Labels() map[string]string {
	return map[string]string{
		LabelAppKubernetesName:   "percona-xtradb-cluster",
		LabelAppKubernetesPartOf: "percona-xtradb-cluster",
	}
}

func LabelsCluster(cr *api.PerconaXtraDBCluster) map[string]string {
	l := Labels()
	l[LabelAppKubernetesInstance] = cr.Name
	l[LabelAppKubernetesManagedBy] = "percona-xtradb-cluster-operator"

	return l
}

const (
	componentPITR            = "pitr"
	componentPXC             = "pxc"
	componentExternalService = "external-service"

	ComponentProxySQL = "proxysql"
	ComponentHAProxy  = "haproxy"
)

func componentLabels(cr *api.PerconaXtraDBCluster, component string) map[string]string {
	m := LabelsCluster(cr)
	m[LabelAppKubernetesComponent] = component
	return m
}

func LabelsPITR(cr *api.PerconaXtraDBCluster) map[string]string {
	return componentLabels(cr, componentPITR)
}

func LabelsProxySQL(cr *api.PerconaXtraDBCluster) map[string]string {
	return componentLabels(cr, ComponentProxySQL)
}

func LabelsHAProxy(cr *api.PerconaXtraDBCluster) map[string]string {
	return componentLabels(cr, ComponentHAProxy)
}

func LabelsPXC(cr *api.PerconaXtraDBCluster) map[string]string {
	return componentLabels(cr, componentPXC)
}

func LabelsRestorePVCPod(cr *api.PerconaXtraDBCluster, storageName string, restoreSvcName string) map[string]string {
	labels := make(map[string]string)
	if cr.Spec.Backup.Storages != nil && cr.Spec.Backup.Storages[storageName] != nil && len(cr.Spec.Backup.Storages[storageName].Labels) > 0 {
		util.MergeMaps(labels, cr.Spec.Backup.Storages[storageName].Labels)
	}

	if cr.CompareVersionWith("1.16.0") < 0 {
		labels["name"] = restoreSvcName
		return labels
	}

	util.MergeMaps(labels, LabelsCluster(cr), map[string]string{
		LabelPerconaRestoreServiceName: restoreSvcName,
	})
	return labels
}

func LabelsRestoreJob(cr *api.PerconaXtraDBCluster, jobName string, storageName string) map[string]string {
	if cr.CompareVersionWith("1.16.0") < 0 {
		return cr.Spec.PXC.Labels
	}

	labels := make(map[string]string)

	// TODO: should we add labels from storage or from .spec.pxc.labels ???
	if cr.Spec.Backup.Storages != nil && cr.Spec.Backup.Storages[storageName] != nil && len(cr.Spec.Backup.Storages[storageName].Labels) > 0 {
		util.MergeMaps(labels, cr.Spec.Backup.Storages[storageName].Labels)
	}

	util.MergeMaps(labels, LabelsCluster(cr), map[string]string{
		LabelPerconaRestoreJobName: jobName,
	})

	return labels
}

func LabelsScheduledBackup(cluster *api.PerconaXtraDBCluster, ancestor string) map[string]string {
	labels := make(map[string]string)

	if cluster.CompareVersionWith("1.16.0") < 0 {
		util.MergeMaps(labels, map[string]string{
			"ancestor": ancestor,
			"cluster":  cluster.Name,
			"type":     "cron",
		})
	} else {
		util.MergeMaps(labels, LabelsCluster(cluster), map[string]string{
			LabelPerconaBackupType:         "cron",
			LabelPerconaClusterName:        cluster.Name,
			LabelPerconaBackupAncestorName: ancestor,
		})
	}

	return labels
}

func LabelsBackup(cluster *api.PerconaXtraDBCluster) map[string]string {
	if cluster.CompareVersionWith("1.16.0") < 0 {
		return map[string]string{
			"type":    "xtrabackup",
			"cluster": cluster.Name,
		}
	}
	return map[string]string{
		LabelPerconaBackupType:  "xtrabackup",
		LabelPerconaClusterName: cluster.Name,
	}
}

func LabelsBackupJob(cr *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster, jobName string) map[string]string {
	labels := make(map[string]string)
	util.MergeMaps(labels, cluster.Spec.Backup.Storages[cr.Spec.StorageName].Labels)

	if cluster.CompareVersionWith("1.16.0") < 0 {
		util.MergeMaps(labels, LabelsBackup(cluster), map[string]string{
			"backup-name": cr.Name,
			"job-name":    jobName,
		})
		return labels
	}
	util.MergeMaps(labels, LabelsCluster(cluster), LabelsBackup(cluster), map[string]string{
		LabelPerconaBackupName:    cr.Name,
		LabelPerconaBackupJobName: jobName,
	})

	return labels
}

func LabelsExternalService(cr *api.PerconaXtraDBCluster) map[string]string {
	if cr.CompareVersionWith("1.16.0") < 0 {
		return map[string]string{
			LabelAppKubernetesName:      "percona-xtradb-cluster",
			LabelAppKubernetesInstance:  cr.Name,
			LabelAppKubernetesComponent: componentExternalService,
		}
	}
	return componentLabels(cr, componentExternalService)
}

func selector(cr *api.PerconaXtraDBCluster, component string) map[string]string {
	if cr.CompareVersionWith("1.16.0") < 0 {
		return map[string]string{
			LabelAppKubernetesName:      "percona-xtradb-cluster",
			LabelAppKubernetesInstance:  cr.Name,
			LabelAppKubernetesComponent: component,
		}
	}
	return componentLabels(cr, component)
}

func SelectorPXC(cr *api.PerconaXtraDBCluster) map[string]string {
	return selector(cr, componentPXC)
}

func SelectorHAProxy(cr *api.PerconaXtraDBCluster) map[string]string {
	return selector(cr, ComponentHAProxy)
}

func SelectorProxySQL(cr *api.PerconaXtraDBCluster) map[string]string {
	return selector(cr, ComponentProxySQL)
}
