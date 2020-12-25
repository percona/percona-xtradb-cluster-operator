package backup

import (
	"fmt"
	"strconv"
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

func PVCRestoreService(cr *api.PerconaXtraDBClusterRestore) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster,
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster,
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

func PVCRestorePod(cr *api.PerconaXtraDBClusterRestore, bcpStorageName, pvcName string, cluster api.PerconaXtraDBClusterSpec) (*corev1.Pod, error) {
	if _, ok := cluster.Backup.Storages[bcpStorageName]; !ok {
		log.Info("storage " + bcpStorageName + " doesn't exist")
		if len(cluster.Backup.Storages) == 0 {
			cluster.Backup.Storages = map[string]*api.BackupStorageSpec{}
		}
		cluster.Backup.Storages[bcpStorageName] = &api.BackupStorageSpec{}
	}

	resources, err := app.CreateResources(cluster.Backup.Storages[bcpStorageName].Resources)
	if err != nil {
		return nil, fmt.Errorf("cannot parse backup resources: %w", err)
	}

	// Copy from the original labels to the restore labels
	labels := make(map[string]string)
	for key, value := range cluster.Backup.Storages[bcpStorageName].Labels {
		labels[key] = value
	}
	labels["name"] = "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster

	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster,
			Namespace:   cr.Namespace,
			Annotations: cluster.Backup.Storages[bcpStorageName].Annotations,
			Labels:      labels,
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: cluster.Backup.ImagePullSecrets,
			SecurityContext:  cluster.Backup.Storages[bcpStorageName].PodSecurityContext,
			Containers: []corev1.Container{
				{
					Name:            "ncat",
					Image:           cluster.Backup.Image,
					ImagePullPolicy: cluster.Backup.ImagePullPolicy,
					Command:         []string{"recovery-pvc-donor.sh"},
					SecurityContext: cluster.Backup.Storages[bcpStorageName].ContainerSecurityContext,
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
				corev1.Volume{
					Name: "backup",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
				app.GetSecretVolumes("ssl-internal", cluster.PXC.SSLInternalSecretName, true),
				app.GetSecretVolumes("ssl", cluster.PXC.SSLSecretName, cluster.AllowUnsafeConfig),
				app.GetSecretVolumes("vault-keyring-secret", cluster.PXC.VaultSecretName, true),
			},
			RestartPolicy:      corev1.RestartPolicyAlways,
			NodeSelector:       cluster.Backup.Storages[bcpStorageName].NodeSelector,
			Affinity:           cluster.Backup.Storages[bcpStorageName].Affinity,
			Tolerations:        cluster.Backup.Storages[bcpStorageName].Tolerations,
			SchedulerName:      cluster.Backup.Storages[bcpStorageName].SchedulerName,
			PriorityClassName:  cluster.Backup.Storages[bcpStorageName].PriorityClassName,
			ServiceAccountName: cluster.PXC.ServiceAccountName,
		},
	}, nil
}

func PVCRestoreJob(cr *api.PerconaXtraDBClusterRestore, cluster api.PerconaXtraDBClusterSpec) (*batchv1.Job, error) {
	resources, err := app.CreateResources(cluster.PXC.Resources)
	if err != nil {
		return nil, fmt.Errorf("cannot parse PXC resources: %w", err)
	}

	jobPVC := corev1.Volume{
		Name: "datadir",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "datadir-" + cr.Spec.PXCCluster + "-pxc-0",
			},
		},
	}

	jobPVCs := []corev1.Volume{
		jobPVC,
		app.GetSecretVolumes("ssl-internal", cluster.PXC.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", cluster.PXC.SSLSecretName, cluster.AllowUnsafeConfig),
		app.GetSecretVolumes("vault-keyring-secret", cluster.PXC.VaultSecretName, true),
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-job-" + cr.Name + "-" + cr.Spec.PXCCluster,
			Namespace: cr.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cluster.PXC.Annotations,
					Labels:      cluster.PXC.Labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Backup.ImagePullSecrets,
					SecurityContext:  cluster.PXC.PodSecurityContext,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Backup.Image,
							ImagePullPolicy: cluster.Backup.ImagePullPolicy,
							Command:         []string{"recovery-pvc-joiner.sh"},
							SecurityContext: cluster.PXC.ContainerSecurityContext,
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
									Value: "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster,
								},
							},
							Resources: resources,
						},
					},
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes:            jobPVCs,
					NodeSelector:       cluster.PXC.NodeSelector,
					Affinity:           cluster.PXC.Affinity.Advanced,
					Tolerations:        cluster.PXC.Tolerations,
					SchedulerName:      cluster.PXC.SchedulerName,
					PriorityClassName:  cluster.PXC.PriorityClassName,
					ServiceAccountName: cluster.PXC.ServiceAccountName,
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

// S3RestoreJob returns restore job object for s3
func S3RestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster api.PerconaXtraDBClusterSpec, pitr bool) (*batchv1.Job, error) {
	resources, err := app.CreateResources(cluster.PXC.Resources)
	if err != nil {
		return nil, fmt.Errorf("cannot parse PXC resources: %w", err)
	}

	if bcp.Status.S3 == nil {
		return nil, errors.New("nil s3 backup status")
	}

	jobPVC := corev1.Volume{
		Name: "datadir",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "datadir-" + cr.Spec.PXCCluster + "-pxc-0",
			},
		},
	}

	jobPVCs := []corev1.Volume{
		jobPVC,
		app.GetSecretVolumes("vault-keyring-secret", cluster.PXC.VaultSecretName, true),
	}
	pxcUser := "xtrabackup"
	command := []string{"recovery-s3.sh"}

	envs := []corev1.EnvVar{
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
		{
			Name:  "PXC_SERVICE",
			Value: cr.Spec.PXCCluster + "-pxc",
		},
		{
			Name:  "PXC_USER",
			Value: pxcUser,
		},
		{
			Name: "PXC_PASS",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(cluster.SecretsName, pxcUser),
			},
		},
	}
	jobName := "restore-job-" + cr.Name + "-" + cr.Spec.PXCCluster
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "datadir",
			MountPath: "/datadir",
		},
		{
			Name:      "vault-keyring-secret",
			MountPath: "/etc/mysql/vault-keyring-secret",
		},
	}
	if pitr {
		bucket := ""
		if cluster.Backup == nil && len(cluster.Backup.Storages) == 0 {
			return nil, errors.New("no storage section")
		}
		storageS3 := api.BackupStorageS3Spec{}

		if len(cr.Spec.PITR.BackupSource.StorageName) > 0 {
			storage, ok := cluster.Backup.Storages[cr.Spec.PITR.BackupSource.StorageName]
			if ok {
				storageS3 = storage.S3
				bucket = storage.S3.Bucket
			}
		}
		if cr.Spec.PITR.BackupSource != nil && cr.Spec.PITR.BackupSource.S3 != nil {
			storageS3 = *cr.Spec.PITR.BackupSource.S3
			bucket = cr.Spec.PITR.BackupSource.S3.Bucket
		}

		if len(bucket) == 0 {
			return nil, errors.New("no backet in storage")
		}

		command = []string{"pitr", "recover"}
		envs = append(envs, corev1.EnvVar{
			Name:  "BINLOG_S3_ENDPOINT",
			Value: storageS3.EndpointURL,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "BINLOG_S3_REGION",
			Value: storageS3.Region,
		})
		envs = append(envs, corev1.EnvVar{
			Name: "BINLOG_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: storageS3.CredentialsSecret,
					},
					Key: "AWS_ACCESS_KEY_ID",
				},
			},
		})
		envs = append(envs, corev1.EnvVar{
			Name: "BINLOG_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: storageS3.CredentialsSecret,
					},
					Key: "AWS_SECRET_ACCESS_KEY",
				},
			},
		})

		envs = append(envs, corev1.EnvVar{
			Name:  "PITR_RECOVERY_TYPE",
			Value: cr.Spec.PITR.Type,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "BINLOG_S3_BUCKET_URL",
			Value: bucket,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "PITR_GTID_SET",
			Value: cr.Spec.PITR.GTIDSet,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "PITR_DATE",
			Value: cr.Spec.PITR.Date,
		})
		jobName = "pitr-job-" + cr.Name + "-" + cr.Spec.PXCCluster
		volumeMounts = []corev1.VolumeMount{}
		jobPVCs = []corev1.Volume{}
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cr.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cluster.PXC.Annotations,
					Labels:      cluster.PXC.Labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Backup.ImagePullSecrets,
					SecurityContext:  cluster.PXC.PodSecurityContext,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Backup.Image,
							ImagePullPolicy: cluster.Backup.ImagePullPolicy,
							Command:         command,
							SecurityContext: cluster.PXC.ContainerSecurityContext,
							VolumeMounts:    volumeMounts,
							Env:             envs,
							Resources:       resources,
						},
					},
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes:            jobPVCs,
					NodeSelector:       cluster.PXC.NodeSelector,
					Affinity:           cluster.PXC.Affinity.Advanced,
					Tolerations:        cluster.PXC.Tolerations,
					SchedulerName:      cluster.PXC.SchedulerName,
					PriorityClassName:  cluster.PXC.PriorityClassName,
					ServiceAccountName: cluster.PXC.ServiceAccountName,
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
