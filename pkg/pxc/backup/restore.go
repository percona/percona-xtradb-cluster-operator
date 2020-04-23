package backup

import (
	"strconv"
	"strings"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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

func PVCRestorePod(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, pvcName string, cluster api.PerconaXtraDBCluster) *corev1.Pod {
	if _, ok := cluster.Spec.Backup.Storages[bcp.Spec.StorageName]; !ok {
		log.Info("storage " + bcp.Spec.StorageName + " doesn't exist")
		if len(cluster.Spec.Backup.Storages) == 0 {
			cluster.Spec.Backup.Storages = map[string]*api.BackupStorageSpec{}
		}
		cluster.Spec.Backup.Storages[bcp.Spec.StorageName] = &api.BackupStorageSpec{}
	}

	resources, err := app.CreateResources(cluster.Spec.Backup.Storages[bcp.Status.StorageName].Resources)
	if err != nil {
		log.Info("cannot parse backup resources: ", err)
	}

	// Copy from the original labels to the restore labels
	labels := make(map[string]string)
	for key, value := range cluster.Spec.Backup.Storages[bcp.Status.StorageName].Labels {
		labels[key] = value
	}
	labels["name"] = "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace:   bcp.Namespace,
			Annotations: cluster.Spec.Backup.Storages[bcp.Status.StorageName].Annotations,
			Labels:      labels,
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
			SecurityContext:  cluster.Spec.Backup.Storages[bcp.Status.StorageName].PodSecurityContext,
			Containers: []corev1.Container{
				{
					Name:            "ncat",
					Image:           cluster.Spec.Backup.Image,
					ImagePullPolicy: corev1.PullAlways,
					Command:         []string{"recovery-pvc-donor.sh"},
					SecurityContext: cluster.Spec.Backup.Storages[bcp.Status.StorageName].ContainerSecurityContext,
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
						{
							Name:      "vault-keyring-secret",
							MountPath: "/etc/mysql/vault-keyring-secret",
						},
					},
					Resources: resources,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "backup",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
				app.GetSecretVolumes("ssl-internal", cluster.Spec.PXC.SSLInternalSecretName, true),
				app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, cluster.Spec.AllowUnsafeConfig),
				app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
			},
			RestartPolicy:     corev1.RestartPolicyAlways,
			NodeSelector:      cluster.Spec.Backup.Storages[bcp.Status.StorageName].NodeSelector,
			Affinity:          cluster.Spec.Backup.Storages[bcp.Status.StorageName].Affinity,
			Tolerations:       cluster.Spec.Backup.Storages[bcp.Status.StorageName].Tolerations,
			SchedulerName:     cluster.Spec.Backup.Storages[bcp.Status.StorageName].SchedulerName,
			PriorityClassName: cluster.Spec.Backup.Storages[bcp.Status.StorageName].PriorityClassName,
		},
	}

	if cluster.CompareVersionWith("1.5.0") >= 0 {
		pod.Spec.Containers[0].Command = []string{"/var/lib/mysql/opts/recovery-pvc-donor.sh"}
	}

	return pod
}

func PVCRestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster api.PerconaXtraDBCluster) *batchv1.Job {
	resources, err := app.CreateResources(cluster.Spec.PXC.Resources)
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
		app.GetSecretVolumes("ssl-internal", cluster.Spec.PXC.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, cluster.Spec.AllowUnsafeConfig),
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
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
					Annotations: cluster.Spec.PXC.Annotations,
					Labels:      cluster.Spec.PXC.Labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
					SecurityContext:  cluster.Spec.PXC.PodSecurityContext,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Spec.Backup.Image,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"recovery-pvc-joiner.sh"},
							SecurityContext: cluster.Spec.PXC.ContainerSecurityContext,
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
								{
									Name:      "vault-keyring-secret",
									MountPath: "/etc/mysql/vault-keyring-secret",
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
					NodeSelector:      cluster.Spec.PXC.NodeSelector,
					Affinity:          cluster.Spec.PXC.Affinity.Advanced,
					Tolerations:       cluster.Spec.PXC.Tolerations,
					SchedulerName:     cluster.Spec.PXC.SchedulerName,
					PriorityClassName: cluster.Spec.PXC.PriorityClassName,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}

	if cluster.CompareVersionWith("1.5.0") >= 0 {
		job.Spec.Template.Spec.Containers[0].Command = []string{"/var/lib/mysql/opts/recovery-pvc-joiner.sh"}
	}

	useMem, k8sq, err := xbMemoryUse(cluster.Spec)

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
func S3RestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster api.PerconaXtraDBCluster) (*batchv1.Job, error) {
	resources, err := app.CreateResources(cluster.Spec.PXC.Resources)
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

	jobPVCs := []corev1.Volume{
		jobPVC,
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
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
					Annotations: cluster.Spec.PXC.Annotations,
					Labels:      cluster.Spec.PXC.Labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
					SecurityContext:  cluster.Spec.PXC.PodSecurityContext,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Spec.Backup.Image,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"recovery-s3.sh"},
							SecurityContext: cluster.Spec.PXC.ContainerSecurityContext,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/datadir",
								},
								{
									Name:      "vault-keyring-secret",
									MountPath: "/etc/mysql/vault-keyring-secret",
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
					Volumes:           jobPVCs,
					NodeSelector:      cluster.Spec.PXC.NodeSelector,
					Affinity:          cluster.Spec.PXC.Affinity.Advanced,
					Tolerations:       cluster.Spec.PXC.Tolerations,
					SchedulerName:     cluster.Spec.PXC.SchedulerName,
					PriorityClassName: cluster.Spec.PXC.PriorityClassName,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}

	if cluster.CompareVersionWith("1.5.0") >= 0 {
		job.Spec.Template.Spec.Containers[0].Command = []string{"/var/lib/mysql/opts/recovery-s3.sh"}
	}

	useMem, k8sq, err := xbMemoryUse(cluster.Spec)

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
	var memory string

	if cluster.PXC.Resources != nil {
		if cluster.PXC.Resources.Requests != nil && cluster.PXC.Resources.Requests.Memory != "" {
			memory = cluster.PXC.Resources.Requests.Memory
		}

		if cluster.PXC.Resources.Limits != nil && cluster.PXC.Resources.Limits.Memory != "" {
			memory = cluster.PXC.Resources.Limits.Memory
		}

		k8sQuantity, err = resource.ParseQuantity(memory)
		if err != nil {
			return "", resource.Quantity{}, err
		}

		useMem75 := k8sQuantity.Value() / int64(100) * int64(75)
		useMem = strconv.FormatInt(useMem75, 10)

		// transform Gi/Mi/etc to G/M
		if strings.Contains(useMem, "i") {
			useMem = strings.Replace(useMem, "i", "", -1)
		}
	}

	return useMem, k8sQuantity, err
}
