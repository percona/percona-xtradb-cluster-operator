package backup

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

func (*Backup) Job(cr *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) *batchv1.Job {
	// Copy from the original labels to the backup labels
	labels := make(map[string]string)
	for key, value := range cluster.Spec.Backup.Storages[cr.Spec.StorageName].Labels {
		labels[key] = value
	}
	labels["type"] = "xtrabackup"
	labels["cluster"] = cr.Spec.PXCCluster
	labels["job-name"] = GenName63(cr)

	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        labels["job-name"],
			Namespace:   cr.Namespace,
			Labels:      labels,
			Annotations: cluster.Spec.Backup.Storages[cr.Spec.StorageName].Annotations,
		},
	}
}

func (bcp *Backup) JobSpec(spec api.PXCBackupSpec, cluster *api.PerconaXtraDBCluster, job *batchv1.Job) (batchv1.JobSpec, error) {
	manualSelector := true
	backoffLimit := int32(10)
	if cluster.CompareVersionWith("1.11.0") >= 0 && cluster.Spec.Backup.BackoffLimit != nil {
		backoffLimit = *cluster.Spec.Backup.BackoffLimit
	}
	verifyTLS := true
	if cluster.Spec.Backup.Storages[spec.StorageName].VerifyTLS != nil {
		verifyTLS = *cluster.Spec.Backup.Storages[spec.StorageName].VerifyTLS
	}
	return batchv1.JobSpec{
		BackoffLimit:   &backoffLimit,
		ManualSelector: &manualSelector,
		Selector: &metav1.LabelSelector{
			MatchLabels: job.Labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      job.Labels,
				Annotations: cluster.Spec.Backup.Storages[spec.StorageName].Annotations,
			},
			Spec: corev1.PodSpec{
				SecurityContext:    cluster.Spec.Backup.Storages[spec.StorageName].PodSecurityContext,
				ImagePullSecrets:   bcp.imagePullSecrets,
				RestartPolicy:      corev1.RestartPolicyNever,
				ServiceAccountName: cluster.Spec.Backup.ServiceAccountName,
				Containers: []corev1.Container{
					{
						Name:            "xtrabackup",
						Image:           bcp.image,
						SecurityContext: cluster.Spec.Backup.Storages[spec.StorageName].ContainerSecurityContext,
						ImagePullPolicy: bcp.imagePullPolicy,
						Command:         []string{"bash", "/usr/bin/backup.sh"},
						Env: []corev1.EnvVar{
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
									SecretKeyRef: app.SecretKeySelector(cluster.Spec.SecretsName, "xtrabackup"),
								},
							},
							{
								Name:  "VERIFY_TLS",
								Value: strconv.FormatBool(verifyTLS),
							},
						},
						Resources: cluster.Spec.Backup.Storages[spec.StorageName].Resources,
					},
				},
				Affinity:          cluster.Spec.Backup.Storages[spec.StorageName].Affinity,
				Tolerations:       cluster.Spec.Backup.Storages[spec.StorageName].Tolerations,
				NodeSelector:      cluster.Spec.Backup.Storages[spec.StorageName].NodeSelector,
				SchedulerName:     cluster.Spec.Backup.Storages[spec.StorageName].SchedulerName,
				PriorityClassName: cluster.Spec.Backup.Storages[spec.StorageName].PriorityClassName,
				RuntimeClassName:  cluster.Spec.Backup.Storages[spec.StorageName].RuntimeClassName,
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
			Name:      "vault-keyring-secret",
			MountPath: "/etc/mysql/vault-keyring-secret",
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

func SetStoragePVC(job *batchv1.JobSpec, cr *api.PerconaXtraDBClusterBackup, volName string) error {
	pvc := corev1.Volume{
		Name: "xtrabackup",
	}
	pvc.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: volName,
	}

	if len(job.Template.Spec.Containers) == 0 {
		return errors.New("no containers in job spec")
	}

	job.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      pvc.Name,
			MountPath: "/backup",
		},
	}

	job.Template.Spec.Volumes = []corev1.Volume{
		pvc,
	}

	err := appendStorageSecret(job, cr)
	if err != nil {
		return errors.Wrap(err, "failed to append storage secret")
	}

	return nil
}

func SetStorageAzure(job *batchv1.JobSpec, cr *api.PerconaXtraDBClusterBackup) error {
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
	container, _ := azure.ContainerAndPrefix()
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
		Value: strings.TrimPrefix(cr.Status.Destination, container+"/"),
	}
	if len(job.Template.Spec.Containers) == 0 {
		return errors.New("no containers in job spec")
	}
	job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, storageAccount, accessKey, containerName, endpoint, storageClass, backupPath)

	// add SSL volumes
	job.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{}
	job.Template.Spec.Volumes = []corev1.Volume{}

	err := appendStorageSecret(job, cr)
	if err != nil {
		return errors.Wrap(err, "failed to append storage secrets")
	}

	return nil
}

func SetStorageS3(job *batchv1.JobSpec, cr *api.PerconaXtraDBClusterBackup) error {
	if cr.Status.S3 == nil {
		return errors.New("s3 storage is not specified in backup status")
	}
	s3 := cr.Status.S3
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
	region := corev1.EnvVar{
		Name:  "DEFAULT_REGION",
		Value: s3.Region,
	}
	endpoint := corev1.EnvVar{
		Name:  "ENDPOINT",
		Value: s3.EndpointURL,
	}

	if len(job.Template.Spec.Containers) == 0 {
		return errors.New("no containers in job spec")
	}
	job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, accessKey, secretKey, region, endpoint)

	u, err := parseS3URL(cr.Status.Destination)
	if err != nil {
		return errors.Wrap(err, "failed to create job")
	}
	bucket := corev1.EnvVar{
		Name:  "S3_BUCKET",
		Value: u.Host,
	}
	bucketPath := corev1.EnvVar{
		Name:  "S3_BUCKET_PATH",
		Value: strings.TrimLeft(u.Path, "/"),
	}
	job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, bucket, bucketPath)

	// add SSL volumes
	job.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{}
	job.Template.Spec.Volumes = []corev1.Volume{}

	err = appendStorageSecret(job, cr)
	if err != nil {
		return errors.Wrap(err, "failed to append storage secrets")
	}

	return nil
}

func parseS3URL(bucketURL string) (*url.URL, error) {
	u, err := url.Parse(bucketURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse s3 URL")
	}

	return u, nil
}
