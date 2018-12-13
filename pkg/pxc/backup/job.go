package backup

// import (
// 	"reflect"

// 	"github.com/operator-framework/operator-sdk/pkg/sdk"
// 	batchv1 "k8s.io/api/batch/v1"
// 	corev1 "k8s.io/api/core/v1"

// 	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
// )

// type Jobster interface {
// 	Create(spec api.PXCBackupSpec) error
// 	UpdateStatus(cr *api.PerconaXtraDBBackup)
// }

// func jobSpec(spec api.PXCBackupSpec, name string) batchv1.JobSpec {
// 	pvc := corev1.Volume{
// 		Name: spec.PXCCluster + "-backup-" + name,
// 	}
// 	pvc.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
// 		ClaimName: spec.PXCCluster + volumeNamePostfix + "." + name,
// 	}

// 	return batchv1.JobSpec{
// 		Template: corev1.PodTemplateSpec{
// 			Spec: corev1.PodSpec{
// 				Containers: []corev1.Container{
// 					{
// 						Name:    "xtrabackup",
// 						Image:   "perconalab/backupjob-openshift",
// 						Command: []string{"bash", "/usr/bin/backup.sh"},
// 						VolumeMounts: []corev1.VolumeMount{
// 							{
// 								Name:      pvc.Name,
// 								MountPath: "/backup",
// 							},
// 						},
// 						Env: []corev1.EnvVar{
// 							{
// 								Name:  "NODE_NAME",
// 								Value: spec.PXCCluster + "-pxc-nodes",
// 							},
// 						},
// 					},
// 				},
// 				RestartPolicy: corev1.RestartPolicyNever,
// 				Volumes: []corev1.Volume{
// 					pvc,
// 				},
// 			},
// 		},
// 	}
// }

// func updateStatus(bcp *api.PerconaXtraDBBackup, status *api.PXCBackupStatus) error {
// 	// don't update the status if there aren't any changes.
// 	if reflect.DeepEqual(bcp.Status, *status) {
// 		return nil
// 	}
// 	bcp.Status = *status
// 	return sdk.Update(bcp)
// }
