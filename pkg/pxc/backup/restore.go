package backup

import (
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util"
)

var log = logf.Log.WithName("backup/restore")

// const (
// 	caBundleCertDir  = "/tmp/s3/certs"
// 	caBundleCertFile = "ca.crt"
// )

func PVCRestoreService(cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster) *corev1.Service {
	restoreSvcName := pvcRestoreSvcName(cr)

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreSvcName,
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": restoreSvcName,
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

	if cluster.CompareVersionWith("1.16.0") >= 0 {
		svc.Labels = naming.LabelsCluster(cluster)
		svc.Spec.Selector = naming.LabelsCluster(cluster)
		svc.Spec.Selector[naming.LabelPerconaRestoreServiceName] = restoreSvcName
	}

	return svc
}

func pvcRestoreSvcName(cr *api.PerconaXtraDBClusterRestore) string {
	return "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster
}

func PVCRestorePod(cr *api.PerconaXtraDBClusterRestore, bcpStorageName, pvcName string, cluster *api.PerconaXtraDBCluster, initImage string) (*corev1.Pod, error) {
	if _, ok := cluster.Spec.Backup.Storages[bcpStorageName]; !ok {
		log.Info("storage " + bcpStorageName + " doesn't exist")
		if len(cluster.Spec.Backup.Storages) == 0 {
			cluster.Spec.Backup.Storages = map[string]*api.BackupStorageSpec{}
		}
		cluster.Spec.Backup.Storages[bcpStorageName] = &api.BackupStorageSpec{}
	}

	sslVolume := app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, !cluster.TLSEnabled())
	if cluster.CompareVersionWith("1.15.0") < 0 {
		sslVolume = app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, cluster.Spec.AllowUnsafeConfig)
	}

	restoreSvcName := pvcRestoreSvcName(cr)

	labels := naming.LabelsRestorePVCPod(cluster, bcpStorageName, restoreSvcName)

	volumes := []corev1.Volume{
		{
			Name: "backup",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		},
		app.GetSecretVolumes("ssl-internal", cluster.Spec.PXC.SSLInternalSecretName, true),
		sslVolume,
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
	}
	volumeMounts := []corev1.VolumeMount{
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
	}

	cmd := []string{"recovery-pvc-donor.sh"}

	var initContainers []corev1.Container
	if cluster.CompareVersionWith("1.18.0") >= 0 {
		volumes = append(volumes,
			corev1.Volume{
				Name: app.BinVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)

		volumeMounts = append(volumeMounts,
			corev1.VolumeMount{
				Name:      app.BinVolumeName,
				MountPath: app.BinVolumeMountPath,
			},
		)
		initContainers = []corev1.Container{statefulset.BackupInitContainer(cluster, initImage, cluster.Spec.PXC.ContainerSecurityContext)}
		cmd = []string{"/opt/percona/backup/recovery-pvc-donor.sh"}
	}
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        restoreSvcName,
			Namespace:   cr.Namespace,
			Annotations: cluster.Spec.Backup.Storages[bcpStorageName].Annotations,
			Labels:      labels,
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
			SecurityContext:  cluster.Spec.Backup.Storages[bcpStorageName].PodSecurityContext,
			InitContainers:   initContainers,
			Containers: []corev1.Container{
				{
					Name:            "ncat",
					Image:           cluster.Spec.Backup.Image,
					ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
					Command:         cmd,
					SecurityContext: cluster.Spec.Backup.Storages[bcpStorageName].ContainerSecurityContext,
					VolumeMounts:    volumeMounts,
					Resources:       cr.Spec.Resources,
				},
			},
			Volumes:                   volumes,
			RestartPolicy:             corev1.RestartPolicyAlways,
			NodeSelector:              cluster.Spec.Backup.Storages[bcpStorageName].NodeSelector,
			Affinity:                  cluster.Spec.Backup.Storages[bcpStorageName].Affinity,
			TopologySpreadConstraints: pxc.PodTopologySpreadConstraints(cluster.Spec.Backup.Storages[bcpStorageName].TopologySpreadConstraints, labels),
			Tolerations:               cluster.Spec.Backup.Storages[bcpStorageName].Tolerations,
			SchedulerName:             cluster.Spec.Backup.Storages[bcpStorageName].SchedulerName,
			PriorityClassName:         cluster.Spec.Backup.Storages[bcpStorageName].PriorityClassName,
			ServiceAccountName:        cluster.Spec.PXC.ServiceAccountName,
			RuntimeClassName:          cluster.Spec.Backup.Storages[bcpStorageName].RuntimeClassName,
		},
	}, nil
}

func appendCABundleSecretVolume(
	volumes *[]corev1.Volume,
	volumeMounts *[]corev1.VolumeMount,
	secretKeySel *corev1.SecretKeySelector,
) {
	const volumeName = "ca-bundle"
	vol := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretKeySel.Name,
				Items: []corev1.KeyToPath{
					{
						Key:  secretKeySel.Key,
						Path: naming.BackupStorageCAFileName,
					},
				},
			},
		},
	}
	*volumes = append(*volumes, vol)
	mnt := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: naming.BackupStorageCAFileDirectory,
	}
	*volumeMounts = append(*volumeMounts, mnt)
}

func RestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster, initImage string, scheme *runtime.Scheme, destination api.PXCBackupDestination, pitr bool) (*batchv1.Job, error) {
	switch bcp.Status.GetStorageType(cluster) {
	case api.BackupStorageAzure:
		if bcp.Status.Azure == nil {
			return nil, errors.New("nil azure backup status storage")
		}
	case api.BackupStorageS3:
		if bcp.Status.S3 == nil {
			return nil, errors.New("nil s3 backup status storage")
		}
	case api.BackupStorageFilesystem:
	default:
		return nil, errors.Errorf("no storage type was specified in status, got: %s", bcp.Status.GetStorageType(cluster))
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
	volumes := []corev1.Volume{
		{
			Name: "datadir",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "datadir-" + cr.Spec.PXCCluster + "-pxc-0",
				},
			},
		},
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
	}

	sslVolume := app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, !cluster.TLSEnabled())
	if cluster.CompareVersionWith("1.15.0") < 0 {
		sslVolume = app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, cluster.Spec.AllowUnsafeConfig)
	}

	var command []string
	switch bcp.Status.GetStorageType(cluster) {
	case api.BackupStorageFilesystem:
		command = []string{"recovery-pvc-joiner.sh"}
		if cluster.CompareVersionWith("1.18.0") >= 0 {
			command = []string{"/opt/percona/backup/recovery-pvc-joiner.sh"}
		}
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      "ssl",
				MountPath: "/etc/mysql/ssl",
			},
			{
				Name:      "ssl-internal",
				MountPath: "/etc/mysql/ssl-internal",
			},
		}...)
		volumes = append(volumes, []corev1.Volume{
			app.GetSecretVolumes("ssl-internal", cluster.Spec.PXC.SSLInternalSecretName, true),
			sslVolume,
		}...)
	case api.BackupStorageAzure, api.BackupStorageS3:
		command = []string{"recovery-cloud.sh"}
		if cluster.CompareVersionWith("1.18.0") >= 0 {
			command = []string{"/opt/percona/backup/recovery-cloud.sh"}
		}

		if pitr {
			if cluster.Spec.Backup == nil && len(cluster.Spec.Backup.Storages) == 0 {
				return nil, errors.New("no storage section")
			}
			jobName = "pitr-job-" + cr.Name + "-" + cr.Spec.PXCCluster
			volumeMounts = []corev1.VolumeMount{}
			volumes = []corev1.Volume{}
			command = []string{"/opt/percona/pitr", "recover"}
			if cluster.CompareVersionWith("1.15.0") < 0 {
				command = []string{"pitr", "recover"}
			}
		}

		// attach optional CA bundle file as a volume (if specified)
		if bcp.Status.S3 != nil && bcp.Status.S3.CABundle != nil {
			appendCABundleSecretVolume(&volumes, &volumeMounts, bcp.Status.S3.CABundle)
		}
	default:
		return nil, errors.Errorf("invalid storage type was specified in status, got: %s", bcp.Status.GetStorageType(cluster))
	}

	var initContainers []corev1.Container
	if pitr {
		if cluster.CompareVersionWith("1.15.0") >= 0 {
			initContainers = []corev1.Container{statefulset.PitrInitContainer(cluster, initImage)}
			volumes = append(volumes,
				corev1.Volume{
					Name: app.BinVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			)

			volumeMounts = append(volumeMounts,
				corev1.VolumeMount{
					Name:      app.BinVolumeName,
					MountPath: app.BinVolumeMountPath,
				},
			)
		}
	}

	envs, err := restoreJobEnvs(bcp, cr, cluster, destination, pitr)
	if err != nil {
		return nil, errors.Wrap(err, "restore job envs")
	}

	if cluster.CompareVersionWith("1.18.0") >= 0 && !pitr {
		volumes = append(volumes,
			corev1.Volume{
				Name: app.BinVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)

		volumeMounts = append(volumeMounts,
			corev1.VolumeMount{
				Name:      app.BinVolumeName,
				MountPath: app.BinVolumeMountPath,
			},
		)
		initContainers = []corev1.Container{statefulset.BackupInitContainer(cluster, initImage, cluster.Spec.PXC.ContainerSecurityContext)}
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cr.Namespace,
			Labels:    naming.LabelsRestoreJob(cluster, jobName, bcp.Status.StorageName),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cluster.Spec.PXC.Annotations,
					Labels:      naming.LabelsRestoreJob(cluster, jobName, bcp.Status.StorageName),
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
					SecurityContext:  cluster.Spec.PXC.PodSecurityContext,
					InitContainers:   initContainers,
					Containers: []corev1.Container{
						xtrabackupContainer(cr, cluster, command, volumeMounts, envs),
					},
					RestartPolicy:             corev1.RestartPolicyNever,
					Volumes:                   volumes,
					NodeSelector:              cluster.Spec.PXC.NodeSelector,
					Affinity:                  cluster.Spec.PXC.Affinity.Advanced,
					TopologySpreadConstraints: pxc.PodTopologySpreadConstraints(cluster.Spec.PXC.TopologySpreadConstraints, cluster.Spec.PXC.Labels),
					Tolerations:               cluster.Spec.PXC.Tolerations,
					SchedulerName:             cluster.Spec.PXC.SchedulerName,
					PriorityClassName:         cluster.Spec.PXC.PriorityClassName,
					ServiceAccountName:        cluster.Spec.PXC.ServiceAccountName,
					RuntimeClassName:          cluster.Spec.PXC.RuntimeClassName,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}
	if cluster.CompareVersionWith("1.16.0") < 0 {
		job.Labels = cluster.Spec.PXC.Labels
	}

	if err := controllerutil.SetControllerReference(cr, job, scheme); err != nil {
		return nil, errors.Wrap(err, "set controller reference")
	}
	for i := range job.OwnerReferences {
		job.OwnerReferences[i].BlockOwnerDeletion = nil
	}
	return job, nil
}

func restoreJobEnvs(bcp *api.PerconaXtraDBClusterBackup, cr *api.PerconaXtraDBClusterRestore, cluster *api.PerconaXtraDBCluster, destination api.PXCBackupDestination, pitr bool) ([]corev1.EnvVar, error) {
	if bcp.Status.GetStorageType(cluster) == api.BackupStorageFilesystem {
		return util.MergeEnvLists(
			[]corev1.EnvVar{
				{
					Name:  "RESTORE_SRC_SERVICE",
					Value: "restore-src-" + cr.Name + "-" + cr.Spec.PXCCluster,
				},
			},
			cr.Spec.ContainerOptions.GetEnvVar(cluster, bcp.Spec.StorageName),
		), nil
	}
	pxcUser := users.Xtrabackup
	verifyTLS := true
	if cluster.Spec.Backup != nil && len(cluster.Spec.Backup.Storages) > 0 {
		storage, ok := cluster.Spec.Backup.Storages[bcp.Spec.StorageName]
		if ok && storage.VerifyTLS != nil {
			verifyTLS = *storage.VerifyTLS
		}
	}
	if bs := cr.Spec.BackupSource; bs != nil {
		if bs.StorageName != "" {
			storage, ok := cluster.Spec.Backup.Storages[bs.StorageName]
			if ok && storage.VerifyTLS != nil {
				verifyTLS = *storage.VerifyTLS
			}
		}
		if bs.VerifyTLS != nil {
			verifyTLS = *bs.VerifyTLS
		}
	}
	envs := []corev1.EnvVar{
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
	if pitr {
		envs = append(envs, []corev1.EnvVar{
			{
				Name:  "PITR_GTID",
				Value: cr.Spec.PITR.GTID,
			},
			{
				Name:  "PITR_DATE",
				Value: cr.Spec.PITR.Date,
			},
			{
				Name:  "PITR_RECOVERY_TYPE",
				Value: cr.Spec.PITR.Type,
			},
		}...)
		if bs := cr.Spec.PITR.BackupSource; bs != nil {
			if bs.StorageName != "" {
				storage, ok := cluster.Spec.Backup.Storages[bs.StorageName]
				if ok && storage.VerifyTLS != nil {
					verifyTLS = *storage.VerifyTLS
				}
			}
			if bs.VerifyTLS != nil {
				verifyTLS = *bs.VerifyTLS
			}
		}
	}

	envs = append(envs, corev1.EnvVar{
		Name:  "VERIFY_TLS",
		Value: strconv.FormatBool(verifyTLS),
	})

	switch bcp.Status.GetStorageType(cluster) {
	case api.BackupStorageAzure:
		azureEnvs, err := azureEnvs(cr, bcp, cluster, destination, pitr)
		if err != nil {
			return nil, err
		}
		envs = append(envs, azureEnvs...)
	case api.BackupStorageS3:
		s3Envs, err := s3Envs(cr, bcp, cluster, destination, pitr)
		if err != nil {
			return nil, err
		}
		envs = append(envs, s3Envs...)
	default:
		return nil, errors.Errorf("invalid storage type was specified in status, got: %s", bcp.Status.GetStorageType(cluster))
	}
	return util.MergeEnvLists(
		envs,
		cr.Spec.ContainerOptions.GetEnvVar(cluster, bcp.Spec.StorageName),
	), nil
}

func azureEnvs(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster, destination api.PXCBackupDestination, pitr bool) ([]corev1.EnvVar, error) {
	azure := bcp.Status.Azure
	container, prefix := azure.ContainerAndPrefix()
	if container == "" {
		container, prefix = destination.BucketAndPrefix()
	}
	backupPath := path.Join(prefix, destination.BackupName())
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
			Value: backupPath,
		},
	}
	if pitr {
		storageAzure := new(api.BackupStorageAzureSpec)
		if bs := cr.Spec.PITR.BackupSource; bs != nil {
			if bs.StorageName != "" {
				storage, ok := cluster.Spec.Backup.Storages[cr.Spec.PITR.BackupSource.StorageName]
				if ok {
					storageAzure = storage.Azure
				}
			}
			if bs.Azure != nil {
				storageAzure = cr.Spec.PITR.BackupSource.Azure
			}
		}
		if len(storageAzure.ContainerPath) == 0 {
			return nil, errors.New("container name is not specified in storage")
		}
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
				Name:  "STORAGE_TYPE",
				Value: "azure",
			},
		}...)
	}
	return envs, nil
}

func s3Envs(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster, destination api.PXCBackupDestination, pitr bool) ([]corev1.EnvVar, error) {
	envs := []corev1.EnvVar{
		{
			Name:  "S3_BUCKET_URL",
			Value: strings.TrimPrefix(destination.String(), destination.StorageTypePrefix()),
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
	}
	if pitr {
		bucket := ""
		storageS3 := new(api.BackupStorageS3Spec)
		if bs := cr.Spec.PITR.BackupSource; bs != nil {
			if bs.StorageName != "" {
				storage, ok := cluster.Spec.Backup.Storages[bs.StorageName]
				if ok {
					storageS3 = storage.S3
					bucket = storage.S3.Bucket
				}
			}
			if bs.S3 != nil {
				storageS3 = bs.S3
				bucket = storageS3.Bucket
			}
		}
		if len(bucket) == 0 {
			return nil, errors.New("no bucket in storage")
		}
		envs = append(envs, []corev1.EnvVar{
			{
				Name:  "BINLOG_S3_ENDPOINT",
				Value: storageS3.EndpointURL,
			},
			{
				Name:  "BINLOG_S3_REGION",
				Value: storageS3.Region,
			},
			{
				Name: "BINLOG_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: storageS3.CredentialsSecret,
						},
						Key: "AWS_ACCESS_KEY_ID",
					},
				},
			},
			{
				Name: "BINLOG_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: storageS3.CredentialsSecret,
						},
						Key: "AWS_SECRET_ACCESS_KEY",
					},
				},
			},
			{
				Name:  "BINLOG_S3_BUCKET_URL",
				Value: bucket,
			},
			{
				Name:  "STORAGE_TYPE",
				Value: "s3",
			},
		}...)
	}
	return envs, nil
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

	useMem := xbMemoryUse(container.Resources)
	container.Env = append(
		container.Env,
		corev1.EnvVar{
			Name:  "XB_USE_MEMORY",
			Value: useMem,
		},
	)
	return container
}

func xbMemoryUse(res corev1.ResourceRequirements) string {
	var k8sQuantity resource.Quantity
	if _, ok := res.Requests[corev1.ResourceMemory]; ok {
		k8sQuantity = *res.Requests.Memory()
	}
	if _, ok := res.Limits[corev1.ResourceMemory]; ok {
		k8sQuantity = *res.Limits.Memory()
	}

	useMem := "100MB"

	useMem75 := k8sQuantity.Value() / int64(100) * int64(75)
	if useMem75 > 2000000000 {
		useMem = "2GB"
	} else if k8sQuantity.Value() > 0 {
		useMem = strconv.FormatInt(useMem75, 10)
	}

	return useMem
}

// PrepareJob creates a Kubernetes Job that prepares a restored PXC cluster for operation.
// This job runs after the data has been restored but before the cluster is started.
//
// The job mounts the PVC of the first PXC node and runs preparation scripts
// to ensure the restored data is ready for the cluster to start properly.
func PrepareJob(
	cr *api.PerconaXtraDBClusterRestore,
	bcp *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
	initImage string,
	scheme *runtime.Scheme,
) (*batchv1.Job, error) {
	jobName := naming.PrepareJobName(cr)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "datadir",
			MountPath: "/var/lib/mysql",
		},
		{
			Name:      "mysql-users-secret-file",
			MountPath: "/etc/mysql/mysql-users-secret",
		},
		{
			Name:      "vault-keyring-secret",
			MountPath: "/etc/mysql/vault-keyring-secret",
		},
		{
			Name:      "ssl",
			MountPath: "/etc/mysql/ssl",
		},
		{
			Name:      "ssl-internal",
			MountPath: "/etc/mysql/ssl-internal",
		},
	}
	volumes := []corev1.Volume{
		{
			Name: "datadir",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "datadir-" + cr.Spec.PXCCluster + "-pxc-0",
				},
			},
		},
		app.GetSecretVolumes("mysql-users-secret-file", "internal-"+cluster.Name, false),
		app.GetSecretVolumes("vault-keyring-secret", cluster.Spec.PXC.VaultSecretName, true),
		app.GetSecretVolumes("ssl", cluster.Spec.PXC.SSLSecretName, !cluster.TLSEnabled()),
		app.GetSecretVolumes("ssl-internal", cluster.Spec.PXC.SSLInternalSecretName, !cluster.TLSEnabled()),
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cr.Namespace,
			Labels:    naming.LabelsRestoreJob(cluster, jobName, bcp.Status.StorageName),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cluster.Spec.PXC.Annotations,
					Labels:      naming.LabelsRestoreJob(cluster, jobName, bcp.Status.StorageName),
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Spec.PXC.ImagePullSecrets,
					SecurityContext:  cluster.Spec.PXC.PodSecurityContext,
					InitContainers: []corev1.Container{
						statefulset.EntrypointInitContainer(cluster, initImage, app.DataVolumeName),
					},
					Containers: []corev1.Container{
						{
							Name:            "mysqld",
							Image:           cluster.Spec.PXC.Image,
							ImagePullPolicy: cluster.Spec.PXC.ImagePullPolicy,
							Command:         []string{"/var/lib/mysql/prepare_restored_cluster.sh"},
							SecurityContext: cluster.Spec.PXC.ContainerSecurityContext,
							VolumeMounts:    volumeMounts,
							Env:             []corev1.EnvVar{},
							Resources:       cluster.Spec.PXC.Resources,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       volumes,
					NodeSelector:  cluster.Spec.PXC.NodeSelector,
					Affinity:      cluster.Spec.PXC.Affinity.Advanced,
					TopologySpreadConstraints: pxc.PodTopologySpreadConstraints(
						cluster.Spec.PXC.TopologySpreadConstraints,
						cluster.Spec.PXC.Labels,
					),
					Tolerations:        cluster.Spec.PXC.Tolerations,
					SchedulerName:      cluster.Spec.PXC.SchedulerName,
					PriorityClassName:  cluster.Spec.PXC.PriorityClassName,
					ServiceAccountName: cluster.Spec.PXC.ServiceAccountName,
					RuntimeClassName:   cluster.Spec.PXC.RuntimeClassName,
				},
			},
			BackoffLimit: ptr.To(int32(4)),
		},
	}

	if err := controllerutil.SetControllerReference(cr, job, scheme); err != nil {
		return nil, errors.Wrap(err, "set controller reference")
	}
	for i := range job.OwnerReferences {
		job.OwnerReferences[i].BlockOwnerDeletion = nil
	}
	return job, nil
}
