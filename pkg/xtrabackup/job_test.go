package xtrabackup

import (
	"testing"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestJobSpec(t *testing.T) {
	activeDeadlineSeconds := int64(3600)
	storageName := "s3-storage"
	verifyTLS := true
	initImage := "percona/percona-xtradb-cluster-operator:init-image"
	primaryPodHost := "cluster-pxc-0.cluster-pxc"
	backupImage := "percona/percona-xtradb-cluster-operator:backup-image"
	serviceAccountName := "backup-service-account"
	schedulerName := "custom-scheduler"
	priorityClassName := "high-priority"
	runtimeClassName := "gvisor"

	spec := &pxcv1.PXCBackupSpec{
		StorageName:           storageName,
		ActiveDeadlineSeconds: &activeDeadlineSeconds,
	}

	cluster := &pxcv1.PerconaXtraDBCluster{
		Spec: pxcv1.PerconaXtraDBClusterSpec{
			Backup: &pxcv1.PXCScheduledBackup{
				Image:           backupImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "backup-registry-secret"},
				},
				ServiceAccountName: serviceAccountName,
				Storages: map[string]*pxcv1.BackupStorageSpec{
					storageName: {
						Type: pxcv1.BackupStorageS3,
						S3: &pxcv1.BackupStorageS3Spec{
							Bucket:            "test-bucket",
							Region:            "us-west-2",
							CredentialsSecret: "s3-credentials",
						},
						VerifyTLS: &verifyTLS,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						ContainerSecurityContext: &corev1.SecurityContext{
							RunAsUser:  ptr.To(int64(1000)),
							RunAsGroup: ptr.To(int64(1000)),
							Privileged: ptr.To(false),
						},
						PodSecurityContext: &corev1.PodSecurityContext{
							RunAsUser:  ptr.To(int64(1000)),
							RunAsGroup: ptr.To(int64(1000)),
							FSGroup:    ptr.To(int64(1000)),
						},
						Annotations: map[string]string{
							"backup.annotation/key": "value",
						},
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "kubernetes.io/arch",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"amd64"},
												},
											},
										},
									},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "backup",
								Operator: corev1.TolerationOpEqual,
								Value:    "true",
								Effect:   corev1.TaintEffectNoSchedule,
							},
						},
						NodeSelector: map[string]string{
							"backup-node": "true",
						},
						SchedulerName:     schedulerName,
						PriorityClassName: priorityClassName,
						RuntimeClassName:  &runtimeClassName,
						TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
							{
								MaxSkew:           1,
								TopologyKey:       "kubernetes.io/hostname",
								WhenUnsatisfiable: corev1.DoNotSchedule,
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "backup",
									},
								},
							},
						},
					},
				},
			},
			InitContainer: pxcv1.InitContainerSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		},
	}

	jobLabels := map[string]string{
		"app":                     "percona-xtradb-cluster-backup",
		"cluster":                 "test-cluster",
		"backup-name":             "test-backup",
		"percona.com/backup-type": "manual",
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels: jobLabels,
		},
	}

	jobSpec, err := JobSpec(spec, cluster, job, initImage, primaryPodHost)
	assert.NoError(t, err)

	// Assert JobSpec fields
	assert.NotNil(t, jobSpec.ManualSelector)
	assert.True(t, *jobSpec.ManualSelector)
	assert.Equal(t, &activeDeadlineSeconds, jobSpec.ActiveDeadlineSeconds)
	assert.NotNil(t, jobSpec.Selector)
	assert.Equal(t, jobLabels, jobSpec.Selector.MatchLabels)

	// Assert PodTemplateSpec
	podTemplate := jobSpec.Template
	assert.Equal(t, jobLabels, podTemplate.Labels)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].Annotations, podTemplate.Annotations)

	// Assert PodSpec
	podSpec := podTemplate.Spec
	assert.Equal(t, corev1.RestartPolicyNever, podSpec.RestartPolicy)
	assert.Equal(t, cluster.Spec.Backup.ServiceAccountName, podSpec.ServiceAccountName)
	assert.Equal(t, cluster.Spec.Backup.ImagePullSecrets, podSpec.ImagePullSecrets)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].PodSecurityContext, podSpec.SecurityContext)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].Affinity, podSpec.Affinity)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].Tolerations, podSpec.Tolerations)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].NodeSelector, podSpec.NodeSelector)
	assert.Equal(t, schedulerName, podSpec.SchedulerName)
	assert.Equal(t, priorityClassName, podSpec.PriorityClassName)
	assert.Equal(t, &runtimeClassName, podSpec.RuntimeClassName)
	assert.NotNil(t, podSpec.TopologySpreadConstraints)
	assert.Len(t, podSpec.TopologySpreadConstraints, 1)

	// Assert Volumes
	assert.Len(t, podSpec.Volumes, 1)
	assert.Equal(t, app.BinVolumeName, podSpec.Volumes[0].Name)
	assert.NotNil(t, podSpec.Volumes[0].EmptyDir)

	// Assert InitContainers
	assert.Len(t, podSpec.InitContainers, 1)
	initContainer := podSpec.InitContainers[0]
	assert.Equal(t, "backup-init", initContainer.Name)
	assert.Equal(t, initImage, initContainer.Image)
	assert.Equal(t, cluster.Spec.Backup.ImagePullPolicy, initContainer.ImagePullPolicy)
	assert.Equal(t, []string{"/backup-init-entrypoint.sh"}, initContainer.Command)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].ContainerSecurityContext, initContainer.SecurityContext)
	assert.Len(t, initContainer.VolumeMounts, 1)
	assert.Equal(t, app.BinVolumeName, initContainer.VolumeMounts[0].Name)
	assert.Equal(t, app.BinVolumeMountPath, initContainer.VolumeMounts[0].MountPath)

	// Assert Containers
	assert.Len(t, podSpec.Containers, 1)
	container := podSpec.Containers[0]
	assert.Equal(t, "xtrabackup", container.Name)
	assert.Equal(t, backupImage, container.Image)
	assert.Equal(t, cluster.Spec.Backup.ImagePullPolicy, container.ImagePullPolicy)
	assert.Equal(t, []string{"/opt/percona/xtrabackup-run-backup"}, container.Command)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].Resources, container.Resources)
	assert.Equal(t, cluster.Spec.Backup.Storages[storageName].ContainerSecurityContext, container.SecurityContext)
	assert.Len(t, container.VolumeMounts, 1)
	assert.Equal(t, app.BinVolumeName, container.VolumeMounts[0].Name)
	assert.Equal(t, app.BinVolumeMountPath, container.VolumeMounts[0].MountPath)

	// Assert Environment Variables
	assert.Len(t, container.Env, 3)
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}
	assert.Equal(t, primaryPodHost, envMap["HOST"])
	assert.Equal(t, string(pxcv1.BackupStorageS3), envMap["STORAGE_TYPE"])
	assert.Equal(t, "true", envMap["VERIFY_TLS"])
}
