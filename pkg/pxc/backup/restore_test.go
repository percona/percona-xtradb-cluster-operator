package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/utils/ptr"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/test"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestPrepareJob(t *testing.T) {
	cluster := pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: pxcv1.PerconaXtraDBClusterSpec{
			CRVersion: version.Version(),
			Backup: &pxcv1.BackupSpec{
				Image: "percona/percona-xtrabackup:8.0",
				Storages: map[string]*pxcv1.BackupStorageSpec{
					"test-storage": {
						Type: pxcv1.BackupStorageS3,
						S3: &pxcv1.BackupStorageS3Spec{
							Bucket:            "operator-testing",
							Region:            "us-west-1",
							CredentialsSecret: "test-secret",
						},
					},
				},
			},
			PXC: &pxcv1.PXCSpec{
				PodSpec: &pxcv1.PodSpec{
					Size:     3,
					Image:    "percona/percona-xtradb-cluster:8.0",
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

	err := cluster.CheckNSetDefaults(sv, log)
	assert.NoError(t, err)

	backup := pxcv1.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "test-ns",
		},
		Spec: pxcv1.PXCBackupSpec{
			PXCCluster:  "test-cluster",
			StorageName: "test-storage",
		},
		Status: pxcv1.PXCBackupStatus{
			StorageName: "test-storage",
		},
	}

	restore := pxcv1.PerconaXtraDBClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-restore",
			Namespace: "test-ns",
		},
		Spec: pxcv1.PerconaXtraDBClusterRestoreSpec{
			PXCCluster: "test-cluster",
			BackupName: "test-backup",
		},
	}

	initImage := "perconalab/percona-xtradb-cluster-operator:main"

	cl := test.BuildFakeClient()

	job, err := PrepareJob(&restore, &backup, &cluster, initImage, cl.Scheme())
	assert.NoError(t, err)

	assert.Equal(t, job.Spec.Template.Spec.Containers[0].Image, "percona/percona-xtradb-cluster:8.0")
	assert.Equal(t, job.Spec.Template.Spec.Containers[0].Command, []string{"/var/lib/mysql/prepare_restored_cluster.sh"})

	expectedVolumes := []corev1.Volume{
		{
			Name: "datadir",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "datadir-test-cluster-pxc-0",
				},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "test-cluster-pxc",
					},
					Optional: ptr.To(true),
				},
			},
		},
		{
			Name: "mysql-users-secret-file",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "internal-test-cluster",
					Optional:   ptr.To(false),
				},
			},
		},
		{
			Name: "vault-keyring-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "test-cluster-vault",
					Optional:   ptr.To(true),
				},
			},
		},
		{
			Name: "ssl",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "test-cluster-ssl",
					Optional:   ptr.To(false),
				},
			},
		},
		{
			Name: "ssl-internal",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "test-cluster-ssl-internal",
					Optional:   ptr.To(false),
				},
			},
		},
	}
	assert.ElementsMatch(t, job.Spec.Template.Spec.Volumes, expectedVolumes)

	expectedVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "datadir",
			MountPath: "/var/lib/mysql",
		},
		{
			Name:      "config",
			MountPath: "/etc/percona-xtradb-cluster.conf.d",
		},
		{
			Name:      "mysql-users-secret-file",
			MountPath: "/etc/mysql/mysql-users-secret",
		},
		{
			Name:      statefulset.VaultSecretVolumeName,
			MountPath: statefulset.VaultSecretMountPath,
		},
		{
			Name:      "ssl",
			MountPath: "/etc/mysql/ssl",
		},
		{
			Name:      "ssl-internal",
			MountPath: "/etc/mysql/ssl-internal",
		},
	}
	assert.ElementsMatch(t, job.Spec.Template.Spec.Containers[0].VolumeMounts, expectedVolumeMounts)
}
