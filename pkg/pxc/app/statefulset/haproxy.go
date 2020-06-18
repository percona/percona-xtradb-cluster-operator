package statefulset

import (
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	haproxyName = "haproxy"
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
	appc := corev1.Container{
		Name:            haproxyName,
		Image:           spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Args: []string{
			"--configmap=<namespace>/<cr.Name>-haproxy",
		},
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3306,
				Name:          "mysql",
			},
			{
				ContainerPort: 1024,
				Name:          "stat",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "POD_NAME",
				Value: c.sfs.ObjectMeta.Name,
			},
			{
				Name:  "POD_NAMESPACE",
				Value: c.sfs.ObjectMeta.Namespace,
			},
		},
		LivenessProbe: &corev1.Probe{
			FailureThreshold:    3,
			SuccessThreshold:    1,
			TimeoutSeconds:      1,
			PeriodSeconds:       10,
			InitialDelaySeconds: *spec.LivenessInitialDelaySeconds,
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/healthz",
					Port:   intstr.FromInt(1024),
					Scheme: "HTTP",
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

func (c *HAProxy) SidecarContainers(spec *api.PodSpec, secrets string) ([]corev1.Container, error) {
	return nil, nil
}

func (c *HAProxy) PMMContainer(spec *api.PMMSpec, secrets string, cr *api.PerconaXtraDBCluster) (corev1.Container, error) {
	return corev1.Container{}, nil
}

func (c *HAProxy) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) (*api.Volume, error) {
	return nil, nil
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
	case api.SmartUpdateStatefulSetStrategyType:
		return appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType}
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
