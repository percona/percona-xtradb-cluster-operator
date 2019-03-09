package backup

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
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

func (bcp *Backup) JobSpec(spec api.PXCBackupSpec, pvcName, pxcNode string, sv *api.ServerVersion) batchv1.JobSpec {
	pvc := corev1.Volume{
		Name: "xtrabackup",
	}
	pvc.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: pvcName,
	}

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

	return batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					FSGroup: fsgroup,
				},
				Containers: []corev1.Container{
					{
						Name:    "xtrabackup",
						Image:   bcp.image,
						Command: []string{"bash", "/usr/bin/backup.sh"},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      pvc.Name,
								MountPath: "/backup",
							},
						},
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
				ImagePullSecrets: bcp.imagePullSecrets,
				RestartPolicy:    corev1.RestartPolicyNever,
				Volumes: []corev1.Volume{
					pvc,
				},
			},
		},
	}
}
