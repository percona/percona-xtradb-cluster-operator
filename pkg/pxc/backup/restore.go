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
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
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

func PVCRestorePod(cr *api.PerconaXtraDBClusterRestore, bcpStorageName, pvcName string, cluster *api.PerconaXtraDBCluster) (*corev1.Pod, error) {
	if _, ok := cluster.Spec.Backup.Storages[bcpStorageName]; !ok {
		log.Info("storage " + bcpStorageName + " doesn't exist")
		if len(cluster.Spec.Backup.Storages) == 0 {
			cluster.Spec.Backup.Storages = map[string]*api.BackupStorageSpec{}
		}
		cluster.Spec.Backup.Storages[bcpStorageName] = &api.BackupStorageSpec{}
	}

	// Copy from the original labels to the restore labels
	labels := make(map[string]string)
	for key, value := range cluster.Spec.Backup.Storages[bcpStorageName].Labels {
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
			Annotations: cluster.Spec.Backup.Storages[bcpStorageName].Annotations,
			Labels:      labels,
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
			SecurityContext:  cluster.Spec.Backup.Storages[bcpStorageName].PodSecurityContext,
			Containers: []corev1.Container{
				{
					Name:            "ncat",
					Image:           cluster.Spec.Backup.Image,
					ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
					Command:         []string{"recovery-pvc-donor.sh"},
					SecurityContext: cluster.Spec.Backup.Storages[bcpStorageName].ContainerSecurityContext,
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
					Resources: cr.Spec.Resources,
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
			RestartPolicy:      corev1.RestartPolicyAlways,
			NodeSelector:       cluster.Spec.Backup.Storages[bcpStorageName].NodeSelector,
			Affinity:           cluster.Spec.Backup.Storages[bcpStorageName].Affinity,
			Tolerations:        cluster.Spec.Backup.Storages[bcpStorageName].Tolerations,
			SchedulerName:      cluster.Spec.Backup.Storages[bcpStorageName].SchedulerName,
			PriorityClassName:  cluster.Spec.Backup.Storages[bcpStorageName].PriorityClassName,
			ServiceAccountName: cluster.Spec.PXC.ServiceAccountName,
			RuntimeClassName:   cluster.Spec.Backup.Storages[bcpStorageName].RuntimeClassName,
		},
	}, nil
}

func PVCRestoreJob(cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster, bcp *api.PerconaXtraDBClusterBackup) (*batchv1.Job, error) {
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
		app.GetSecretVolumes("ssl-internal", cluster.Spec.PXC.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, cluster.Spec.AllowUnsafeConfig),
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
	}
	command := []string{"recovery-pvc-joiner.sh"}
	volumeMounts := []corev1.VolumeMount{
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
	}
	envs := []corev1.EnvVar{
		{
			Name:  "RESTORE_SRC_SERVICE",
			Value: "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster,
		},
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
					Annotations: cluster.Spec.PXC.Annotations,
					Labels:      cluster.Spec.PXC.Labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
					SecurityContext:  cluster.Spec.PXC.PodSecurityContext,
					Containers: []corev1.Container{
						xtrabackupContainer(cr, cluster, command, volumeMounts, envs),
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

	return job, nil
}

func AzureRestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster, destination string, pitr bool) (*batchv1.Job, error) {
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
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
	}
	pxcUser := users.Xtrabackup
	command := []string{"recovery-cloud.sh"}

	verifyTLS := true
	if cluster.Spec.Backup != nil && len(cluster.Spec.Backup.Storages) > 0 {
		storage, ok := cluster.Spec.Backup.Storages[bcp.Spec.StorageName]
		if ok && storage.VerifyTLS != nil {
			verifyTLS = *storage.VerifyTLS
		}
	}
	if bcp.Status.VerifyTLS != nil {
		verifyTLS = *bcp.Status.VerifyTLS
	}
	azure := bcp.Status.Azure
	if azure == nil {
		return nil, errors.New("azure storage is not specified")
	}
	container, _ := azure.ContainerAndPrefix()
	destination = strings.TrimPrefix(destination, api.AzureBlobStoragePrefix+container+"/")
	destination = strings.TrimPrefix(destination, container+"/")
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
			Value: destination,
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
		if cluster.Spec.Backup == nil && len(cluster.Spec.Backup.Storages) == 0 {
			return nil, errors.New("no storage section")
		}
		storageAzure := new(api.BackupStorageAzureSpec)

		if len(cr.Spec.PITR.BackupSource.StorageName) > 0 {
			storage, ok := cluster.Spec.Backup.Storages[cr.Spec.PITR.BackupSource.StorageName]
			if ok {
				storageAzure = storage.Azure
			}
			if ok && storage.VerifyTLS != nil {
				verifyTLS = *storage.VerifyTLS
			}
		}
		if cr.Spec.PITR.BackupSource != nil && cr.Spec.PITR.BackupSource.Azure != nil {
			storageAzure = cr.Spec.PITR.BackupSource.Azure
			if cr.Spec.PITR.BackupSource.VerifyTLS != nil {
				verifyTLS = *cr.Spec.PITR.BackupSource.VerifyTLS
			}
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
	envs = append(envs, corev1.EnvVar{
		Name:  "VERIFY_TLS",
		Value: strconv.FormatBool(verifyTLS),
	})
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
					ImagePullSecrets:   cluster.Spec.Backup.ImagePullSecrets,
					SecurityContext:    cluster.Spec.PXC.PodSecurityContext,
					Containers:         []corev1.Container{xtrabackupContainer(cr, cluster, command, volumeMounts, envs)},
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
	pxcUser := users.Xtrabackup
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
	if bcp.Status.VerifyTLS != nil {
		verifyTLS = *bcp.Status.VerifyTLS
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
				if storage.VerifyTLS != nil {
					verifyTLS = *storage.VerifyTLS
				}
			}
		}
		if cr.Spec.PITR.BackupSource != nil {
			if cr.Spec.PITR.BackupSource.VerifyTLS != nil {
				verifyTLS = *cr.Spec.PITR.BackupSource.VerifyTLS
			}
			if cr.Spec.PITR.BackupSource.S3 != nil {
				storageS3 = cr.Spec.PITR.BackupSource.S3
				bucket = storageS3.Bucket
			}
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

	envs = append(envs, corev1.EnvVar{
		Name:  "VERIFY_TLS",
		Value: strconv.FormatBool(verifyTLS),
	})

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
					ImagePullSecrets:   cluster.Spec.Backup.ImagePullSecrets,
					SecurityContext:    cluster.Spec.PXC.PodSecurityContext,
					Containers:         []corev1.Container{xtrabackupContainer(cr, cluster, command, volumeMounts, envs)},
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

	return job, nil
}

func xtrabackupContainer(cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster, cmd []string, volumeMounts []corev1.VolumeMount, envs []corev1.EnvVar) corev1.Container {
	container := corev1.Container{
		Name:            "xtrabackup",
		Image:           cluster.Spec.Backup.Image,
		ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
		Command:         cmd,
		SecurityContext: cluster.Spec.PXC.ContainerSecurityContext,
		VolumeMounts:    volumeMounts,
		Env:             envs,
		Resources:       *cr.Spec.Resources.DeepCopy(),
	}
	if cluster.CompareVersionWith("1.13.0") < 0 {
		container.Resources = cluster.Spec.PXC.Resources
	}

	useMem, k8sq := xbMemoryUse(container.Resources)
	container.Env = append(
		container.Env,
		corev1.EnvVar{
			Name:  "XB_USE_MEMORY",
			Value: useMem,
		},
	)
	if k8sq.Value() > 0 {
		container.Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: k8sq,
		}
	}
	return container
}

func xbMemoryUse(res corev1.ResourceRequirements) (useMem string, k8sQuantity resource.Quantity) {
	if _, ok := res.Requests[corev1.ResourceMemory]; ok {
		k8sQuantity = *res.Requests.Memory()
	}
	if _, ok := res.Limits[corev1.ResourceMemory]; ok {
		k8sQuantity = *res.Limits.Memory()
	}

	useMem = "100MB"

	useMem75 := k8sQuantity.Value() / int64(100) * int64(75)
	if useMem75 > 2000000000 {
		useMem = "2GB"
	} else if k8sQuantity.Value() > 0 {
		useMem = strconv.FormatInt(useMem75, 10)
	}

	return useMem, k8sQuantity
}
