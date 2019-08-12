package backup

import (
	"strings"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

func PVCRestoreService(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace: bcp.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			},
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port: 3307,
					Name: "ncat",
				},
			},
		},
	}
}

func PVCRestorePod(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, pvcName string, cluster api.PerconaXtraDBClusterSpec) *corev1.Pod {
	podPVC := corev1.Volume{
		Name: "backup",
	}
	podPVC.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: pvcName,
	}

	podPVCs := []corev1.Volume{
		podPVC,
		app.GetSecretVolumes("ssl-internal", cluster.PXC.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", cluster.PXC.SSLSecretName, cluster.PXC.AllowUnsafeConfig),
	}
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace: bcp.Namespace,
			Labels: map[string]string{
				"name": "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			},
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: cluster.Backup.ImagePullSecrets,
			Containers: []corev1.Container{
				{
					Name:            "ncat",
					Image:           cluster.Backup.Image,
					ImagePullPolicy: corev1.PullAlways,
					Command:         []string{"recovery-pvc-donor.sh"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "backup",
							MountPath: "/backup",
						},
						{
							Name:      "ssl",
							MountPath: "/etc/mysql/ssl",
						},
						{
							Name:      "ssl-internal",
							MountPath: "/etc/mysql/ssl-internal",
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
			Volumes:       podPVCs,
		},
	}
}

func PVCRestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, cluster api.PerconaXtraDBClusterSpec) *batchv1.Job {
	nodeSelector := make(map[string]string)
	if val, ok := cluster.Backup.Storages[bcp.Spec.StorageName]; ok {
		nodeSelector = val.NodeSelector
	}

	jobPVC := corev1.Volume{
		Name: "datadir",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "datadir-" + bcp.Spec.PXCCluster + "-pxc-0",
			},
		},
	}

	jobPVCs := []corev1.Volume{
		jobPVC,
		app.GetSecretVolumes("ssl-internal", cluster.PXC.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", cluster.PXC.SSLSecretName, cluster.PXC.AllowUnsafeConfig),
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-job-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace: bcp.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Backup.ImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Backup.Image,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"recovery-pvc-joiner.sh"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/datadir",
								},
								{
									Name:      "ssl",
									MountPath: "/etc/mysql/ssl",
								},
								{
									Name:      "ssl-internal",
									MountPath: "/etc/mysql/ssl-internal",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "RESTORE_SRC_SERVICE",
									Value: "restore-src-" + cr.Name + "-" + bcp.Spec.PXCCluster,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       jobPVCs,
					NodeSelector:  nodeSelector,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}

	useMem, k8sq, err := xbMemoryUse(cluster)

	if useMem != "" && err == nil {
		job.Spec.Template.Spec.Containers[0].Env = append(
			job.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "XB_USE_MEMORY",
				Value: useMem,
			},
		)
		job.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: k8sq,
		}
	}

	return job
}

// S3RestoreJob returns restore job object for s3
func S3RestoreJob(cr *api.PerconaXtraDBClusterRestore, bcp *api.PerconaXtraDBClusterBackup, s3dest string, cluster api.PerconaXtraDBClusterSpec) (*batchv1.Job, error) {
	nodeSelector := make(map[string]string)
	if val, ok := cluster.Backup.Storages[bcp.Spec.StorageName]; ok {
		nodeSelector = val.NodeSelector
	}

	if bcp.Status.S3 == nil {
		return nil, errors.New("nil s3 backup status")
	}

	jobPVC := corev1.Volume{
		Name: "datadir",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: "datadir-" + bcp.Spec.PXCCluster + "-pxc-0",
			},
		},
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-job-" + cr.Name + "-" + bcp.Spec.PXCCluster,
			Namespace: bcp.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ImagePullSecrets: cluster.Backup.ImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           cluster.Backup.Image,
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"recovery-s3.sh"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/datadir",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "S3_BUCKET_URL",
									Value: s3dest,
								},
								{
									Name:  "ENDPOINT",
									Value: bcp.Status.S3.EndpointURL,
								},
								{
									Name:  "DEFAULT_REGION",
									Value: bcp.Status.S3.Region,
								},
								{
									Name: "ACCESS_KEY_ID",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: bcp.Status.S3.CredentialsSecret,
											},
											Key: "AWS_ACCESS_KEY_ID",
										},
									},
								},
								{
									Name: "SECRET_ACCESS_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: bcp.Status.S3.CredentialsSecret,
											},
											Key: "AWS_SECRET_ACCESS_KEY",
										},
									},
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						jobPVC,
					},
					NodeSelector: nodeSelector,
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}

	useMem, k8sq, err := xbMemoryUse(cluster)

	if useMem != "" && err == nil {
		job.Spec.Template.Spec.Containers[0].Env = append(
			job.Spec.Template.Spec.Containers[0].Env,
			corev1.EnvVar{
				Name:  "XB_USE_MEMORY",
				Value: useMem,
			},
		)
		job.Spec.Template.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: k8sq,
		}
	}

	return job, nil
}

func xbMemoryUse(cluster api.PerconaXtraDBClusterSpec) (useMem string, k8sQuantity resource.Quantity, err error) {
	if cluster.PXC.Resources != nil {
		if cluster.PXC.Resources.Requests != nil {
			useMem = cluster.PXC.Resources.Requests.Memory
			k8sQuantity, err = resource.ParseQuantity(cluster.PXC.Resources.Requests.Memory)
		}

		if cluster.PXC.Resources.Limits != nil && cluster.PXC.Resources.Limits.Memory != "" {
			useMem = cluster.PXC.Resources.Limits.Memory
			k8sQuantity, err = resource.ParseQuantity(cluster.PXC.Resources.Limits.Memory)
		}

		// make the 90% value
		q := k8sQuantity.DeepCopy()
		q.Sub(*resource.NewQuantity(k8sQuantity.Value()/10, k8sQuantity.Format))
		useMem = q.String()
		// transform Gi/Mi/etc to G/M
		if strings.Contains(useMem, "i") {
			useMem = strings.Replace(useMem, "i", "", -1)
		}
	}

	return useMem, k8sQuantity, err
}
