package statefulset

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	app "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app"
)

const (
	nodeName = "node"
)

type Node struct {
	sfs    *appsv1.StatefulSet
	lables map[string]string
}

func NewNode(cr *api.PerconaXtraDBCluster) *Node {
	sfs := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + app.Name + "-" + nodeName,
			Namespace: cr.Namespace,
		},
	}

	lables := map[string]string{
		"app":       app.Name,
		"component": cr.Name + "-" + app.Name + "-nodes",
		"cluster":   cr.Name,
	}

	return &Node{
		sfs:    sfs,
		lables: lables,
	}
}

func (c *Node) AppContainer(spec *api.PodSpec, secrets string) corev1.Container {
	appc := corev1.Container{
		Name:            nodeName,
		Image:           spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		ReadinessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: 15,
			TimeoutSeconds:      15,
			PeriodSeconds:       30,
			FailureThreshold:    5,
		}, "/usr/bin/clustercheck.sh"),
		LivenessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: 300,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
		}, "/usr/bin/clustercheck.sh"),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3306,
				Name:          "mysql",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "datadir",
				MountPath: "/var/lib/mysql",
			},
			{
				Name:      "config-volume",
				MountPath: "/etc/mysql/conf.d/",
			},
		},
		Env: []corev1.EnvVar{
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

func (c *Node) PMMContainer(spec *api.PMMSpec, secrets string) corev1.Container {
	ct := app.PMMClient(spec, secrets)

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
	ct.Env = append(ct.Env, pmmEnvs...)

	ct.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "datadir",
			MountPath: "/var/lib/mysql",
		},
	}

	return ct
}

func (c *Node) Resources(spec *api.PodResources) (corev1.ResourceRequirements, error) {
	return app.CreateResources(spec)
}

func (c *Node) PVCs(spec *api.PodVolumeSpec) ([]corev1.PersistentVolumeClaim, error) {
	return app.PVCs("datadir", spec)
}

func (c *Node) StatefulSet() *appsv1.StatefulSet {
	return c.sfs
}

func (c *Node) Lables() map[string]string {
	return c.lables
}
