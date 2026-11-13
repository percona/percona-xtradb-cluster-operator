package backup

import (
	"context"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

const (
	// logicalRestoreContainerName is the name of the container of a logical restore job.
	logicalRestoreContainerName = "myloader"

	logicalRestoreScript = "/opt/percona/backup/logical-restore.sh"

	// logicalRestoreMinVolumeSize is the smallest volume a logical restore gets.
	logicalRestoreMinVolumeSize = int64(1) << 30 // 1Gi
	// logicalRestoreHeadroomDivisor adds 1/10 of the dump size on top of it.
	logicalRestoreHeadroomDivisor = 10
)

// LogicalRestoreVolumeSize returns the size of the volume a logical restore
// downloads the dump to. An override is used as is. Otherwise the size is
// the dump size plus 10%, and at least 1Gi.
func LogicalRestoreVolumeSize(dumpSize int64, override *resource.Quantity) resource.Quantity {
	if override != nil {
		return override.DeepCopy()
	}

	// round the headroom up to a whole byte
	headroom := (dumpSize + logicalRestoreHeadroomDivisor - 1) / logicalRestoreHeadroomDivisor
	size := max(dumpSize+headroom, logicalRestoreMinVolumeSize)

	return *resource.NewQuantity(size, resource.BinarySI)
}

// LogicalRestoreJob returns the job that restores a logical backup: it
// downloads the dump to a temporary volume and loads it with myloader into
// the first PXC pod of the running cluster.
//
// The volume is a generic ephemeral volume, so its claim is created and
// deleted together with the job's pod. The job never mounts a database volume.
func LogicalRestoreJob(
	ctx context.Context,
	cr *api.PerconaXtraDBClusterRestore,
	bcp *api.PerconaXtraDBClusterBackup,
	cluster *api.PerconaXtraDBCluster,
	initImage string,
	scheme *runtime.Scheme,
	volumeSize resource.Quantity,
) (*batchv1.Job, error) {
	if bcp.Status.S3 == nil {
		return nil, errors.New("nil s3 backup status storage")
	}

	envs, err := restoreJobEnvs(ctx, bcp, cr, cluster, bcp.Status.Destination, false)
	if err != nil {
		return nil, errors.Wrap(err, "restore job envs")
	}
	envs = append(envs,
		corev1.EnvVar{
			Name:  "PXC_HOST",
			Value: logicalRestoreHost(cr.Spec.PXCCluster),
		},
		corev1.EnvVar{
			Name:  "SCRATCH_DIR",
			Value: LogicalScratchMountPath,
		},
	)

	volumes := []corev1.Volume{
		{
			Name: naming.BinVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		logicalRestoreScratchVolume(cr, volumeSize),
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      naming.BinVolumeName,
			MountPath: naming.BinVolumeMountPath,
		},
		{
			Name:      LogicalScratchVolumeName,
			MountPath: LogicalScratchMountPath,
		},
	}

	// ca bundle (this is used by the aws-cli to verify the connection to S3)
	if bcp.Status.S3.CABundle != nil {
		appendCABundleSecretVolume(&volumes, &volumeMounts, bcp.Status.S3.CABundle)
	}

	jobName := naming.RestoreJobName(cr, false)
	labels := naming.LabelsRestoreJob(cluster, jobName, bcp.Status.StorageName)

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cr.Namespace,
			Labels:    labels,
			Finalizers: []string{
				naming.FinalizerKeepJob,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: cluster.Spec.Backup.TTLSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: cluster.Spec.PXC.Annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Spec.Backup.ImagePullSecrets,
					SecurityContext:  cluster.Spec.PXC.PodSecurityContext,
					InitContainers: []corev1.Container{
						statefulset.BackupInitContainer(cluster, initImage, cluster.Spec.PXC.ContainerSecurityContext),
					},
					Containers: []corev1.Container{
						{
							Name:            logicalRestoreContainerName,
							Image:           cluster.Spec.Backup.Image,
							ImagePullPolicy: cluster.Spec.Backup.ImagePullPolicy,
							Command:         []string{"bash", logicalRestoreScript},
							SecurityContext: cluster.Spec.PXC.ContainerSecurityContext,
							VolumeMounts:    volumeMounts,
							Env:             envs,
							Resources:       *cr.Spec.Resources.DeepCopy(),
						},
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
			BackoffLimit: new(int32(4)),
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

// logicalRestoreHost returns the address of the PXC pod a logical restore
// writes through. It is the first pod, which is where the proxy sends writes
// by default, so the load doesn't conflict with application writes that
// arrive on another node.
func logicalRestoreHost(clusterName string) string {
	service := clusterName + "-" + naming.ComponentPXC
	return service + "-0." + service
}

// logicalRestoreScratchVolume returns the temporary volume the dump is
// downloaded to.
func logicalRestoreScratchVolume(cr *api.PerconaXtraDBClusterRestore, size resource.Quantity) corev1.Volume {
	var storageClassName *string
	if cr.Spec.Job != nil && cr.Spec.Job.Volume != nil {
		storageClassName = cr.Spec.Job.Volume.StorageClassName
	}

	return corev1.Volume{
		Name: LogicalScratchVolumeName,
		VolumeSource: corev1.VolumeSource{
			Ephemeral: &corev1.EphemeralVolumeSource{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplate{
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: storageClassName,
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: size,
							},
						},
					},
				},
			},
		},
	}
}
