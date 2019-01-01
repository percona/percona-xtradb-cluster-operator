package backup

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func NewJob(cr *api.PerconaXtraDBBackup, sv *api.ServerVersion) *batchv1.Job {
	jb := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.PXCCluster + "-xtrabackup." + cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"cluster": cr.Spec.PXCCluster,
				"type":    "xtrabackup",
			},
		},
		Spec: jobSpec(cr.Spec, cr.Name, sv),
	}

	return jb
}

func jobSpec(spec api.PXCBackupSpec, name string, sv *api.ServerVersion) batchv1.JobSpec {
	pvc := corev1.Volume{
		Name: spec.PXCCluster + "-backup-" + name,
	}
	pvc.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: spec.PXCCluster + volumeNamePostfix + "." + name,
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
						Image:   "perconalab/backupjob-openshift",
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
								Value: spec.PXCCluster + "-pxc-nodes",
							},
							{
								Name:  "BACKUP_DIR",
								Value: "/backup",
							},
						},
					},
				},
				RestartPolicy: corev1.RestartPolicyNever,
				Volumes: []corev1.Volume{
					pvc,
				},
			},
		},
	}
}
