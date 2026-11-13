package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

const logicalTestStorageName = "test-storage"

// newLogicalTestCluster returns a cluster with defaults applied and one s3
// storage, whose container options are the given ones.
func newLogicalTestCluster(t *testing.T, storageOptions *pxcv1.BackupContainerOptions) *pxcv1.PerconaXtraDBCluster {
	t.Helper()

	cluster := &pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: pxcv1.PerconaXtraDBClusterSpec{
			CRVersion: version.Version(),
			Backup: &pxcv1.BackupSpec{
				Image: "percona/percona-xtrabackup:8.4",
				Storages: map[string]*pxcv1.BackupStorageSpec{
					logicalTestStorageName: {
						Type: pxcv1.BackupStorageS3,
						S3: &pxcv1.BackupStorageS3Spec{
							Bucket:            "operator-testing",
							Region:            "us-west-1",
							CredentialsSecret: "test-secret",
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
						NodeSelector:     map[string]string{"disk": "fast"},
						ContainerOptions: storageOptions,
					},
				},
			},
			PXC: &pxcv1.PXCSpec{
				PodSpec: &pxcv1.PodSpec{
					Size:     3,
					Image:    "percona/percona-xtradb-cluster:8.4",
					Affinity: &pxcv1.PodAffinity{},
					VolumeSpec: &pxcv1.VolumeSpec{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
		},
	}

	sv := &version.ServerVersion{
		Platform: version.PlatformKubernetes,
		Info:     k8sversion.Info{},
	}
	require.NoError(t, cluster.CheckNSetDefaults(sv, log))

	return cluster
}

func TestLogicalJobSpec(t *testing.T) {
	tests := map[string]struct {
		storageOptions *pxcv1.BackupContainerOptions
		backupOptions  *pxcv1.BackupContainerOptions
		// expectedMydumperArgs is the expected value of MYDUMPER_EXTRA_ARGS.
		// Empty means the variable must not be set.
		expectedMydumperArgs string
	}{
		"no container options": {},
		"mydumper args on the backup": {
			backupOptions: &pxcv1.BackupContainerOptions{
				Args: pxcv1.BackupContainerArgs{Mydumper: []string{"--threads=8", "--sync-thread-lock-mode=NO_LOCK"}},
			},
			expectedMydumperArgs: "--threads=8 --sync-thread-lock-mode=NO_LOCK",
		},
		"mydumper args on the storage": {
			storageOptions: &pxcv1.BackupContainerOptions{
				Args: pxcv1.BackupContainerArgs{Mydumper: []string{"--threads=2"}},
			},
			expectedMydumperArgs: "--threads=2",
		},
		"backup args take precedence over storage args": {
			storageOptions: &pxcv1.BackupContainerOptions{
				Args: pxcv1.BackupContainerArgs{Mydumper: []string{"--threads=2"}},
			},
			backupOptions: &pxcv1.BackupContainerOptions{
				Args: pxcv1.BackupContainerArgs{Mydumper: []string{"--threads=8"}},
			},
			expectedMydumperArgs: "--threads=8",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cluster := newLogicalTestCluster(t, tt.storageOptions)
			cr := &pxcv1.PerconaXtraDBClusterBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test-ns",
				},
				Spec: pxcv1.PXCBackupSpec{
					PXCCluster:       "test-cluster",
					StorageName:      logicalTestStorageName,
					Type:             pxcv1.BackupTypeLogical,
					ContainerOptions: tt.backupOptions,
				},
			}

			bcp := New(cluster)
			job := bcp.Job(cr, cluster)

			physical, err := bcp.JobSpec(cr.Spec, cluster, job, "init-image")
			require.NoError(t, err)

			logical, err := bcp.LogicalJobSpec(cr.Spec, cluster, job, "init-image")
			require.NoError(t, err)

			require.Len(t, logical.Template.Spec.Containers, 1)
			container := logical.Template.Spec.Containers[0]
			physicalContainer := physical.Template.Spec.Containers[0]

			// what a logical backup job changes
			assert.Equal(t, "mydumper", container.Name)
			assert.Equal(t, "xtrabackup", physicalContainer.Name)
			assert.Equal(t, []string{"bash", "/opt/percona/backup/logical-backup.sh"}, container.Command)
			assert.Contains(t, container.Env, corev1.EnvVar{Name: "SCRATCH_DIR", Value: "/scratch"})
			assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "logical-scratch", MountPath: "/scratch"})
			assert.Contains(t, logical.Template.Spec.Volumes, corev1.Volume{
				Name:         "logical-scratch",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			})

			mydumperArgs := ""
			for _, env := range container.Env {
				if env.Name == "MYDUMPER_EXTRA_ARGS" {
					mydumperArgs = env.Value
				}
			}
			assert.Equal(t, tt.expectedMydumperArgs, mydumperArgs)

			// what it keeps from the physical backup job
			assert.Subset(t, container.Env, physicalContainer.Env)
			assert.Subset(t, container.VolumeMounts, physicalContainer.VolumeMounts)
			assert.Subset(t, logical.Template.Spec.Volumes, physical.Template.Spec.Volumes)
			assert.Equal(t, physicalContainer.Image, container.Image)
			assert.Equal(t, physicalContainer.Resources, container.Resources)
			assert.Equal(t, physicalContainer.SecurityContext, container.SecurityContext)
			assert.Equal(t, physical.Template.Spec.InitContainers, logical.Template.Spec.InitContainers)
			assert.Equal(t, physical.Template.Spec.NodeSelector, logical.Template.Spec.NodeSelector)
			assert.Equal(t, physical.Template.Spec.Affinity, logical.Template.Spec.Affinity)
			assert.Equal(t, physical.Template.Spec.ServiceAccountName, logical.Template.Spec.ServiceAccountName)
			assert.Equal(t, physical.Template.Spec.RestartPolicy, logical.Template.Spec.RestartPolicy)
			assert.Equal(t, physical.BackoffLimit, logical.BackoffLimit)
			assert.Equal(t, physical.ActiveDeadlineSeconds, logical.ActiveDeadlineSeconds)
			assert.Equal(t, physical.Selector, logical.Selector)

			// building a logical job spec must not change what the physical one returns
			physicalAgain, err := bcp.JobSpec(cr.Spec, cluster, job, "init-image")
			require.NoError(t, err)
			assert.Equal(t, physical, physicalAgain)
		})
	}
}
