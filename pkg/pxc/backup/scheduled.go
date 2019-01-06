package backup

import (
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func NewScheduled(cr *api.PerconaXtraDBCluster, spec *api.PXCScheduledBackup) *batchv1beta1.CronJob {
	jb := &batchv1beta1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1beta1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"type":     "cron",
				"cluster":  cr.Name,
				"schedule": genScheduleLabel(spec.Schedule),
			},
		},
		Spec: batchv1beta1.CronJobSpec{
			Schedule:                   spec.Schedule,
			SuccessfulJobsHistoryLimit: func(i int32) *int32 { return &i }(1),
		},
	}

	jb.Spec.JobTemplate.ObjectMeta.Labels = map[string]string{
		"cluster": cr.Name,
		"type":    "xtrabackup",
	}
	jb.Spec.JobTemplate.Labels = jb.Labels
	jb.Spec.JobTemplate.Spec = scheduledJob(cr.Name, spec)

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

func scheduledJob(cluster string, spec *api.PXCScheduledBackup) batchv1.JobSpec {
	// originClusterName := cluster
	// if len(cluster) > 16 {
	// 	cluster = cluster[:16]
	// }

	env := []corev1.EnvVar{
		{
			Name:  "pxcCluster",
			Value: cluster,
		},
		{
			Name:  "size",
			Value: spec.Volume.Size,
		},
		{
			Name:  "suffix",
			Value: genRandString(5),
		},
	}

	if spec.Volume.StorageClass != nil {
		env = append(env, corev1.EnvVar{
			Name:  "storageClass",
			Value: *spec.Volume.StorageClass,
		},
		)
	}

	return batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "run-backup",
						Image: "delgod/kubectl:1.13.1",
						Env:   env,
						Args: []string{
							"sh", "-c",
							`
							cat <<-EOF | kubectl apply -f -
									apiVersion: pxc.percona.com/v1alpha1
									kind: PerconaXtraDBBackup
									metadata:
									  name: "cron-${pxcCluster:0:16}-$(date -u "+%Y%m%d%H%M%S")-${suffix}"
									  labels:
									    ancestor: "` + spec.Name + `"
									    cluster: "${pxcCluster}"
									    type: "cron"
									spec:
									  pxcCluster: "${pxcCluster}"
									  volume:
									    size: "${size}"
									    ${storageClass:+storageClass: "$storageClass"}
							EOF
							`,
						},
					},
				},
				RestartPolicy: corev1.RestartPolicyNever,
			},
		},
	}
}
