package statefulset

import (
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	haproxyName           = "haproxy"
	haproxyDataVolumeName = "haproxydata"
)

type HAProxy struct {
	sfs     *appsv1.StatefulSet
	labels  map[string]string
	service string
}

func NewHAProxy(cr *api.PerconaXtraDBCluster) *HAProxy {
	sfs := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + haproxyName,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			PodManagementPolicy: "OrderedReady",
		},
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/component":  haproxyName,
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}

	return &HAProxy{
		sfs:     sfs,
		labels:  labels,
		service: cr.Name + "-" + haproxyName,
	}
}

func (c *HAProxy) AppContainer(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) (corev1.Container, error) {
	imagePullPolicy := spec.ImagePullPolicy
	if len(spec.ImagePullPolicy) == 0 {
		imagePullPolicy = corev1.PullAlways
	}
	appc := corev1.Container{
		Name:            haproxyName,
		Image:           spec.Image,
		ImagePullPolicy: imagePullPolicy,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3306,
				Name:          "mysql",
			},
			{
				ContainerPort: 3307,
				Name:          "mysql-replicas",
			},
			{
				ContainerPort: 3309,
				Name:          "proxy-protocol",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "haproxy-custom",
				MountPath: "/etc/haproxy-custom/",
			},
			{
				Name:      "haproxy-auto",
				MountPath: "/etc/haproxy/pxc",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "PXC_SERVICE",
				Value: c.labels["app.kubernetes.io/instance"] + "-" + "pxc",
			},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
				},
			},
		},
		SecurityContext: spec.ContainerSecurityContext,
	}

	res, err := app.CreateResources(spec.Resources)
	if err != nil {
		return appc, fmt.Errorf("create resources error: %v", err)
	}
	appc.Resources = res

	return appc, nil
}

func (c *HAProxy) SidecarContainers(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	res, err := app.CreateResources(spec.SidecarResources)
	if err != nil {
		return nil, fmt.Errorf("create sidecar resources error: %v", err)
	}
	imagePullPolicy := spec.ImagePullPolicy
	if len(spec.ImagePullPolicy) == 0 {
		imagePullPolicy = corev1.PullAlways
	}
	return []corev1.Container{
		{
			Name:            "pxc-monit",
			Image:           spec.Image,
			ImagePullPolicy: imagePullPolicy,
			Args: []string{
				"/usr/bin/peer-list",
				"-on-change=/usr/bin/add_pxc_nodes.sh",
				"-service=$(PXC_SERVICE)",
			},
			Env: []corev1.EnvVar{
				{
					Name:  "PXC_SERVICE",
					Value: c.labels["app.kubernetes.io/instance"] + "-" + "pxc",
				},
				{
					Name: "MONITOR_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
					},
				},
			},
			Resources: res,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "haproxy-custom",
					MountPath: "/etc/haproxy-custom/",
				},
				{
					Name:      "haproxy-auto",
					MountPath: "/etc/haproxy/pxc",
				},
			},
			SecurityContext: spec.ContainerSecurityContext,
		},
	}, nil
}

func (c *HAProxy) PMMContainer(spec *api.PMMSpec, secrets string, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	return nil, nil
}

func (c *HAProxy) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) (*api.Volume, error) {
	vol := app.Volumes(podSpec, haproxyDataVolumeName)
	vol.Volumes = append(
		vol.Volumes,
		app.GetConfigVolumes("haproxy-custom", c.labels["app.kubernetes.io/instance"]+"-haproxy"),
		app.GetTmpVolume("haproxy-auto"),
	)
	return vol, nil
}

func (c *HAProxy) StatefulSet() *appsv1.StatefulSet {
	return c.sfs
}

func (c *HAProxy) Labels() map[string]string {
	return c.labels
}

func (c *HAProxy) Service() string {
	return c.service
}

func (c *HAProxy) UpdateStrategy(cr *api.PerconaXtraDBCluster) appsv1.StatefulSetUpdateStrategy {
	switch cr.Spec.UpdateStrategy {
	case appsv1.OnDeleteStatefulSetStrategyType:
		return appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType}
	default:
		var zero int32 = 0
		return appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
				Partition: &zero,
			},
		}
	}
}
