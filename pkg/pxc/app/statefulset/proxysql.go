package statefulset

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

const (
	proxyName           = "proxysql"
	proxyDataVolumeName = "proxydata"
)

type Proxy struct {
	sfs    *appsv1.StatefulSet
	lables map[string]string
}

func NewProxy(cr *api.PerconaXtraDBCluster) *Proxy {
	sfs := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + app.Name + "-" + proxyName,
			Namespace: cr.Namespace,
		},
	}

	lables := map[string]string{
		"app":       app.Name,
		"component": cr.Name + "-" + app.Name + "-proxysql",
		"cluster":   cr.Name,
	}

	return &Proxy{
		sfs:    sfs,
		lables: lables,
	}
}

func (c *Proxy) AppContainer(spec *api.PodSpec, secrets string) corev1.Container {
	appc := corev1.Container{
		Name:            proxyName,
		Image:           spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3306,
				Name:          "mysql",
			},
			{
				ContainerPort: 6032,
				Name:          "proxyadm",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      proxyDataVolumeName,
				MountPath: "/var/lib/proxysql",
				SubPath:   "data",
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
				Name:  "PROXY_ADMIN_USER",
				Value: "proxyadmin",
			},
			{
				Name: "PROXY_ADMIN_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "proxyadmin"),
				},
			},
			{
				Name:  "MYSQL_PROXY_USER",
				Value: "proxyuser",
			},
			{
				Name: "MYSQL_PROXY_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "proxyuser"),
				},
			},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
				},
			},
			{
				Name:  "PXCSERVICE",
				Value: c.lables["cluster"] + "-" + c.lables["app"] + "-nodes",
			},
		},
	}

	return appc
}

func (c *Proxy) PMMContainer(spec *api.PMMSpec, secrets string) corev1.Container {
	ct := app.PMMClient(spec, secrets)

	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "DB_TYPE",
			Value: "proxysql",
		},
		{
			Name:  "MONITOR_USER",
			Value: "monitor",
		},
		{
			Name: "MONITOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
			},
		},
		{
			Name:  "DB_ARGS",
			Value: "--dsn $(MONITOR_USER):$(MONITOR_PASSWORD)@tcp(localhost:6032)/",
		},
	}
	ct.Env = append(ct.Env, pmmEnvs...)

	return ct
}

func (c *Proxy) Resources(spec *api.PodResources) (corev1.ResourceRequirements, error) {
	return app.CreateResources(spec)
}

func (c *Proxy) Volumes(podSpec *api.PodSpec) *api.Volume {
	var (
		volume     api.Volume
		dataVolume corev1.VolumeSource
	)

	// 1. check whether PVC is existed
	if podSpec.VolumeSpec.PersistentVolumeClaim != nil {
		pvcs := app.PVCs(proxyDataVolumeName, &podSpec.VolumeSpec)
		volume.PVCs = pvcs
		return &volume
	}

	// 2. check whether hostPath is existed.
	if podSpec.VolumeSpec.HostPath != nil {
		dataVolume.HostPath = podSpec.VolumeSpec.HostPath
	}

	// 3. check whether emptyDir is existed.
	if podSpec.VolumeSpec.EmptyDir != nil {
		dataVolume.EmptyDir = podSpec.VolumeSpec.EmptyDir
	}

	volume.Volumes = []corev1.Volume{
		corev1.Volume{
			VolumeSource: dataVolume,
			Name:         proxyDataVolumeName,
		},
	}

	return &volume
}

func (c *Proxy) StatefulSet() *appsv1.StatefulSet {
	return c.sfs
}

func (c *Proxy) Lables() map[string]string {
	return c.lables
}
