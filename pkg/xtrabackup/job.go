package xtrabackup

import (
	"fmt"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func JobSpec(
	spec *pxcv1.PXCBackupSpec,
	cluster *pxcv1.PerconaXtraDBCluster,
	job *batchv1.Job,
	initImage string,
	primaryPodHost string,
) (batchv1.JobSpec, error) {
	var volumeMounts []corev1.VolumeMount
	var volumes []corev1.Volume
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

	storage := cluster.Spec.Backup.Storages[spec.StorageName]
	var initContainers []corev1.Container
	initContainers = append(initContainers, statefulset.BackupInitContainer(cluster, initImage, storage.ContainerSecurityContext))

	envs, err := xtrabackupJobEnvVars(storage, primaryPodHost)
	if err != nil {
		return batchv1.JobSpec{}, fmt.Errorf("failed to get xtrabackup job env vars: %w", err)
	}

	container := corev1.Container{
		Name:            "xtrabackup",
		Image:           cluster.Spec.Backup.Image,
		SecurityContext: storage.ContainerSecurityContext,
		ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
		Command:         []string{"/opt/percona/xtrabackup-run-backup"},
		Resources:       storage.Resources,
		VolumeMounts:    volumeMounts,
		Env:             envs,
	}

	manualSelector := true
	return batchv1.JobSpec{
		ActiveDeadlineSeconds: spec.ActiveDeadlineSeconds,
		ManualSelector:        &manualSelector,
		Selector: &metav1.LabelSelector{
			MatchLabels: job.Labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      job.Labels,
				Annotations: storage.Annotations,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					container,
				},

				RestartPolicy:             corev1.RestartPolicyNever,
				Volumes:                   volumes,
				InitContainers:            initContainers,
				SecurityContext:           storage.PodSecurityContext,
				ImagePullSecrets:          cluster.Spec.Backup.ImagePullSecrets,
				ServiceAccountName:        cluster.Spec.Backup.ServiceAccountName,
				Affinity:                  storage.Affinity,
				TopologySpreadConstraints: pxc.PodTopologySpreadConstraints(storage.TopologySpreadConstraints, job.Labels),
				Tolerations:               storage.Tolerations,
				NodeSelector:              storage.NodeSelector,
				SchedulerName:             storage.SchedulerName,
				PriorityClassName:         storage.PriorityClassName,
				RuntimeClassName:          storage.RuntimeClassName,
			},
		},
	}, nil
}

func xtrabackupJobEnvVars(
	storage *pxcv1.BackupStorageSpec,
	primaryPodHost string,
) ([]corev1.EnvVar, error) {
	envs := []corev1.EnvVar{
		{
			Name:  "HOST",
			Value: primaryPodHost,
		},
		{
			Name:  "STORAGE_TYPE",
			Value: string(storage.Type),
		},
		{
			Name:  "VERIFY_TLS",
			Value: fmt.Sprintf("%t", ptr.Deref(storage.VerifyTLS, true)),
		},
	}
	return envs, nil
}
