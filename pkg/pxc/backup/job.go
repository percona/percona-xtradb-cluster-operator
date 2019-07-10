package backup

import (
	"net/url"
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

func (*Backup) Job(cr *api.PerconaXtraDBClusterBackup) *batchv1.Job {
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      genName63(cr),
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"cluster": cr.Spec.PXCCluster,
				"type":    "xtrabackup",
			},
		},
	}
}

func (bcp *Backup) JobSpec(spec api.PXCBackupSpec, sv *api.ServerVersion, secrets string) batchv1.JobSpec {
	var fsgroup *int64
	if sv.Platform == api.PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}

	return batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					SupplementalGroups: []int64{1001},
					FSGroup:            fsgroup,
				},
				ImagePullSecrets: bcp.imagePullSecrets,
				RestartPolicy:    corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:            "xtrabackup",
						Image:           bcp.image,
						Command:         []string{"bash", "/usr/bin/backup.sh"},
						ImagePullPolicy: corev1.PullAlways,
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
									SecretKeyRef: app.SecretKeySelector(secrets, "root"),
								},
							},
						},
					},
				},
				NodeSelector: bcp.nodeSelector,
			},
		},
	}
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
	)
	job.Template.Spec.Volumes = append(
		job.Template.Spec.Volumes,
		secretVol,
		secretIntVol,
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
	appendStorageSecret(job, cr)

	return nil
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

	// add SSL volumes
	job.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{}
	job.Template.Spec.Volumes = []corev1.Volume{}
	appendStorageSecret(job, cr)

	return nil
}

func parseS3URL(bucketURL string) (*url.URL, error) {
	u, err := url.Parse(bucketURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse s3 URL")
	}

	return u, nil
}
