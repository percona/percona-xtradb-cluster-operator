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
					Optional: new(true),
				},
			},
		},
		{
			Name: "mysql-users-secret-file",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "internal-test-cluster",
					Optional:   new(false),
				},
			},
		},
		{
			Name: "vault-keyring-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "test-cluster-vault",
					Optional:   new(true),
				},
			},
		},
		{
			Name: "ssl",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "test-cluster-ssl",
					Optional:   new(false),
				},
			},
		},
		{
			Name: "ssl-internal",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "test-cluster-ssl-internal",
					Optional:   new(false),
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

func TestRestoreJobEnvsIncrementals(t *testing.T) {
	const (
		namespace = "test-ns"
		baseName  = "c1-2026-11-03-10:00:00-full"
		incr1     = "c1-2026-11-03-11:00:00-incr"
		incr2     = "c1-2026-11-03-12:00:00-incr"
		incr3     = "c1-2026-11-03-13:00:00-incr"
	)

	cluster := &pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: namespace},
		Spec: pxcv1.PerconaXtraDBClusterSpec{
			CRVersion:   version.Version(),
			SecretsName: "c1-secrets",
			Backup: &pxcv1.BackupSpec{
				Image: "percona/percona-xtrabackup:8.0",
				Storages: map[string]*pxcv1.BackupStorageSpec{
					"s3-us-west": {Type: pxcv1.BackupStorageS3, S3: &pxcv1.BackupStorageS3Spec{
						Bucket: "my-bucket", CredentialsSecret: "s3-secret",
					}},
					"azure-blob": {Type: pxcv1.BackupStorageAzure, Azure: &pxcv1.BackupStorageAzureSpec{
						ContainerPath: "my-container", CredentialsSecret: "azure-secret",
					}},
				},
			},
			PXC: &pxcv1.PXCSpec{PodSpec: &pxcv1.PodSpec{
				Size:     3,
				Image:    "percona/percona-xtradb-cluster:8.0",
				Affinity: &pxcv1.PodAffinity{},
				VolumeSpec: &pxcv1.VolumeSpec{PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				}},
			}},
		},
	}
	require.NoError(t, cluster.CheckNSetDefaults(&version.ServerVersion{
		Platform: version.PlatformKubernetes, Info: k8sversion.Info{},
	}, log))

	s3Backup := &pxcv1.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "target-backup", Namespace: namespace},
		Spec:       pxcv1.PXCBackupSpec{PXCCluster: "c1", StorageName: "s3-us-west"},
		Status: pxcv1.PXCBackupStatus{
			StorageName: "s3-us-west",
			StorageType: pxcv1.BackupStorageS3,
			S3:          &pxcv1.BackupStorageS3Spec{Bucket: "my-bucket", CredentialsSecret: "s3-secret"},
		},
	}
	azureBackup := &pxcv1.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "target-backup", Namespace: namespace},
		Spec:       pxcv1.PXCBackupSpec{PXCCluster: "c1", StorageName: "azure-blob"},
		Status: pxcv1.PXCBackupStatus{
			StorageName: "azure-blob",
			StorageType: pxcv1.BackupStorageAzure,
			Azure: &pxcv1.BackupStorageAzureSpec{
				// container + prefix: azureEnvs derives BACKUP_PATH's prefix from
				// the recorded storage spec, not from the destination string.
				ContainerPath: "my-container/pfx", CredentialsSecret: "azure-secret",
			},
		},
	}

	restore := &pxcv1.PerconaXtraDBClusterRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: namespace},
		Spec:       pxcv1.PerconaXtraDBClusterRestoreSpec{PXCCluster: "c1", BackupName: "target-backup"},
	}

	tests := []struct {
		name         string
		bcp          *pxcv1.PerconaXtraDBClusterBackup
		destination  pxcv1.PXCBackupDestination
		incrementals []string
		// wantDestEnv is the env var carrying the backup path for this storage
		// type, and the value it must hold. For an incremental restore it must
		// still point at the chain's BASE backup.
		wantDestEnvName  string
		wantDestEnvValue string
		wantIncrementals string
		wantNoIncrEnv    bool
	}{
		{
			name:             "s3 full backup sets no incrementals env",
			bcp:              s3Backup,
			destination:      pxcv1.PXCBackupDestination("s3://my-bucket/pfx/" + baseName),
			incrementals:     nil,
			wantDestEnvName:  "S3_BUCKET_URL",
			wantDestEnvValue: "my-bucket/pfx/" + baseName,
			wantNoIncrEnv:    true,
		},
		{
			name:             "s3 full backup with an empty slice sets no incrementals env",
			bcp:              s3Backup,
			destination:      pxcv1.PXCBackupDestination("s3://my-bucket/pfx/" + baseName),
			incrementals:     []string{},
			wantDestEnvName:  "S3_BUCKET_URL",
			wantDestEnvValue: "my-bucket/pfx/" + baseName,
			wantNoIncrEnv:    true,
		},
		{
			name:        "s3 one incremental",
			bcp:         s3Backup,
			destination: pxcv1.PXCBackupDestination("s3://my-bucket/pfx/" + baseName),
			incrementals: []string{
				"pfx/" + baseName + ".incr/" + incr1,
			},
			wantDestEnvName:  "S3_BUCKET_URL",
			wantDestEnvValue: "my-bucket/pfx/" + baseName,
			wantIncrementals: "pfx/" + baseName + ".incr/" + incr1,
		},
		{
			name:        "s3 three incrementals are comma joined in order",
			bcp:         s3Backup,
			destination: pxcv1.PXCBackupDestination("s3://my-bucket/pfx/" + baseName),
			incrementals: []string{
				"pfx/" + baseName + ".incr/" + incr1,
				"pfx/" + baseName + ".incr/" + incr2,
				"pfx/" + baseName + ".incr/" + incr3,
			},
			wantDestEnvName:  "S3_BUCKET_URL",
			wantDestEnvValue: "my-bucket/pfx/" + baseName,
			wantIncrementals: "pfx/" + baseName + ".incr/" + incr1 + "," +
				"pfx/" + baseName + ".incr/" + incr2 + "," +
				"pfx/" + baseName + ".incr/" + incr3,
		},
		{
			name:             "azure full backup sets no incrementals env",
			bcp:              azureBackup,
			destination:      pxcv1.PXCBackupDestination("azure://my-container/pfx/" + baseName),
			incrementals:     nil,
			wantDestEnvName:  "BACKUP_PATH",
			wantDestEnvValue: "pfx/" + baseName,
			wantNoIncrEnv:    true,
		},
		{
			name:        "azure incrementals",
			bcp:         azureBackup,
			destination: pxcv1.PXCBackupDestination("azure://my-container/pfx/" + baseName),
			incrementals: []string{
				"pfx/" + baseName + ".incr/" + incr1,
				"pfx/" + baseName + ".incr/" + incr2,
			},
			wantDestEnvName:  "BACKUP_PATH",
			wantDestEnvValue: "pfx/" + baseName,
			wantIncrementals: "pfx/" + baseName + ".incr/" + incr1 + "," +
				"pfx/" + baseName + ".incr/" + incr2,
		},
	}

	envValue := func(envs []corev1.EnvVar, name string) (string, bool) {
		for _, e := range envs {
			if e.Name == name {
				return e.Value, true
			}
		}
		return "", false
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			envs, err := restoreJobEnvs(t.Context(), tc.bcp, restore, cluster, tc.destination, tc.incrementals, false)
			require.NoError(t, err)

			got, ok := envValue(envs, "BACKUP_INCREMENTALS_DEST")
			if tc.wantNoIncrEnv {
				// The restore script keys its whole prepare strategy off this
				// variable being non-empty, so it must be absent - not empty -
				// for every full-backup restore.
				assert.False(t, ok, "BACKUP_INCREMENTALS_DEST must not be set for a full backup")
			} else {
				require.True(t, ok, "BACKUP_INCREMENTALS_DEST must be set for an incremental restore")
				assert.Equal(t, tc.wantIncrementals, got)
			}

			// The storage destination env vars must keep carrying the BASE
			// backup: the script downloads the base from them and each
			// incremental from BACKUP_INCREMENTALS_DEST.
			destVal, ok := envValue(envs, tc.wantDestEnvName)
			require.True(t, ok, "%s must be set", tc.wantDestEnvName)
			assert.Equal(t, tc.wantDestEnvValue, destVal)
		})
	}
}
