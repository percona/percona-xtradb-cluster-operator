package backup

import (
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

func (bcp *Backup) Scheduled(spec *api.PXCScheduledBackupSchedule, strg *api.BackupStorageSpec, cr *api.PerconaXtraDBCluster) *batchv1beta1.CronJob {
	// Copy from the original labels to the backup labels
	labels := make(map[string]string)
	for key, value := range strg.Labels {
		labels[key] = value
	}
	labels["type"] = "cron"
	labels["cluster"] = bcp.cluster
	labels["schedule"] = genScheduleLabel(spec.Schedule)

	jb := &batchv1beta1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1beta1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   bcp.namespace,
			Labels:      labels,
			Annotations: strg.Annotations,
		},
		Spec: batchv1beta1.CronJobSpec{
			Schedule:                   spec.Schedule,
			SuccessfulJobsHistoryLimit: func(i int32) *int32 { return &i }(1),
			JobTemplate: batchv1beta1.JobTemplateSpec{
				Spec: bcp.scheduledJob(spec, strg, labels),
			},
		},
	}

	jb.Spec.JobTemplate.SetOwnerReferences(
		append(jb.Spec.JobTemplate.GetOwnerReferences(),
			metav1.OwnerReference{
				APIVersion: cr.APIVersion,
				Kind:       cr.Kind,
				Name:       cr.GetName(),
				UID:        cr.GetUID(),
			},
		),
	)

	return jb
}

func (bcp *Backup) scheduledJob(spec *api.PXCScheduledBackupSchedule, strg *api.BackupStorageSpec, labels map[string]string) batchv1.JobSpec {
	resources, err := app.CreateResources(strg.Resources)
	if err != nil {
		log.Info("cannot parse backup resources: ", err)
	}

	return batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: strg.Annotations,
				Labels:      labels,
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: bcp.serviceAccountName,
				SecurityContext:    strg.PodSecurityContext,
				Containers: []corev1.Container{
					{
						Name:            "run-backup",
						Image:           bcp.image,
						ImagePullPolicy: corev1.PullAlways,
						Resources:       resources,
						SecurityContext: strg.ContainerSecurityContext,
						Env: []corev1.EnvVar{
							{
								Name:  "pxcCluster",
								Value: bcp.cluster,
							},
							{
								Name:  "suffix",
								Value: genRandString(5),
							},
						},
						Args: []string{
							"sh", "-c",
							`
							cat <<-EOF | kubectl apply -f -
									apiVersion: pxc.percona.com/v1
									kind: PerconaXtraDBClusterBackup
									metadata:
									  name: "cron-${pxcCluster:0:16}-$(date -u "+%Y%m%d%H%M%S")-${suffix}"
									  labels:
									    ancestor: "` + spec.Name + `"
									    cluster: "${pxcCluster}"
									    type: "cron"
									spec:
									  pxcCluster: "${pxcCluster}"
									  storageName: "` + spec.StorageName + `"
							EOF
							`,
						},
					},
				},
				RestartPolicy:     corev1.RestartPolicyNever,
				ImagePullSecrets:  bcp.imagePullSecrets,
				NodeSelector:      strg.NodeSelector,
				Affinity:          strg.Affinity,
				Tolerations:       strg.Tolerations,
				SchedulerName:     strg.SchedulerName,
				PriorityClassName: strg.PriorityClassName,
			},
		},
	}
}
