package backup

import (
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func PVCRestoreService(cr *api.PerconaXtraDBBackupRestore, bcp *api.PerconaXtraDBBackup) *corev1.Service {
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

func PVCRestorePod(cr *api.PerconaXtraDBBackupRestore, bcp *api.PerconaXtraDBBackup, pvcName string) *corev1.Pod {
	podPVC := corev1.Volume{
		Name: "backup",
	}
	podPVC.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: pvcName,
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
			Containers: []corev1.Container{
				{
					Name:            "ncat",
					Image:           "percona/percona-xtradb-cluster-operator:0.3.0-backup",
					ImagePullPolicy: corev1.PullAlways,
					Command: []string{
						"bash",
						"-exc",
						"cat /backup/xtrabackup.stream | ncat -l --send-only 3307",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "backup",
							MountPath: "/backup",
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
			Volumes: []corev1.Volume{
				podPVC,
			},
		},
	}
}

func PVCRestoreJob(cr *api.PerconaXtraDBBackupRestore, bcp *api.PerconaXtraDBBackup) *batchv1.Job {
	jobPVC := corev1.Volume{
		Name: "datadir",
	}
	jobPVC.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: "datadir-" + bcp.Spec.PXCCluster + "-pxc-0",
	}
	return &batchv1.Job{
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
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           "percona/percona-xtradb-cluster-operator:0.3.0-backup",
							ImagePullPolicy: corev1.PullAlways,
							Command: []string{
								"bash",
								"-exc",
								`ping -c1 restore-src-` + cr.Name + "-" + bcp.Spec.PXCCluster + ` || :
								 rm -rf /datadir/*
								 ncat restore-src-` + cr.Name + "-" + bcp.Spec.PXCCluster + ` 3307 | xbstream -x -C /datadir
								 xtrabackup --prepare --target-dir=/datadir`,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/datadir",
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						jobPVC,
					},
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}
}

// S3RestoreJob returns restore job object for s3
func S3RestoreJob(cr *api.PerconaXtraDBBackupRestore, bcp *api.PerconaXtraDBBackup, s3dest string) (*batchv1.Job, error) {
	if bcp.Status.S3 == nil {
		return nil, errors.New("nil s3 backup status")
	}

	jobPVC := corev1.Volume{
		Name: "datadir",
	}
	jobPVC.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: "datadir-" + bcp.Spec.PXCCluster + "-pxc-0",
	}

	return &batchv1.Job{
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
					Containers: []corev1.Container{
						{
							Name:            "xtrabackup",
							Image:           "percona/percona-xtradb-cluster-operator:0.3.0-backup",
							ImagePullPolicy: corev1.PullAlways,
							Command: []string{
								"bash",
								"-exc",
								`mc -C /tmp/mc config host add dest "${AWS_ENDPOINT_URL:-https://s3.amazonaws.com}" "$AWS_ACCESS_KEY_ID" "$AWS_SECRET_ACCESS_KEY"
								 mc -C /tmp/mc ls dest/` + s3dest + `
								 rm -rf /datadir/*
								 mc -C /tmp/mc cat dest/` + s3dest + ` | xbstream -x -C /datadir
								 xtrabackup --prepare --target-dir=/datadir`,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/datadir",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "AWS_ENDPOINT_URL",
									Value: bcp.Status.S3.EndpointURL,
								},
								{
									Name: "AWS_ACCESS_KEY_ID",
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
									Name: "AWS_SECRET_ACCESS_KEY",
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
				},
			},
			BackoffLimit: func(i int32) *int32 { return &i }(4),
		},
	}, nil
}
