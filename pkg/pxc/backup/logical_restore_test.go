package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

func TestLogicalRestoreVolumeSize(t *testing.T) {
	const (
		mi = int64(1) << 20
		gi = int64(1) << 30
	)

	tests := map[string]struct {
		dumpSize int64
		override *resource.Quantity
		expected resource.Quantity
	}{
		"unknown dump size gets the minimum": {
			dumpSize: 0,
			expected: resource.MustParse("1Gi"),
		},
		"small dump gets the minimum": {
			dumpSize: 100 * mi,
			expected: resource.MustParse("1Gi"),
		},
		"dump just under the minimum after headroom gets the minimum": {
			dumpSize: 900 * mi,
			expected: resource.MustParse("1Gi"),
		},
		"large dump gets 10% headroom": {
			dumpSize: 10 * gi,
			expected: resource.MustParse("11Gi"),
		},
		"headroom is rounded up to a whole byte": {
			dumpSize: 2*gi + 1,
			// 2Gi + 1 byte, plus a tenth of that rounded up
			expected: *resource.NewQuantity(2*gi+1+214748365, resource.BinarySI),
		},
		"override is used for a large dump": {
			dumpSize: 10 * gi,
			override: new(resource.MustParse("50Gi")),
			expected: resource.MustParse("50Gi"),
		},
		"override is used when the dump size is unknown": {
			dumpSize: 0,
			override: new(resource.MustParse("2Gi")),
			expected: resource.MustParse("2Gi"),
		},
		"override smaller than the dump is still used": {
			dumpSize: 10 * gi,
			override: new(resource.MustParse("5Gi")),
			expected: resource.MustParse("5Gi"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := LogicalRestoreVolumeSize(tt.dumpSize, tt.override)
			assert.Zero(t, tt.expected.Cmp(got), "expected %s, got %s", tt.expected.String(), got.String())
		})
	}
}

func TestLogicalRestoreJob(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, pxcv1.SchemeBuilder.AddToScheme(scheme))

	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
	}

	tests := map[string]struct {
		restoreSpec pxcv1.PerconaXtraDBClusterRestoreSpec
		caBundle    *corev1.SecretKeySelector
		volumeSize  resource.Quantity

		expectedStorageClass *string
		// expectedMyloaderArgs is the expected value of MYLOADER_EXTRA_ARGS.
		// Empty means the variable must not be set.
		expectedMyloaderArgs string
	}{
		"defaults": {
			restoreSpec: pxcv1.PerconaXtraDBClusterRestoreSpec{},
			volumeSize:  resource.MustParse("1Gi"),
		},
		"storage class, myloader args and resources": {
			restoreSpec: pxcv1.PerconaXtraDBClusterRestoreSpec{
				Job: &pxcv1.RestoreJobSpec{
					Volume: &pxcv1.RestoreJobVolume{StorageClassName: new("fast")},
				},
				ContainerOptions: &pxcv1.BackupContainerOptions{
					Args: pxcv1.BackupContainerArgs{Myloader: []string{"--threads=2", "--optimize-keys=SKIP"}},
				},
				Resources: resources,
			},
			volumeSize:           resource.MustParse("11Gi"),
			expectedStorageClass: new("fast"),
			expectedMyloaderArgs: "--threads=2 --optimize-keys=SKIP",
		},
		"job volume without a storage class": {
			restoreSpec: pxcv1.PerconaXtraDBClusterRestoreSpec{
				Job: &pxcv1.RestoreJobSpec{
					Volume: &pxcv1.RestoreJobVolume{Size: new(resource.MustParse("50Gi"))},
				},
			},
			volumeSize: resource.MustParse("50Gi"),
		},
		"storage with a ca bundle": {
			restoreSpec: pxcv1.PerconaXtraDBClusterRestoreSpec{},
			caBundle: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "s3-ca"},
				Key:                  "ca.crt",
			},
			volumeSize: resource.MustParse("1Gi"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cluster := newLogicalTestCluster(t, nil)

			bcp := &pxcv1.PerconaXtraDBClusterBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test-ns",
				},
				Spec: pxcv1.PXCBackupSpec{
					PXCCluster:  "test-cluster",
					StorageName: logicalTestStorageName,
					Type:        pxcv1.BackupTypeLogical,
				},
				Status: pxcv1.PXCBackupStatus{
					Type:        pxcv1.BackupTypeLogical,
					State:       pxcv1.BackupSucceeded,
					StorageName: logicalTestStorageName,
					StorageType: pxcv1.BackupStorageS3,
					Destination: "s3://operator-testing/test-cluster-2026-11-10-10:00:00-logical",
					S3: &pxcv1.BackupStorageS3Spec{
						Bucket:            "operator-testing",
						Region:            "us-west-1",
						CredentialsSecret: "test-secret",
						CABundle:          tt.caBundle,
					},
				},
			}

			cr := &pxcv1.PerconaXtraDBClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-restore",
					Namespace: "test-ns",
					UID:       "restore-uid",
				},
				Spec: tt.restoreSpec,
			}
			cr.Spec.PXCCluster = "test-cluster"
			cr.Spec.BackupName = "test-backup"

			job, err := LogicalRestoreJob(t.Context(), cr, bcp, cluster, "init-image", scheme, tt.volumeSize)
			require.NoError(t, err)

			// identity and ownership
			assert.Equal(t, naming.RestoreJobName(cr, false), job.Name)
			assert.Equal(t, "test-ns", job.Namespace)
			assert.Equal(t, naming.LabelsRestoreJob(cluster, job.Name, logicalTestStorageName), job.Labels)
			assert.Contains(t, job.Finalizers, naming.FinalizerKeepJob)
			require.Len(t, job.OwnerReferences, 1)
			assert.Equal(t, cr.Name, job.OwnerReferences[0].Name)
			assert.Equal(t, cr.UID, job.OwnerReferences[0].UID)

			// container
			podSpec := job.Spec.Template.Spec
			require.Len(t, podSpec.Containers, 1)
			container := podSpec.Containers[0]
			assert.Equal(t, "myloader", container.Name)
			assert.Equal(t, []string{"bash", "/opt/percona/backup/logical-restore.sh"}, container.Command)
			assert.Equal(t, cluster.Spec.Backup.ImagePullPolicy, container.ImagePullPolicy)
			assert.Equal(t, cluster.Spec.PXC.ContainerSecurityContext, container.SecurityContext)
			for _, env := range container.Env {
				assert.NotEqual(t, "XB_USE_MEMORY", env.Name, "XB_USE_MEMORY is an xtrabackup setting")
			}
			assert.Equal(t, cluster.Spec.Backup.Image, container.Image)
			assert.Equal(t, tt.restoreSpec.Resources, container.Resources)
			assert.Equal(t, corev1.RestartPolicyNever, podSpec.RestartPolicy)

			require.Len(t, podSpec.InitContainers, 1)
			assert.Equal(t, "init-image", podSpec.InitContainers[0].Image)

			// environment
			assert.Contains(t, container.Env, corev1.EnvVar{Name: "PXC_HOST", Value: "test-cluster-pxc-0.test-cluster-pxc"})
			assert.Contains(t, container.Env, corev1.EnvVar{Name: "SCRATCH_DIR", Value: "/scratch"})
			assert.Contains(t, container.Env, corev1.EnvVar{
				Name:  "S3_BUCKET_URL",
				Value: "operator-testing/test-cluster-2026-11-10-10:00:00-logical",
			})
			assert.Contains(t, container.Env, corev1.EnvVar{Name: "VERIFY_TLS", Value: "true"})

			envNames := make(map[string]string)
			for _, env := range container.Env {
				envNames[env.Name] = env.Value
			}
			assert.Contains(t, envNames, "PXC_PASS")
			assert.Contains(t, envNames, "ACCESS_KEY_ID")
			assert.Contains(t, envNames, "SECRET_ACCESS_KEY")
			assert.Equal(t, tt.expectedMyloaderArgs, envNames["MYLOADER_EXTRA_ARGS"])

			// scratch volume: one generic ephemeral volume, mounted at /scratch
			assert.Contains(t, container.VolumeMounts, corev1.VolumeMount{Name: "logical-scratch", MountPath: "/scratch"})

			var ephemeral []corev1.Volume
			for _, volume := range podSpec.Volumes {
				if volume.Ephemeral != nil {
					ephemeral = append(ephemeral, volume)
				}
				// the dump must never land on a database volume
				assert.Nil(t, volume.PersistentVolumeClaim, "volume %s mounts an existing claim", volume.Name)
			}
			require.Len(t, ephemeral, 1)
			assert.Equal(t, "logical-scratch", ephemeral[0].Name)

			require.NotNil(t, ephemeral[0].Ephemeral.VolumeClaimTemplate)
			claim := ephemeral[0].Ephemeral.VolumeClaimTemplate.Spec
			assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, claim.AccessModes)
			assert.Equal(t, tt.expectedStorageClass, claim.StorageClassName)
			requested := claim.Resources.Requests[corev1.ResourceStorage]
			assert.Zero(t, tt.volumeSize.Cmp(requested), "expected %s, got %s", tt.volumeSize.String(), requested.String())

			// ca bundle for the storage
			hasCABundle := false
			for _, volume := range podSpec.Volumes {
				if volume.Secret != nil && tt.caBundle != nil && volume.Secret.SecretName == tt.caBundle.Name {
					hasCABundle = true
				}
			}
			assert.Equal(t, tt.caBundle != nil, hasCABundle)
		})
	}
}
