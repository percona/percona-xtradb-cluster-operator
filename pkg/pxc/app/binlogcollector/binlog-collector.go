package binlogcollector

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const gtidCacheKey = "gtid-binlog-cache.json"

func GetService(cr *api.PerconaXtraDBCluster) *corev1.Service {
	labels := naming.LabelsPITR(cr)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.BinlogCollectorServiceName(cr),
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port: 8080,
					Name: "http",
				},
			},
		},
	}
}

func GetDeployment(cr *api.PerconaXtraDBCluster, initImage string, existingMatchLabels map[string]string) (appsv1.Deployment, error) {
	binlogCollectorName := naming.BinlogCollectorDeploymentName(cr)
	pxcUser := users.Xtrabackup
	sleepTime := fmt.Sprintf("%.2f", cr.Spec.Backup.PITR.TimeBetweenUploads)

	bufferSize, err := getBufferSize(cr.Spec)
	if err != nil {
		return appsv1.Deployment{}, errors.Wrap(err, "get buffer size")
	}

	labels := naming.LabelsPITR(cr)
	if stg, ok := cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName]; ok {
		for key, value := range stg.Labels {
			labels[key] = value
		}
	}

	matchLabels := naming.LabelsPITR(cr)
	if len(existingMatchLabels) != 0 && !reflect.DeepEqual(existingMatchLabels, matchLabels) {
		matchLabels = existingMatchLabels
	}

	envs, err := getStorageEnvs(cr)
	if err != nil {
		return appsv1.Deployment{}, errors.Wrap(err, "get storage envs")
	}
	timeout := fmt.Sprintf("%.2f", cr.Spec.Backup.PITR.TimeoutSeconds)
	envs = append(envs, []corev1.EnvVar{
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
			Name:  "COLLECT_SPAN_SEC",
			Value: sleepTime,
		},
		{
			Name:  "BUFFER_SIZE",
			Value: strconv.FormatInt(bufferSize, 10),
		},
		{
			Name:  "TIMEOUT_SECONDS",
			Value: timeout,
		},
	}...)

	if cr.CompareVersionWith("1.17.0") >= 0 {
		envs = append(envs, corev1.EnvVar{
			Name:  "GTID_CACHE_KEY",
			Value: gtidCacheKey,
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

	if cr.CompareVersionWith("1.17.0") >= 0 {
		container.Ports = []corev1.ContainerPort{
			{
				ContainerPort: 8080,
				Name:          "metrics",
			},
		}
	}

	replicas := int32(1)

	var initContainers []corev1.Container
	volumes := []corev1.Volume{
		app.GetSecretVolumes("mysql-users-secret-file", "internal-"+cr.Name, false),
	}

	container.Command = []string{"/opt/percona/pitr"}
	initContainers = []corev1.Container{statefulset.PitrInitContainer(cr, initImage)}
	volumes = append(volumes,
		corev1.Volume{
			Name: app.BinVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	)

	container.VolumeMounts = append(container.VolumeMounts,
		corev1.VolumeMount{
			Name:      app.BinVolumeName,
			MountPath: app.BinVolumeMountPath,
		},
	)

	// Add CA bundle to the container, if specified
	storage, ok := cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName]
	if ok && storage.S3 != nil && storage.S3.CABundle != nil {
		sel := storage.S3.CABundle
		volumes = append(volumes,
			corev1.Volume{
				Name: "ca-bundle",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: sel.Name,
						Items: []corev1.KeyToPath{
							{
								Key:  sel.Key,
								Path: "ca.crt",
							},
						},
					},
				},
			},
		)
		container.VolumeMounts = append(container.VolumeMounts,
			corev1.VolumeMount{
				Name:      "ca-bundle",
				MountPath: "/tmp/s3/certs",
			},
		)
	}

	depl := appsv1.Deployment{
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
				MatchLabels: matchLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        binlogCollectorName,
					Namespace:   cr.Namespace,
					Labels:      labels,
					Annotations: cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].Annotations,
				},
				Spec: corev1.PodSpec{
					InitContainers:            initContainers,
					Containers:                []corev1.Container{container},
					ImagePullSecrets:          cr.Spec.Backup.ImagePullSecrets,
					ServiceAccountName:        cr.Spec.Backup.ServiceAccountName,
					SecurityContext:           cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].PodSecurityContext,
					Affinity:                  cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].Affinity,
					TopologySpreadConstraints: pxc.PodTopologySpreadConstraints(cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].TopologySpreadConstraints, labels),
					Tolerations:               cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].Tolerations,
					NodeSelector:              cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].NodeSelector,
					SchedulerName:             cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].SchedulerName,
					PriorityClassName:         cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].PriorityClassName,
					Volumes:                   volumes,
					RuntimeClassName:          cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName].RuntimeClassName,
				},
			},
		},
	}

	if cr.CompareVersionWith("1.18.0") < 0 {
		depl.Labels = labels
	}

	return depl, nil
}

func getStorageEnvs(cr *api.PerconaXtraDBCluster) ([]corev1.EnvVar, error) {
	storage, ok := cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName]
	if !ok {
		return nil, errors.Errorf("storage %s does not exist", cr.Spec.Backup.PITR.StorageName)
	}

	verifyTLS := "true"
	if storage.VerifyTLS != nil && !*storage.VerifyTLS {
		verifyTLS = "false"
	}
	var envs []corev1.EnvVar

	switch storage.Type {
	case api.BackupStorageS3:
		if storage.S3 == nil {
			return nil, errors.New("s3 storage is not specified")
		}
		envs = []corev1.EnvVar{
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
				Name:  "DEFAULT_REGION",
				Value: storage.S3.Region,
			},
			{
				Name:  "STORAGE_TYPE",
				Value: "s3",
			},
		}
		if len(storage.S3.EndpointURL) > 0 {
			envs = append(envs, corev1.EnvVar{
				Name:  "ENDPOINT",
				Value: storage.S3.EndpointURL,
			})
		}
	case api.BackupStorageAzure:
		if storage.Azure == nil {
			return nil, errors.New("azure storage is not specified")
		}
		envs = []corev1.EnvVar{
			{
				Name: "AZURE_STORAGE_ACCOUNT",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(storage.Azure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_NAME"),
				},
			},
			{
				Name: "AZURE_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(storage.Azure.CredentialsSecret, "AZURE_STORAGE_ACCOUNT_KEY"),
				},
			},
			{
				Name:  "AZURE_STORAGE_CLASS",
				Value: storage.Azure.StorageClass,
			},
			{
				Name:  "AZURE_CONTAINER_PATH",
				Value: storage.Azure.ContainerPath,
			},
			{
				Name:  "AZURE_ENDPOINT",
				Value: storage.Azure.Endpoint,
			},
			{
				Name:  "STORAGE_TYPE",
				Value: "azure",
			},
		}
	default:
		return nil, errors.Errorf("%s storage has unsupported type %s", cr.Spec.Backup.PITR.StorageName, storage.Type)
	}

	if cr.CompareVersionWith("1.13.0") >= 0 {
		envs = append(envs, corev1.EnvVar{
			Name:  "VERIFY_TLS",
			Value: verifyTLS,
		})
	}

	return envs, nil
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

func GetPod(ctx context.Context, c client.Client, cr *api.PerconaXtraDBCluster) (*corev1.Pod, error) {
	collectorPodList := corev1.PodList{}

	err := c.List(ctx, &collectorPodList,
		&client.ListOptions{
			Namespace:     cr.Namespace,
			LabelSelector: labels.SelectorFromSet(naming.LabelsPITR(cr)),
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "get binlog collector pods")
	}

	if len(collectorPodList.Items) < 1 {
		return nil, errors.New("no binlog collector pods")
	}

	return &collectorPodList.Items[0], nil
}

var GapFileNotFound = errors.New("gap file not found")

func RemoveGapFile(c *clientcmd.Client, pod *corev1.Pod) error {
	stderrBuf := &bytes.Buffer{}
	err := c.Exec(pod, "pitr", []string{"/bin/bash", "-c", "rm /tmp/gap-detected"}, nil, nil, stderrBuf, false)
	if err != nil {
		if strings.Contains(stderrBuf.String(), "No such file or directory") {
			return GapFileNotFound
		}
		return errors.Wrapf(err, "delete gap file in collector pod %s", pod.Name)
	}

	return nil
}

func RemoveTimelineFile(c *clientcmd.Client, pod *corev1.Pod) error {
	stderrBuf := &bytes.Buffer{}
	err := c.Exec(pod, "pitr", []string{"/bin/bash", "-c", "rm /tmp/pitr-timeline"}, nil, nil, stderrBuf, false)
	if err != nil {
		if strings.Contains(stderrBuf.String(), "No such file or directory") {
			return nil
		}
		return errors.Wrapf(err, "delete timeline file in collector pod %s", pod.Name)
	}

	return nil
}

func InvalidateCache(
	ctx context.Context,
	cl client.Client,
	cluster *api.PerconaXtraDBCluster,
) error {
	log := logf.FromContext(ctx)

	opts, err := storage.GetOptions(ctx, cl, cluster, cluster.Spec.Backup.PITR.StorageName)
	if err != nil {
		return errors.Wrap(err, "get pitr storage options")
	}

	stg, err := storage.NewClient(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "new storage client")
	}

	log.Info("invalidating binlog collector cache",
		"storage", cluster.Spec.Backup.PITR.StorageName,
		"file", gtidCacheKey)

	return stg.DeleteObject(ctx, gtidCacheKey)

}
