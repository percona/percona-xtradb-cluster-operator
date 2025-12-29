package backup

import (
	"context"
	"path"
	"strconv"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/util"
)

func (*Backup) Job(cr *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) *batchv1.Job {
	jobName := naming.BackupJobName(cr.Name)

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   cr.Namespace,
			Labels:      naming.LabelsBackupJob(cr, cluster, jobName),
			Annotations: cluster.Spec.Backup.Storages[cr.Spec.StorageName].Annotations,
			Finalizers: []string{
				naming.FinalizerKeepJob,
			},
		},
	}
}

func (bcp *Backup) JobSpec(spec api.PXCBackupSpec, cluster *api.PerconaXtraDBCluster, job *batchv1.Job, initImage string) (batchv1.JobSpec, error) {
	manualSelector := true
	backoffLimit := int32(10)
	if cluster.CompareVersionWith("1.11.0") >= 0 && cluster.Spec.Backup.BackoffLimit != nil {
		backoffLimit = *cluster.Spec.Backup.BackoffLimit
	}
	var activeDeadlineSeconds *int64
	if cluster.CompareVersionWith("1.16.0") >= 0 {
		if spec.ActiveDeadlineSeconds != nil {
			activeDeadlineSeconds = spec.ActiveDeadlineSeconds
		} else if cluster.Spec.Backup.ActiveDeadlineSeconds != nil {
			activeDeadlineSeconds = cluster.Spec.Backup.ActiveDeadlineSeconds
		}
	}
	verifyTLS := true
	storage := cluster.Spec.Backup.Storages[spec.StorageName]
	if storage.VerifyTLS != nil {
		verifyTLS = *storage.VerifyTLS
	}
	envs := []corev1.EnvVar{
		{
			Name:  "BACKUP_DIR",
			Value: "/backup",
		},
		{
			Name:  "PXC_SERVICE",
			Value: spec.PXCCluster + "-pxc",
		},
		{
			Name: "PXC_PASS",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(cluster.Spec.SecretsName, users.Xtrabackup),
			},
		},
		{
			Name:  "VERIFY_TLS",
			Value: strconv.FormatBool(verifyTLS),
		},
	}
	envs = util.MergeEnvLists(envs, spec.ContainerOptions.GetEnvVar(cluster, spec.StorageName))

	var volumeMounts []corev1.VolumeMount
	var volumes []corev1.Volume
	var initContainers []corev1.Container
	if cluster.CompareVersionWith("1.15.0") >= 0 {
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

		initContainers = append(initContainers, statefulset.BackupInitContainer(cluster, initImage, storage.ContainerSecurityContext))
	}

	cmd := []string{"bash", "/usr/bin/backup.sh"}
	if cluster.CompareVersionWith("1.18.0") >= 0 {
		cmd = []string{"bash", "/opt/percona/backup/backup.sh"}
	}

	return batchv1.JobSpec{
		ActiveDeadlineSeconds:   activeDeadlineSeconds,
		BackoffLimit:            &backoffLimit,
		ManualSelector:          &manualSelector,
		TTLSecondsAfterFinished: cluster.Spec.Backup.TTLSecondsAfterFinished,
		Selector: &metav1.LabelSelector{
			MatchLabels: job.Labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      job.Labels,
				Annotations: storage.Annotations,
			},
			Spec: corev1.PodSpec{
				SecurityContext:    storage.PodSecurityContext,
				ImagePullSecrets:   bcp.imagePullSecrets,
				RestartPolicy:      corev1.RestartPolicyNever,
				ServiceAccountName: cluster.Spec.Backup.ServiceAccountName,
				InitContainers:     initContainers,
				Containers: []corev1.Container{
					{
						Name:            "xtrabackup",
						Image:           bcp.image,
						SecurityContext: storage.ContainerSecurityContext,
						ImagePullPolicy: bcp.imagePullPolicy,
						Command:         cmd,
						Env:             envs,
						Resources:       storage.Resources,
						VolumeMounts:    volumeMounts,
					},
				},
				Affinity:                  storage.Affinity,
				TopologySpreadConstraints: pxc.PodTopologySpreadConstraints(storage.TopologySpreadConstraints, job.Labels),
				Tolerations:               storage.Tolerations,
				NodeSelector:              storage.NodeSelector,
				SchedulerName:             storage.SchedulerName,
				PriorityClassName:         storage.PriorityClassName,
				RuntimeClassName:          storage.RuntimeClassName,
				Volumes:                   volumes,
			},
		},
	}, nil
}

func appendStorageSecret(job *batchv1.JobSpec, cr *api.PerconaXtraDBClusterBackup) error {
	// Volume for secret
	secretVol := corev1.Volume{
		Name: "ssl",
	}
	secretVol.Secret = &corev1.SecretVolumeSource{}
	secretVol.Secret.SecretName = cr.Status.SSLSecretName
	t := true
	secretVol.Secret.Optional = &t

	// IntVolume for secret
	secretIntVol := corev1.Volume{
		Name: "ssl-internal",
	}
	secretIntVol.Secret = &corev1.SecretVolumeSource{}
	secretIntVol.Secret.SecretName = cr.Status.SSLInternalSecretName
	secretIntVol.Secret.Optional = &t

	// Volume for vault secret
	secretVaultVol := corev1.Volume{
		Name: "vault-keyring-secret",
	}
	secretVaultVol.Secret = &corev1.SecretVolumeSource{}
	secretVaultVol.Secret.SecretName = cr.Status.VaultSecretName
	secretVaultVol.Secret.Optional = &t

	if len(job.Template.Spec.Containers) == 0 {
		return errors.New("no containers in job spec")
	}
	job.Template.Spec.Containers[0].VolumeMounts = append(
		job.Template.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "ssl",
			MountPath: "/etc/mysql/ssl",
		},
		corev1.VolumeMount{
			Name:      "ssl-internal",
			MountPath: "/etc/mysql/ssl-internal",
		},
		corev1.VolumeMount{
			Name:      statefulset.VaultSecretVolumeName,
			MountPath: statefulset.VaultSecretMountPath,
		},
	)
	job.Template.Spec.Volumes = append(
		job.Template.Spec.Volumes,
		secretVol,
		secretIntVol,
		secretVaultVol,
	)
	return nil
}

func SetStoragePVC(ctx context.Context, job *batchv1.JobSpec, cr *api.PerconaXtraDBClusterBackup, volName string) error {
	pvc := corev1.Volume{
		Name: "xtrabackup",
	}
	pvc.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: volName,
	}

	if len(job.Template.Spec.Containers) == 0 {
		return errors.New("no containers in job spec")
	}

	job.Template.Spec.Containers[0].VolumeMounts = append(job.Template.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{
		{
			Name:      pvc.Name,
			MountPath: "/backup",
		},
	}...)

	job.Template.Spec.Volumes = append(job.Template.Spec.Volumes, []corev1.Volume{
		pvc,
	}...)

	err := appendStorageSecret(job, cr)
	if err != nil {
		return errors.Wrap(err, "failed to append storage secret")
	}

	return nil
}

func SetStorageAzure(ctx context.Context, job *batchv1.JobSpec, cr *api.PerconaXtraDBClusterBackup) error {
	if cr.Status.Azure == nil {
		return errors.New("azure storage is not specified in backup status")
	}
	azure := cr.Status.Azure
	storageAccount := corev1.EnvVar{
		Name: "AZURE_STORAGE_ACCOUNT",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: app.SecretKeySelector(azure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_NAME"),
		},
	}
	accessKey := corev1.EnvVar{
		Name: "AZURE_ACCESS_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: app.SecretKeySelector(azure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_KEY"),
		},
	}
	container, prefix := azure.ContainerAndPrefix()
	if container == "" {
		container, prefix = cr.Status.Destination.BucketAndPrefix()
	}
	bucketPath := path.Join(prefix, cr.Status.Destination.BackupName())

	containerName := corev1.EnvVar{
		Name:  "AZURE_CONTAINER_NAME",
		Value: container,
	}
	endpoint := corev1.EnvVar{
		Name:  "AZURE_ENDPOINT",
		Value: azure.Endpoint,
	}
	storageClass := corev1.EnvVar{
		Name:  "AZURE_STORAGE_CLASS",
		Value: azure.StorageClass,
	}
	backupPath := corev1.EnvVar{
		Name:  "BACKUP_PATH",
		Value: bucketPath,
	}
	if len(job.Template.Spec.Containers) == 0 {
		return errors.New("no containers in job spec")
	}
	job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, storageAccount, accessKey, containerName, endpoint, storageClass, backupPath)

	// add SSL volumes
	err := appendStorageSecret(job, cr)
	if err != nil {
		return errors.Wrap(err, "failed to append storage secrets")
	}

	return nil
}

func SetStorageS3(ctx context.Context, job *batchv1.JobSpec, cr *api.PerconaXtraDBClusterBackup) error {
	if cr.Status.S3 == nil {
		return errors.New("s3 storage is not specified in backup status")
	}

	if len(job.Template.Spec.Containers) == 0 {
		return errors.New("no containers in job spec")
	}

	s3 := cr.Status.S3

	region := corev1.EnvVar{
		Name:  "DEFAULT_REGION",
		Value: s3.Region,
	}
	endpoint := corev1.EnvVar{
		Name:  "ENDPOINT",
		Value: s3.EndpointURL,
	}

	if s3.CredentialsSecret != "" {
		accessKey := corev1.EnvVar{
			Name: "ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(s3.CredentialsSecret, "AWS_ACCESS_KEY_ID"),
			},
		}
		secretKey := corev1.EnvVar{
			Name: "SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(s3.CredentialsSecret, "AWS_SECRET_ACCESS_KEY"),
			},
		}
		sessionToken := corev1.EnvVar{
			Name: "S3_SESSION_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelectorWithOptional(s3.CredentialsSecret, "AWS_SESSION_TOKEN", true),
			},
		}

		job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, accessKey, secretKey, sessionToken)
	}

	job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, region, endpoint)

	bucket, prefix := s3.BucketAndPrefix()
	if bucket == "" {
		bucket, prefix = cr.Status.Destination.BucketAndPrefix()
	}
	bucketPath := path.Join(prefix, cr.Status.Destination.BackupName())

	bucketEnv := corev1.EnvVar{
		Name:  "S3_BUCKET",
		Value: bucket,
	}
	bucketPathEnv := corev1.EnvVar{
		Name:  "S3_BUCKET_PATH",
		Value: bucketPath,
	}
	job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, bucketEnv, bucketPathEnv)

	// add SSL volumes
	err := appendStorageSecret(job, cr)
	if err != nil {
		return errors.Wrap(err, "failed to append storage secrets")
	}

	// add ca bundle (this is used by the aws-cli to verify the connection to S3)
	if sel := s3.CABundle; sel != nil {
		appendCABundleSecretVolume(
			&job.Template.Spec.Volumes,
			&job.Template.Spec.Containers[0].VolumeMounts,
			sel,
		)
	}

	return nil
}
