package backup

import (
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func (bcp *Backup) Scheduled(spec *api.PXCScheduledBackupSchedule, strg *api.BackupStorageSpec) *batchv1beta1.CronJob {
	jb := &batchv1beta1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1beta1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: bcp.namespace,
			Labels: map[string]string{
				"type":     "cron",
				"cluster":  bcp.cluster,
				"schedule": genScheduleLabel(spec.Schedule),
			},
		},
		Spec: batchv1beta1.CronJobSpec{
			Schedule:                   spec.Schedule,
			SuccessfulJobsHistoryLimit: func(i int32) *int32 { return &i }(1),
		},
	}

	jb.Spec.JobTemplate.ObjectMeta.Labels = map[string]string{
		"cluster": bcp.cluster,
		"type":    "xtrabackup",
	}
	jb.Spec.JobTemplate.Labels = jb.Labels
	jb.Spec.JobTemplate.Spec = bcp.scheduledJob(spec, strg)

	jb.Spec.JobTemplate.SetOwnerReferences(
		append(jb.Spec.JobTemplate.GetOwnerReferences(),
			metav1.OwnerReference{
				APIVersion: jb.APIVersion,
				Kind:       jb.Kind,
				Name:       jb.GetName(),
				UID:        jb.GetUID(),
			},
		),
	)

	return jb
}

func (bcp *Backup) scheduledJob(spec *api.PXCScheduledBackupSchedule, strg *api.BackupStorageSpec) batchv1.JobSpec {
	return batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				ServiceAccountName: bcp.serviceAccountName,
				Containers: []corev1.Container{
					{
						Name:  "run-backup",
						Image: bcp.image,
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
									  schedulerName: "` + spec.SchedulerName + `"
									  priorityClassName: "` + spec.PriorityClassName + `"
							EOF
							`,
						},
					},
				},
				RestartPolicy:    corev1.RestartPolicyNever,
				ImagePullSecrets: bcp.imagePullSecrets,
				NodeSelector:     strg.NodeSelector,
			},
		},
	}
}
