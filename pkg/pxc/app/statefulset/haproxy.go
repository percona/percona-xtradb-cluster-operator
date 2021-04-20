package statefulset

import (
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/pkg/errors"
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
	appc := corev1.Container{
		Name:            haproxyName,
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
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
		},
		SecurityContext: spec.ContainerSecurityContext,
	}

	if cr.CompareVersionWith("1.7.0") < 0 {
		appc.Env = append(appc.Env, corev1.EnvVar{
			Name: "MONITOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
			},
		})
	}

	if cr.CompareVersionWith("1.6.0") >= 0 {
		redinessDelay := int32(15)
		if spec.ReadinessInitialDelaySeconds != nil {
			redinessDelay = *spec.ReadinessInitialDelaySeconds
		}
		appc.ReadinessProbe = app.Probe(&corev1.Probe{
			InitialDelaySeconds: redinessDelay,
			TimeoutSeconds:      1,
			PeriodSeconds:       5,
			FailureThreshold:    3,
		}, "/usr/local/bin/readiness-check.sh")

		appc.Ports = append(
			appc.Ports,
			corev1.ContainerPort{
				ContainerPort: 33062,
				Name:          "mysql-admin",
			},
		)
	}

	if cr.CompareVersionWith("1.7.0") >= 0 {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "mysql-users-secret-file",
			MountPath: "/etc/mysql/mysql-users-secret",
		})

		livenessDelay := int32(60)
		if spec.LivenessInitialDelaySeconds != nil {
			livenessDelay = *spec.LivenessInitialDelaySeconds
		}
		appc.LivenessProbe = app.Probe(&corev1.Probe{
			InitialDelaySeconds: livenessDelay,
			TimeoutSeconds:      5,
			PeriodSeconds:       30,
			FailureThreshold:    4,
		}, "/usr/local/bin/readiness-check.sh")
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		fvar := true
		appc.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.HAProxy.EnvVarsSecretName,
					},
					Optional: &fvar,
				},
			},
		}
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      cr.Spec.HAProxy.EnvVarsSecretName,
			MountPath: "/etc/mysql/haproxy-env-secret",
		})
	}
	hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
	if err != nil {
		return appc, errors.Wrap(err, "check if congfig has proxy_protocol_networks key")
	}
	if hasKey {
		appc.Env = append(appc.Env, corev1.EnvVar{
			Name:  "IS_PROXY_PROTOCOL",
			Value: "yes",
		})
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

	container := corev1.Container{
		Name:            "pxc-monit",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
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
	}

	hasKey, err := cr.ConfigHasKey("mysqld", "proxy_protocol_networks")
	if err != nil {
		return nil, errors.Wrap(err, "check if congfig has proxy_protocol_networks key")
	}
	if hasKey {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "IS_PROXY_PROTOCOL",
			Value: "yes",
		})
	}
	if cr.CompareVersionWith("1.7.0") < 0 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "MONITOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
			},
		})
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "mysql-users-secret-file",
			MountPath: "/etc/mysql/mysql-users-secret",
		})
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		fvar := true
		container.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.HAProxy.EnvVarsSecretName,
					},
					Optional: &fvar,
				},
			},
		}
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      cr.Spec.HAProxy.EnvVarsSecretName,
			MountPath: "/etc/mysql/haproxy-env-secret",
		})
	}
	return []corev1.Container{container}, nil
}

func (c *HAProxy) LogCollectorContainer(spec *api.LogCollectorSpec, logPsecrets string, logRsecrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	return nil, nil
}

func (c *HAProxy) PMMContainer(spec *api.PMMSpec, secrets string, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	if cr.CompareVersionWith("1.9.0") < 0 {
		return nil, nil
	}

	ct := app.PMMClient(spec, secrets, cr.CompareVersionWith("1.2.0") >= 0, cr.CompareVersionWith("1.7.0") >= 0)

	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "DB_TYPE",
			Value: "haproxy",
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
		{
			Name:  "CLUSTER_NAME",
			Value: cr.Name,
		},
		{
			Name:  "PMM_ADMIN_CUSTOM_PARAMS",
			Value: "--listen-port=8404",
		},
	}
	ct.Env = append(ct.Env, pmmEnvs...)

	pmmAgentScriptEnv := app.PMMAgentScript("haproxy")
	ct.Env = append(ct.Env, pmmAgentScriptEnv...)

	res, err := app.CreateResources(spec.Resources)
	if err != nil {
		return nil, fmt.Errorf("create resources error: %v", err)
	}
	ct.Resources = res

	return &ct, nil
}

func (c *HAProxy) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) (*api.Volume, error) {
	vol := app.Volumes(podSpec, haproxyDataVolumeName)
	vol.Volumes = append(
		vol.Volumes,
		app.GetConfigVolumes("haproxy-custom", c.labels["app.kubernetes.io/instance"]+"-haproxy"),
		app.GetTmpVolume("haproxy-auto"),
	)
	if cr.CompareVersionWith("1.7.0") >= 0 {
		vol.Volumes = append(vol.Volumes, app.GetSecretVolumes("mysql-users-secret-file", "internal-"+cr.Name, false))
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		vol.Volumes = append(vol.Volumes, app.GetSecretVolumes(cr.Spec.HAProxy.EnvVarsSecretName, cr.Spec.HAProxy.EnvVarsSecretName, true))
	}

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
