package users

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("users-manager")

func Job(cr *api.PerconaXtraDBCluster, jobName, secretHash string) *batchv1.Job {
	labels := make(map[string]string)
	for key, value := range cr.Spec.Users.Labels {
		labels[key] = value
	}

	labels["app.kubernetes.io/name"] = "percona-xtradb-cluster"
	labels["app.kubernetes.io/instance"] = cr.Name
	labels["app.kubernetes.io/component"] = "user-manager"
	labels["app.kubernetes.io/managed-by"] = "percona-xtradb-cluster-operator"
	labels["app.kubernetes.io/part-of"] = "percona-xtradb-cluster"
	labels["job-name"] = jobName

	annotations := make(map[string]string)
	for key, value := range cr.Spec.Users.Annotations {
		annotations[key] = value
	}
	annotations["secret-hash"] = secretHash

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        labels["job-name"],
			Labels:      labels,
			Annotations: annotations,
			Namespace:   cr.Namespace,
		},
	}
}

func JobSpec(secretName, image string, job *batchv1.Job, cr *api.PerconaXtraDBCluster, imagePullSecrets []corev1.LocalObjectReference) batchv1.JobSpec {
	resources, err := app.CreateResources(cr.Spec.Users.Resources)
	if err != nil {
		log.Info("cannot parse users resources: ", err)
	}
	backbackoffLimit := int32(3)
	return batchv1.JobSpec{
		BackoffLimit: &backbackoffLimit,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      job.Labels,
				Annotations: cr.Spec.Users.Annotations,
			},
			Spec: corev1.PodSpec{
				RestartPolicy:    corev1.RestartPolicyNever,
				ImagePullSecrets: imagePullSecrets,
				Containers: []corev1.Container{
					{
						Name:            job.Name,
						Image:           image,
						SecurityContext: cr.Spec.Users.ContainerSecurityContext,
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
								Value: cr.Name + "-pxc",
							},
							{
								Name:  "PROXY_SERVICE",
								Value: cr.Name + "-proxysql",
							},
							{
								Name: "MYSQL_ROOT_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: app.SecretKeySelector(cr.Spec.SecretsName, "root"),
								},
							},
							{
								Name: "PROXY_ADMIN_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: app.SecretKeySelector(cr.Spec.SecretsName, "proxyadmin"),
								},
							},
						},
						Command:   []string{"user-manager"},
						Resources: resources,
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "userssecret",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: secretName,
							},
						},
					},
				},
				Affinity:          cr.Spec.Users.Affinity,
				Tolerations:       cr.Spec.Users.Tolerations,
				NodeSelector:      cr.Spec.Users.NodeSelector,
				SchedulerName:     cr.Spec.Users.SchedulerName,
				PriorityClassName: cr.Spec.Users.PriorityClassName,
				SecurityContext:   cr.Spec.Users.PodSecurityContext,
			},
		},
	}
}
