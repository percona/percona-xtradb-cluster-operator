package deployment

import (
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/pkg/errors"
)

func GetBinlogCollectorDeployment(cr *api.PerconaXtraDBCluster) (appsv1.Deployment, error) {
	storage := cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName]
	binlogCollectorName := GetBinlogCollectorDeploymentName(cr)
	pxcUser := "xtrabackup"
	sleepTime := fmt.Sprintf("%.2f", cr.Spec.Backup.PITR.TimeBetweenUploads)

	bufferSize, err := getBufferSize(cr.Spec)
	if err != nil {
		return appsv1.Deployment{}, errors.Wrap(err, "get buffer size")
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/component":  "pitr",
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}
	for key, value := range cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].Labels {
		labels[key] = value
	}
	if storage.S3 == nil {
		return appsv1.Deployment{}, errors.New("s3 storage is not specified")
	}
	envs := []corev1.EnvVar{
		{
			Name: "SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(storage.S3.CredentialsSecret, "AWS_SECRET_ACCESS_KEY"),
			},
		},
		{
			Name: "ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(storage.S3.CredentialsSecret, "AWS_ACCESS_KEY_ID"),
			},
		},
		{
			Name:  "S3_BUCKET_URL",
			Value: storage.S3.Bucket,
		},
		{
			Name:  "PXC_SERVICE",
			Value: cr.Name + "-pxc",
		},
		{
			Name:  "PXC_USER",
			Value: pxcUser,
		},
		{
			Name: "PXC_PASS",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(cr.Spec.SecretsName, pxcUser),
			},
		},
		{
			Name:  "DEFAULT_REGION",
			Value: cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].S3.Region,
		},
		{
			Name:  "COLLECT_SPAN_SEC",
			Value: sleepTime,
		},
		{
			Name:  "BUFFER_SIZE",
			Value: strconv.FormatInt(bufferSize, 10),
		},
	}
	if len(storage.S3.EndpointURL) > 0 {
		envs = append(envs, corev1.EnvVar{
			Name:  "ENDPOINT",
			Value: storage.S3.EndpointURL,
		})
	}
	container := corev1.Container{
		Name:            "pitr",
		Image:           cr.Spec.Backup.Image,
		ImagePullPolicy: cr.Spec.Backup.ImagePullPolicy,
		Env:             envs,
		SecurityContext: cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].ContainerSecurityContext,
		Command:         []string{"pitr"},
		Resources:       cr.Spec.Backup.PITR.Resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "mysql-users-secret-file",
				MountPath: "/etc/mysql/mysql-users-secret",
			},
		},
	}
	replicas := int32(1)

	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      binlogCollectorName,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        binlogCollectorName,
					Namespace:   cr.Namespace,
					Labels:      labels,
					Annotations: cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].Annotations,
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					ImagePullSecrets:   cr.Spec.Backup.ImagePullSecrets,
					ServiceAccountName: cr.Spec.Backup.ServiceAccountName,
					SecurityContext:    cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].PodSecurityContext,
					Affinity:           cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].Affinity,
					Tolerations:        cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].Tolerations,
					NodeSelector:       cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].NodeSelector,
					SchedulerName:      cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].SchedulerName,
					PriorityClassName:  cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].PriorityClassName,
					Volumes: []corev1.Volume{
						app.GetSecretVolumes("mysql-users-secret-file", "internal-"+cr.Name, false),
					},
					RuntimeClassName: cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].RuntimeClassName,
				},
			},
		},
	}, nil
}

func GetBinlogCollectorDeploymentName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + "-pitr"
}

func getBufferSize(cluster api.PerconaXtraDBClusterSpec) (mem int64, err error) {
	res := cluster.Backup.PITR.Resources
	if res.Size() == 0 {
		return 0, nil
	}

	var memory *resource.Quantity

	if _, ok := res.Requests[corev1.ResourceMemory]; ok {
		memory = res.Requests.Memory()
	}

	if _, ok := res.Limits[corev1.ResourceMemory]; ok {
		memory = res.Limits.Memory()
	}

	return memory.Value() / int64(100) * int64(75), nil
}
