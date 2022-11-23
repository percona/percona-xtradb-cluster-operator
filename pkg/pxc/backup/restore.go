package backup

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

var log = logf.Log.WithName("backup/restore")

func PVCRestoreService(cr *api.PerconaXtraDBClusterRestore) *corev1.Service {
	svc := &corev1.Service{
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

	if cr.Annotations["percona.com/headless-service"] == "true" {
		svc.Spec.ClusterIP = corev1.ClusterIPNone
	}

	return svc
}

func PVCRestorePod(cr *api.PerconaXtraDBClusterRestore, bcpStorageName, pvcName string, cluster api.PerconaXtraDBClusterSpec) (*corev1.Pod, error) {
	if _, ok := cluster.Backup.Storages[bcpStorageName]; !ok {
		log.Info("storage " + bcpStorageName + " doesn't exist")
		if len(cluster.Backup.Storages) == 0 {
			cluster.Backup.Storages = map[string]*api.BackupStorageSpec{}
		}
		cluster.Backup.Storages[bcpStorageName] = &api.BackupStorageSpec{}
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
					Resources: cluster.Backup.Storages[bcpStorageName].Resources,
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
			RuntimeClassName:   cluster.Backup.Storages[bcpStorageName].RuntimeClassName,
		},
	}, nil
}

func PVCRestoreJob(cr *api.PerconaXtraDBClusterRestore, cluster api.PerconaXtraDBClusterSpec) (*batchv1.Job, error) {
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
							Resources: cluster.PXC.Resources,
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
					RuntimeClassName:   cluster.PXC.RuntimeClassName,
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

func AzureRestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster api.PerconaXtraDBClusterSpec, destination string, pitr bool) (*batchv1.Job, error) {
	if bcp.Status.Azure == nil {
		return nil, errors.New("nil azure storage backup status")
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
	command := []string{"recovery-cloud.sh"}

	verifyTLS := true
	if cluster.Backup != nil && len(cluster.Backup.Storages) > 0 {
		storage, ok := cluster.Backup.Storages[bcp.Spec.StorageName]
		if ok && storage.VerifyTLS != nil {
			verifyTLS = *storage.VerifyTLS
		}
	}
	azure := bcp.Status.Azure
	if azure == nil {
		return nil, errors.New("azure storage is not specified")
	}
	container, _ := azure.ContainerAndPrefix()
	envs := []corev1.EnvVar{
		{
			Name: "AZURE_STORAGE_ACCOUNT",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(azure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_NAME"),
			},
		},
		{
			Name: "AZURE_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(azure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_KEY"),
			},
		},
		{
			Name:  "AZURE_CONTAINER_NAME",
			Value: container,
		},
		{
			Name:  "AZURE_ENDPOINT",
			Value: azure.Endpoint,
		},
		{
			Name:  "AZURE_STORAGE_CLASS",
			Value: azure.StorageClass,
		},
		{
			Name:  "BACKUP_PATH",
			Value: strings.TrimPrefix(destination, container+"/"),
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
		{
			Name:  "VERIFY_TLS",
			Value: strconv.FormatBool(verifyTLS),
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
		if cluster.Backup == nil && len(cluster.Backup.Storages) == 0 {
			return nil, errors.New("no storage section")
		}
		storageAzure := new(api.BackupStorageAzureSpec)

		if len(cr.Spec.PITR.BackupSource.StorageName) > 0 {
			storage, ok := cluster.Backup.Storages[cr.Spec.PITR.BackupSource.StorageName]
			if ok {
				storageAzure = storage.Azure
			}
		}
		if cr.Spec.PITR.BackupSource != nil && cr.Spec.PITR.BackupSource.Azure != nil {
			storageAzure = cr.Spec.PITR.BackupSource.Azure
		}

		if len(storageAzure.ContainerPath) == 0 {
			return nil, errors.New("container name is not specified in storage")
		}

		command = []string{"pitr", "recover"}
		envs = append(envs, []corev1.EnvVar{
			{
				Name: "BINLOG_AZURE_STORAGE_ACCOUNT",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(storageAzure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_NAME"),
				},
			},
			{
				Name: "BINLOG_AZURE_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(storageAzure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_KEY"),
				},
			},
			{
				Name:  "BINLOG_AZURE_STORAGE_CLASS",
				Value: storageAzure.StorageClass,
			},
			{
				Name:  "BINLOG_AZURE_CONTAINER_PATH",
				Value: storageAzure.ContainerPath,
			},
			{
				Name:  "BINLOG_AZURE_ENDPOINT",
				Value: storageAzure.Endpoint,
			},
			{
				Name:  "PITR_RECOVERY_TYPE",
				Value: cr.Spec.PITR.Type,
			},
			{
				Name:  "PITR_GTID",
				Value: cr.Spec.PITR.GTID,
			},
			{
				Name:  "PITR_DATE",
				Value: cr.Spec.PITR.Date,
			},
			{
				Name:  "STORAGE_TYPE",
				Value: "azure",
			},
		}...)
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
							Resources:       cluster.PXC.Resources,
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
					RuntimeClassName:   cluster.PXC.RuntimeClassName,
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
func S3RestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster *api.PerconaXtraDBCluster, pitr bool) (*batchv1.Job, error) {
	if bcp.Status.S3 == nil {
		return nil, errors.New("nil s3 backup status storage")
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
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
	}
	pxcUser := "xtrabackup"
	command := []string{"recovery-cloud.sh"}
	if cluster.CompareVersionWith("1.12.0") < 0 {
		command = []string{"recovery-s3.sh"}
	}

	verifyTLS := true
	if cluster.Spec.Backup != nil && len(cluster.Spec.Backup.Storages) > 0 {
		storage, ok := cluster.Spec.Backup.Storages[bcp.Spec.StorageName]
		if ok && storage.VerifyTLS != nil {
			verifyTLS = *storage.VerifyTLS
		}
	}
	if bcp.Status.S3 == nil {
		return nil, errors.New("s3 storage is not specified")
	}
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
				SecretKeyRef: app.SecretKeySelector(cluster.Spec.SecretsName, pxcUser),
			},
		},
		{
			Name:  "VERIFY_TLS",
			Value: strconv.FormatBool(verifyTLS),
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
		if cluster.Spec.Backup == nil && len(cluster.Spec.Backup.Storages) == 0 {
			return nil, errors.New("no storage section")
		}
		storageS3 := new(api.BackupStorageS3Spec)

		if bs := cr.Spec.PITR.BackupSource; bs != nil && len(bs.StorageName) > 0 {
			storage, ok := cluster.Spec.Backup.Storages[cr.Spec.PITR.BackupSource.StorageName]
			if ok {
				storageS3 = storage.S3
				bucket = storage.S3.Bucket
			}
		}
		if cr.Spec.PITR.BackupSource != nil && cr.Spec.PITR.BackupSource.S3 != nil {
			storageS3 = cr.Spec.PITR.BackupSource.S3
			bucket = storageS3.Bucket
		}

		if len(bucket) == 0 {
			return nil, errors.New("no bucket in storage")
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
			Name:  "PITR_GTID",
			Value: cr.Spec.PITR.GTID,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "PITR_DATE",
			Value: cr.Spec.PITR.Date,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "STORAGE_TYPE",
			Value: "s3",
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
							ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
							Command:         command,
							SecurityContext: cluster.Spec.PXC.ContainerSecurityContext,
							VolumeMounts:    volumeMounts,
							Env:             envs,
							Resources:       cluster.Spec.PXC.Resources,
						},
					},
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes:            jobPVCs,
					NodeSelector:       cluster.Spec.PXC.NodeSelector,
					Affinity:           cluster.Spec.PXC.Affinity.Advanced,
					Tolerations:        cluster.Spec.PXC.Tolerations,
					SchedulerName:      cluster.Spec.PXC.SchedulerName,
					PriorityClassName:  cluster.Spec.PXC.PriorityClassName,
					ServiceAccountName: cluster.Spec.PXC.ServiceAccountName,
					RuntimeClassName:   cluster.Spec.PXC.RuntimeClassName,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
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
	if res := cluster.PXC.Resources; res.Size() > 0 {
		if _, ok := res.Requests[corev1.ResourceMemory]; ok {
			k8sQuantity = *res.Requests.Memory()
		}
		if _, ok := res.Limits[corev1.ResourceMemory]; ok {
			k8sQuantity = *res.Limits.Memory()
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
