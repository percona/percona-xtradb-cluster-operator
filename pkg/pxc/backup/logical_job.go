package backup

import (
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

const (
	// LogicalScratchVolumeName is the name of the volume a logical backup job
	// writes mydumper files to before they are uploaded.
	LogicalScratchVolumeName = "logical-scratch"
	// LogicalScratchMountPath is where the scratch volume is mounted.
	LogicalScratchMountPath = "/scratch"

	logicalBackupScript = "/opt/percona/backup/logical-backup.sh"
)

// LogicalJobSpec returns the spec of a job that takes a logical backup with
// mydumper. It is the physical backup job spec with a different command and
// a scratch volume for the dump files, so that pod settings stay the same
// for both backup types.
func (bcp *Backup) LogicalJobSpec(
	spec api.PXCBackupSpec,
	cluster *api.PerconaXtraDBCluster,
	job *batchv1.Job,
	initImage string,
) (batchv1.JobSpec, error) {
	jobSpec, err := bcp.JobSpec(spec, cluster, job, initImage)
	if err != nil {
		return batchv1.JobSpec{}, errors.Wrap(err, "get job spec")
	}

	if len(jobSpec.Template.Spec.Containers) == 0 {
		return batchv1.JobSpec{}, errors.New("no containers in job spec")
	}

	container := &jobSpec.Template.Spec.Containers[0]
	container.Command = []string{"bash", logicalBackupScript}
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  "SCRATCH_DIR",
		Value: LogicalScratchMountPath,
	})
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      LogicalScratchVolumeName,
		MountPath: LogicalScratchMountPath,
	})

	jobSpec.Template.Spec.Volumes = append(jobSpec.Template.Spec.Volumes, corev1.Volume{
		Name: LogicalScratchVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	return jobSpec, nil
}
