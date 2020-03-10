package users

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Job(cr *api.PerconaXtraDBCluster) *batchv1.Job {
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pxc-user-manager",
			Namespace: cr.Namespace,
		},
	}
}

func JobSpec(rootPass, conns, image string, job *batchv1.Job) batchv1.JobSpec {
	backbackoffLimit := int32(3)
	return batchv1.JobSpec{
		BackoffLimit: &backbackoffLimit,
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:            job.Name,
						Image:           image + "-docker",
						ImagePullPolicy: corev1.PullAlways,
						VolumeMounts: []corev1.VolumeMount{
							{
								MountPath: "/data",
								Name:      "userssecret",
								ReadOnly:  true,
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "PXC_SERVICE",
								Value: conns,
							},
							{
								Name:  "MYSQL_ROOT_PASSWORD",
								Value: rootPass,
							},
						},
						Command: []string{"user-manager"},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "userssecret",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "secret-for-users",
							},
						},
					},
				},
			},
		},
	}
}
