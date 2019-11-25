package backup

import (
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

var log = logf.Log.WithName("backup/restore")

func PVCRestoreService(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace: bcp.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			},
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port: 3307,
					Name: "ncat",
				},
			},
		},
	}
}

func PVCRestorePod(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, pvcName string, cluster api.PerconaXtraDBClusterSpec) *corev1.Pod {
	if _, ok := cluster.Backup.Storages[bcp.Spec.StorageName]; !ok {
		log.Info("storage " + bcp.Spec.StorageName + " doesn't exist")
		cluster.Backup.Storages[bcp.Spec.StorageName] = &api.BackupStorageSpec{}
	}

	resources, err := app.CreateResources(cluster.Backup.Storages[bcp.Status.StorageName].Resources)
	if err != nil {
		log.Info("cannot parse backup resources: ", err)
	}

	// Copy from the original labels to the restore labels
	labels := make(map[string]string)
	for key, value := range cluster.Backup.Storages[bcp.Status.StorageName].Labels {
		labels[key] = value
	}
	labels["name"] = "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster

	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace:   bcp.Namespace,
			Annotations: cluster.Backup.Storages[bcp.Status.StorageName].Annotations,
			Labels:      labels,
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: cluster.Backup.ImagePullSecrets,
			Containers: []corev1.Container{
				{
					Name:            "ncat",
					Image:           cluster.Backup.Image,
					ImagePullPolicy: corev1.PullAlways,
					Command:         []string{"recovery-pvc-donor.sh"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "backup",
							MountPath: "/backup",
						},
						{
							Name:      "ssl",
							MountPath: "/etc/mysql/ssl",
						},
						{
							Name:      "ssl-internal",
							MountPath: "/etc/mysql/ssl-internal",
						},
					},
					Resources: resources,
				},
			},
			Volumes: []corev1.Volume{
				corev1.Volume{
					Name: "backup",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
				app.GetSecretVolumes("ssl-internal", cluster.PXC.SSLInternalSecretName, true),
				app.GetSecretVolumes("ssl", cluster.PXC.SSLSecretName, cluster.PXC.AllowUnsafeConfig),
			},
			RestartPolicy:     corev1.RestartPolicyAlways,
			NodeSelector:      cluster.Backup.Storages[bcp.Status.StorageName].NodeSelector,
			Affinity:          cluster.Backup.Storages[bcp.Status.StorageName].Affinity,
			Tolerations:       cluster.Backup.Storages[bcp.Status.StorageName].Tolerations,
			SchedulerName:     cluster.Backup.Storages[bcp.Status.StorageName].SchedulerName,
			PriorityClassName: cluster.Backup.Storages[bcp.Status.StorageName].PriorityClassName,
		},
	}
}

func PVCRestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster api.PerconaXtraDBClusterSpec) *batchv1.Job {
	resources, err := app.CreateResources(cluster.PXC.Resources)
	if err != nil {
		log.Info("cannot parse PXC resources: ", err)
	}

	jobPVC := corev1.Volume{
		Name: "datadir",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "datadir-" + bcp.Spec.PXCCluster + "-pxc-0",
			},
		},
	}

	jobPVCs := []corev1.Volume{
		jobPVC,
		app.GetSecretVolumes("ssl-internal", cluster.PXC.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", cluster.PXC.SSLSecretName, cluster.PXC.AllowUnsafeConfig),
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-job-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace: bcp.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cluster.PXC.Annotations,
					Labels:      cluster.PXC.Labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Backup.ImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Backup.Image,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"recovery-pvc-joiner.sh"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/datadir",
								},
								{
									Name:      "ssl",
									MountPath: "/etc/mysql/ssl",
								},
								{
									Name:      "ssl-internal",
									MountPath: "/etc/mysql/ssl-internal",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "RESTORE_SRC_SERVICE",
									Value: "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
								},
							},
							Resources: resources,
						},
					},
					RestartPolicy:     corev1.RestartPolicyNever,
					Volumes:           jobPVCs,
					NodeSelector:      cluster.PXC.NodeSelector,
					Affinity:          cluster.PXC.Affinity.Advanced,
					Tolerations:       cluster.PXC.Tolerations,
					SchedulerName:     cluster.PXC.SchedulerName,
					PriorityClassName: cluster.PXC.PriorityClassName,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}

	useMem, k8sq, err := xbMemoryUse(cluster)

	if useMem != "" && err == nil {
		job.Spec.Template.Spec.Containers[0].Env = append(
			job.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "XB_USE_MEMORY",
				Value: useMem,
			},
		)
		job.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: k8sq,
		}
	}

	return job
}

// S3RestoreJob returns restore job object for s3
func S3RestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster api.PerconaXtraDBClusterSpec) (*batchv1.Job, error) {
	resources, err := app.CreateResources(cluster.PXC.Resources)
	if err != nil {
		log.Info("cannot parse PXC resources: ", err)
	}

	if bcp.Status.S3 == nil {
		return nil, errors.New("nil s3 backup status")
	}

	jobPVC := corev1.Volume{
		Name: "datadir",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "datadir-" + bcp.Spec.PXCCluster + "-pxc-0",
			},
		},
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-job-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace: bcp.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cluster.PXC.Annotations,
					Labels:      cluster.PXC.Labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Backup.ImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Backup.Image,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"recovery-s3.sh"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/datadir",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "S3_BUCKET_URL",
									Value: s3dest,
								},
								{
									Name:  "ENDPOINT",
									Value: bcp.Status.S3.EndpointURL,
								},
								{
									Name:  "DEFAULT_REGION",
									Value: bcp.Status.S3.Region,
								},
								{
									Name: "ACCESS_KEY_ID",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: bcp.Status.S3.CredentialsSecret,
											},
											Key: "AWS_ACCESS_KEY_ID",
										},
									},
								},
								{
									Name: "SECRET_ACCESS_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: bcp.Status.S3.CredentialsSecret,
											},
											Key: "AWS_SECRET_ACCESS_KEY",
										},
									},
								},
							},
							Resources: resources,
						},
					},
					RestartPolicy:     corev1.RestartPolicyNever,
					Volumes:           []corev1.Volume{jobPVC},
					NodeSelector:      cluster.PXC.NodeSelector,
					Affinity:          cluster.PXC.Affinity.Advanced,
					Tolerations:       cluster.PXC.Tolerations,
					SchedulerName:     cluster.PXC.SchedulerName,
					PriorityClassName: cluster.PXC.PriorityClassName,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}

	useMem, k8sq, err := xbMemoryUse(cluster)

	if useMem != "" && err == nil {
		job.Spec.Template.Spec.Containers[0].Env = append(
			job.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "XB_USE_MEMORY",
				Value: useMem,
			},
		)
		job.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: k8sq,
		}
	}

	return job, nil
}

func xbMemoryUse(cluster api.PerconaXtraDBClusterSpec) (useMem string, k8sQuantity resource.Quantity, err error) {
	if cluster.PXC.Resources != nil {
		if cluster.PXC.Resources.Requests != nil {
			useMem = cluster.PXC.Resources.Requests.Memory
			k8sQuantity, err = resource.ParseQuantity(cluster.PXC.Resources.Requests.Memory)
		}

		if cluster.PXC.Resources.Limits != nil && cluster.PXC.Resources.Limits.Memory != "" {
			useMem = cluster.PXC.Resources.Limits.Memory
			k8sQuantity, err = resource.ParseQuantity(cluster.PXC.Resources.Limits.Memory)
		}

		// make the 90% value
		q := k8sQuantity.DeepCopy()
		q.Sub(*resource.NewQuantity(k8sQuantity.Value()/10, k8sQuantity.Format))
		useMem = q.String()
		// transform Gi/Mi/etc to G/M
		if strings.Contains(useMem, "i") {
			useMem = strings.Replace(useMem, "i", "", -1)
		}
	}

	return useMem, k8sQuantity, err
}
