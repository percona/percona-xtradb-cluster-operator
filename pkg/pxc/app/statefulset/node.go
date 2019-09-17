package statefulset

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

const (
	DataVolumeName = "datadir"
)

type Node struct {
	sfs     *appsv1.StatefulSet
	labels  map[string]string
	service string
}

func NewNode(cr *api.PerconaXtraDBCluster) *Node {
	sfs := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + app.Name,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SchedulerName: cr.Spec.PXC.SchedulerName,
				},
			},
		},
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/component":  "pxc",
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}

	return &Node{
		sfs:     sfs,
		labels:  labels,
		service: cr.Name + "-" + app.Name,
	}
}

func (c *Node) AppContainer(spec *api.PodSpec, secrets string) corev1.Container {
	redinessDelay := int32(15)
	if spec.ReadinessInitialDelaySeconds != nil {
		redinessDelay = *spec.ReadinessInitialDelaySeconds
	}
	livenessDelay := int32(300)
	if spec.LivenessInitialDelaySeconds != nil {
		livenessDelay = *spec.LivenessInitialDelaySeconds
	}

	appc := corev1.Container{
		Name:            app.Name,
		Image:           spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		ReadinessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: redinessDelay,
			TimeoutSeconds:      15,
			PeriodSeconds:       30,
			FailureThreshold:    5,
		}, "/usr/bin/clustercheck.sh"),
		LivenessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: livenessDelay,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
		}, "/usr/bin/clustercheck.sh"),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3306,
				Name:          "mysql",
			},
			{
				ContainerPort: 4444,
				Name:          "sst",
			},
			{
				ContainerPort: 4567,
				Name:          "write-set",
			},
			{
				ContainerPort: 4568,
				Name:          "ist",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
			{
				Name:      "config",
				MountPath: "/etc/mysql/conf.d",
			},
			{
				Name:      "tmp",
				MountPath: "/tmp",
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
				Name:  "PXC_SERVICE",
				Value: c.labels["app.kubernetes.io/instance"] + "-" + c.labels["app.kubernetes.io/component"] + "-unready",
			},
			{
				Name:  "MONITOR_HOST",
				Value: "%",
			},
			{
				Name: "MYSQL_ROOT_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "root"),
				},
			},
			{
				Name: "XTRABACKUP_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "xtrabackup"),
				},
			},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
				},
			},
			{
				Name: "CLUSTERCHECK_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "clustercheck"),
				},
			},
		},
	}

	return appc
}

func (c *Node) SidecarContainers(spec *api.PodSpec, secrets string) []corev1.Container { return nil }

func (c *Node) PMMContainer(spec *api.PMMSpec, secrets string, v120OrGreater bool) corev1.Container {
	ct := app.PMMClient(spec, secrets, v120OrGreater)

	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "DB_TYPE",
			Value: "mysql",
		},
		{
			Name:  "DB_USER",
			Value: "monitor",
		},
		{
			Name: "DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
			},
		},
		{
			Name:  "DB_ARGS",
			Value: "--query-source=perfschema",
		},
	}

	clusterEnvs := []corev1.EnvVar{
		{
			Name:  "DB_CLUSTER",
			Value: app.Name,
		},
		{
			Name:  "DB_HOST",
			Value: "localhost",
		},
		{
			Name:  "DB_PORT",
			Value: "3306",
		},
	}

	ct.Env = append(ct.Env, pmmEnvs...)
	if v120OrGreater {
		ct.Env = append(ct.Env, clusterEnvs...)
	}

	ct.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      DataVolumeName,
			MountPath: "/var/lib/mysql",
		},
	}

	return ct
}

func (c *Node) Resources(spec *api.PodResources) (corev1.ResourceRequirements, error) {
	return app.CreateResources(spec)
}

func (c *Node) Volumes(podSpec *api.PodSpec) *api.Volume {
	vol := app.Volumes(podSpec, DataVolumeName)
	ls := c.Labels()
	vol.Volumes = append(
		vol.Volumes,
		app.GetTmpVolume(),
		app.GetConfigVolumes("config", ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"]),
		app.GetSecretVolumes("ssl-internal", podSpec.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", podSpec.SSLSecretName, podSpec.AllowUnsafeConfig))
	return vol
}

func (c *Node) StatefulSet() *appsv1.StatefulSet {
	return c.sfs
}

func (c *Node) Labels() map[string]string {
	return c.labels
}

func (c *Node) Service() string {
	return c.service
}
