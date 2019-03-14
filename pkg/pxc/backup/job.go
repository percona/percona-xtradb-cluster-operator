package backup

import (
	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/url"
	"strings"
)

func (*Backup) Job(cr *api.PerconaXtraDBBackup) *batchv1.Job {
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

func (bcp *Backup) JobSpec(spec api.PXCBackupSpec, stor api.BackupStorageSpec, pxcNode string, sv *api.ServerVersion) (*batchv1.JobSpec, error) {
	// if a suitable node hasn't been chosen - try to make a lucky shot.
	// it's better than the failed backup at all
	if pxcNode == "" {
		pxcNode = spec.PXCCluster + "-pxc-nodes"
	}

	var fsgroup *int64
	if sv.Platform == api.PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}

	job := batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					FSGroup: fsgroup,
				},
				ImagePullSecrets: bcp.imagePullSecrets,
				RestartPolicy:    corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:    "xtrabackup",
						Image:   bcp.image,
						Command: []string{"bash", "/usr/bin/backup.sh"},
						Env: []corev1.EnvVar{
							{
								Name:  "NODE_NAME",
								Value: pxcNode,
							},
							{
								Name:  "BACKUP_DIR",
								Value: "/backup",
							},
						},
					},
				},
			},
		},
	}

	switch stor.Type {
	case api.BackupStorageFilesystem:
		pvc := corev1.Volume{
			Name: "xtrabackup",
		}
		pvc.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: stor.Volume.PersistentVolumeClaim.VolumeName,
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
	case api.BackupStorageS3:
		accessKey := corev1.EnvVar{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(stor.S3.CredentialsSecret, "accessKey"),
			},
		}
		secretKey := corev1.EnvVar{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(stor.S3.CredentialsSecret, "secretKey"),
			},
		}
		region := corev1.EnvVar{
			Name:  "AWS_DEFAULT_REGION",
			Value: stor.S3.Region,
		}
		endpoint := corev1.EnvVar{
			Name:  "AWS_ENDPOINT_URL",
			Value: stor.S3.EndpointURL,
		}
		job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, accessKey, secretKey, region, endpoint)

		u, err := parseS3URL(stor.S3.Bucket)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create job")
		}
		bucket := corev1.EnvVar{
			Name:  "AWS_S3_BUCKET",
			Value: u.Host,
		}
		bucketPath := corev1.EnvVar{
			Name:  "AWS_S3_BUCKET_PATH",
			Value: strings.TrimLeft(u.Path, "/"),
		}
		job.Template.Spec.Containers[0].Env = append(job.Template.Spec.Containers[0].Env, bucket, bucketPath)
	}

	return &job, nil
}

func parseS3URL(bucketUrl string) (*url.URL, error) {
	u, err := url.Parse(bucketUrl)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse s3 URL")
	}
	if u.Scheme != "s3" {
		return nil, errors.New("invalid scheme")
	}
	return u, nil
}
