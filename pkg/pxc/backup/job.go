package backup

import (
	"net/url"
	"strings"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (*Backup) Job(cr *api.PerconaXtraDBClusterBackup, cluster *api.PerconaXtraDBCluster) *batchv1.Job {
	// Copy from the original labels to the backup labels
	labels := make(map[string]string)
	for key, value := range cluster.Spec.Backup.Storages[cr.Spec.StorageName].Labels {
		labels[key] = value
	}
	labels["type"] = "xtrabackup"
	labels["cluster"] = cr.Spec.PXCCluster
	labels["job-name"] = genName63(cr)

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

func (bcp *Backup) JobSpec(spec api.PXCBackupSpec, cluster *api.PerconaXtraDBCluster, job *batchv1.Job, initImageName string) batchv1.JobSpec {
	resources, err := app.CreateResources(cluster.Spec.Backup.Storages[spec.StorageName].Resources)
	if err != nil {
		log.Info("cannot parse Backup resources: ", err)
	}

	manualSelector := true
	backbackoffLimit := int32(10)
	jobSpec := batchv1.JobSpec{
		BackoffLimit:   &backbackoffLimit,
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
				SecurityContext:  cluster.Spec.Backup.Storages[spec.StorageName].PodSecurityContext,
				ImagePullSecrets: bcp.imagePullSecrets,
				RestartPolicy:    corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:            "xtrabackup",
						Image:           bcp.image,
						SecurityContext: cluster.Spec.Backup.Storages[spec.StorageName].ContainerSecurityContext,
						ImagePullPolicy: corev1.PullAlways,
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
								Name: "MYSQL_ROOT_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: app.SecretKeySelector(cluster.Spec.SecretsName, "root"),
								},
							},
						},
						Resources: resources,
					},
				},
				Affinity:          cluster.Spec.Backup.Storages[spec.StorageName].Affinity,
				Tolerations:       cluster.Spec.Backup.Storages[spec.StorageName].Tolerations,
				NodeSelector:      cluster.Spec.Backup.Storages[spec.StorageName].NodeSelector,
				SchedulerName:     cluster.Spec.Backup.Storages[spec.StorageName].SchedulerName,
				PriorityClassName: cluster.Spec.Backup.Storages[spec.StorageName].PriorityClassName,
			},
		},
	}

	if cluster.CompareVersionWith("1.5.0") >= 0 {
		jobSpec.Template.Spec.InitContainers = append(jobSpec.Template.Spec.InitContainers, corev1.Container{
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "xtrabackup",
					MountPath: "/backup",
				},
			},
			Image:   initImageName,
			Name:    "backup-init",
			Command: []string{"bash", "/backup-init-entrypoint.sh"},
		})

		jobSpec.Template.Spec.Containers[0].Command = []string{"bash", "/backup/backup.sh"}
	}

	return jobSpec
}

func appendStorageSecret(job *batchv1.JobSpec, cr *api.PerconaXtraDBCluster) error {
	// Volume for secret
	secretVol := corev1.Volume{
		Name: "ssl",
	}
	secretVol.Secret = &corev1.SecretVolumeSource{}
	secretVol.Secret.SecretName = cr.Spec.PXC.SSLSecretName
	t := true
	secretVol.Secret.Optional = &t

	// IntVolume for secret
	secretIntVol := corev1.Volume{
		Name: "ssl-internal",
	}
	secretIntVol.Secret = &corev1.SecretVolumeSource{}
	secretIntVol.Secret.SecretName = cr.Spec.PXC.SSLInternalSecretName
	secretIntVol.Secret.Optional = &t

	// Volume for vault secret
	secretVaultVol := corev1.Volume{
		Name: "vault-keyring-secret",
	}
	secretVaultVol.Secret = &corev1.SecretVolumeSource{}
	secretVaultVol.Secret.SecretName = cr.Spec.PXC.VaultSecretName
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

func (Backup) SetStoragePVC(job *batchv1.JobSpec, cr *api.PerconaXtraDBCluster, volName string) error {
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

	return err
}

func (Backup) SetStorageS3(job *batchv1.JobSpec, cr *api.PerconaXtraDBCluster, s3 api.BackupStorageS3Spec, destination string) error {
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

	u, err := parseS3URL(destination)
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

	shared := app.GetTmpVolume()

	// add SSL volumes
	job.Template.Spec.Volumes = []corev1.Volume{shared}
	job.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "tmp",
			MountPath: "/backup",
		},
	}
	err = appendStorageSecret(job, cr)

	return err
}

func parseS3URL(bucketURL string) (*url.URL, error) {
	u, err := url.Parse(bucketURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse s3 URL")
	}

	return u, nil
}
