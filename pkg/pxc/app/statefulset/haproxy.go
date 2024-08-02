package statefulset

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
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

func (c *HAProxy) Name() string {
	return haproxyName
}

func (c *HAProxy) InitContainers(cr *api.PerconaXtraDBCluster, initImageName string) []corev1.Container {
	inits := proxyInitContainers(cr, initImageName)

	if cr.CompareVersionWith("1.15.0") >= 0 {
		inits = append(inits, HaproxyEntrypointInitContainer(cr, initImageName))
	}

	return inits
}

func (c *HAProxy) AppContainer(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster,
	_ []corev1.Volume,
) (corev1.Container, error) {
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
		Resources:       spec.Resources,
	}

	if cr.CompareVersionWith("1.15.0") >= 0 {
		appc.Command = []string{"/opt/percona/haproxy-entrypoint.sh"}
		appc.Args = []string{"haproxy"}
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      app.BinVolumeName,
			MountPath: app.BinVolumeMountPath,
		})

	}

	if cr.CompareVersionWith("1.7.0") < 0 {
		appc.Env = append(appc.Env, corev1.EnvVar{
			Name: "MONITOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, users.Monitor),
			},
		})
	}

	appc.Ports = append(
		appc.Ports,
		corev1.ContainerPort{
			ContainerPort: 33062,
			Name:          "mysql-admin",
		},
	)

	appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
		Name:      "mysql-users-secret-file",
		MountPath: "/etc/mysql/mysql-users-secret",
	})

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

	appc.Ports = append(
		appc.Ports,
		corev1.ContainerPort{
			ContainerPort: 33060,
			Name:          "mysqlx",
		},
	)

	rsCmd := "/opt/percona/haproxy_readiness_check.sh"
	lsCmd := "/opt/percona/haproxy_liveness_check.sh"
	if cr.CompareVersionWith("1.15.0") < 0 {
		rsCmd = "/usr/local/bin/readiness-check.sh"
		lsCmd = "/usr/local/bin/liveness-check.sh"
	}
	appc.ReadinessProbe = &cr.Spec.HAProxy.ReadinessProbes
	appc.ReadinessProbe.Exec = &corev1.ExecAction{
		Command: []string{rsCmd},
	}
	appc.LivenessProbe = &cr.Spec.HAProxy.LivenessProbes
	appc.LivenessProbe.Exec = &corev1.ExecAction{
		Command: []string{lsCmd},
	}

	probsEnvs := []corev1.EnvVar{
		{
			Name:  "LIVENESS_CHECK_TIMEOUT",
			Value: fmt.Sprint(cr.Spec.HAProxy.LivenessProbes.TimeoutSeconds),
		},
		{
			Name:  "READINESS_CHECK_TIMEOUT",
			Value: fmt.Sprint(cr.Spec.HAProxy.ReadinessProbes.TimeoutSeconds),
		},
	}
	appc.Env = append(appc.Env, probsEnvs...)

	if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.HAProxy != nil && cr.Spec.HAProxy.HookScript != "" {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "hookscript",
			MountPath: "/opt/percona/hookscript",
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

	if cr.Spec.HAProxy != nil && (cr.Spec.HAProxy.Lifecycle.PostStart != nil || cr.Spec.HAProxy.Lifecycle.PreStop != nil) {
		appc.Lifecycle = &cr.Spec.HAProxy.Lifecycle
	}

	return appc, nil
}

func (c *HAProxy) SidecarContainers(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	container := corev1.Container{
		Name:            "pxc-monit",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Args: []string{
			"/opt/percona/peer-list",
			"-on-change=/opt/percona/haproxy_add_pxc_nodes.sh",
			"-service=$(PXC_SERVICE)",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "PXC_SERVICE",
				Value: c.labels["app.kubernetes.io/instance"] + "-" + "pxc",
			},
		},
		Resources: spec.SidecarResources,
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

	if cr.CompareVersionWith("1.15.0") >= 0 {
		container.Command = []string{"/opt/percona/haproxy-entrypoint.sh"}
	}

	if cr.CompareVersionWith("1.15.0") < 0 {
		container.Args = []string{
			"/usr/bin/peer-list",
			"-on-change=/usr/bin/add_pxc_nodes.sh",
			"-service=$(PXC_SERVICE)",
		}
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
				SecretKeyRef: app.SecretKeySelector(secrets, users.Monitor),
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

	if cr.CompareVersionWith("1.15.0") >= 0 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "REPLICAS_SVC_ONLY_READERS",
			Value: strconv.FormatBool(cr.Spec.HAProxy.ExposeReplicas.OnlyReaders),
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      app.BinVolumeName,
			MountPath: app.BinVolumeMountPath,
		})
	}

	return []corev1.Container{container}, nil
}

func (c *HAProxy) LogCollectorContainer(_ *api.LogCollectorSpec, _ string, _ string, _ *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	return nil, nil
}

func (c *HAProxy) PMMContainer(ctx context.Context, cl client.Client, spec *api.PMMSpec, secret *corev1.Secret, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	if cr.CompareVersionWith("1.9.0") < 0 {
		return nil, nil
	}

	envVarsSecret := &corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{Name: cr.Spec.HAProxy.EnvVarsSecretName, Namespace: cr.Namespace}, envVarsSecret)
	if client.IgnoreNotFound(err) != nil {
		return nil, errors.Wrap(err, "get env vars secret")
	}

	ct := app.PMMClient(cr, spec, secret, envVarsSecret)

	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "DB_TYPE",
			Value: "haproxy",
		},
		{
			Name:  "MONITOR_USER",
			Value: users.Monitor,
		},
		{
			Name: "MONITOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secret.Name, users.Monitor),
			},
		},
		{
			Name:  "DB_USER",
			Value: users.Monitor,
		},
		{
			Name: "DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secret.Name, users.Monitor),
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

	pmmAgentScriptEnv := app.PMMAgentScript(cr, "haproxy")
	ct.Env = append(ct.Env, pmmAgentScriptEnv...)

	if cr.CompareVersionWith("1.10.0") >= 0 {
		// PMM team added these flags which allows us to avoid
		// container crash, but just restart pmm-agent till it recovers
		// the connection.
		sidecarEnvs := []corev1.EnvVar{
			{
				Name:  "PMM_AGENT_SIDECAR",
				Value: "true",
			},
			{
				Name:  "PMM_AGENT_SIDECAR_SLEEP",
				Value: "5",
			},
		}
		ct.Env = append(ct.Env, sidecarEnvs...)
	}

	if cr.CompareVersionWith("1.14.0") >= 0 {
		// PMM team moved temp directory to /usr/local/percona/pmm2/tmp
		// but it doesn't work on OpenShift so we set it back to /tmp
		sidecarEnvs := []corev1.EnvVar{
			{
				Name:  "PMM_AGENT_PATHS_TEMPDIR",
				Value: "/tmp",
			},
		}
		ct.Env = append(ct.Env, sidecarEnvs...)

		fvar := true
		ct.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.HAProxy.EnvVarsSecretName,
					},
					Optional: &fvar,
				},
			},
		}
	}

	ct.Resources = spec.Resources

	return &ct, nil
}

func (c *HAProxy) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, vg api.CustomVolumeGetter) (*api.Volume, error) {
	vol := app.Volumes(podSpec, haproxyDataVolumeName)
	configVolume, err := vg(cr.Namespace, "haproxy-custom", c.labels["app.kubernetes.io/instance"]+"-haproxy", true)
	if err != nil {
		return nil, err
	}
	vol.Volumes = append(
		vol.Volumes,
		configVolume,
		app.GetTmpVolume("haproxy-auto"),
	)
	if cr.CompareVersionWith("1.7.0") >= 0 {
		vol.Volumes = append(vol.Volumes, app.GetSecretVolumes("mysql-users-secret-file", "internal-"+cr.Name, false))
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		vol.Volumes = append(vol.Volumes, app.GetSecretVolumes(cr.Spec.HAProxy.EnvVarsSecretName, cr.Spec.HAProxy.EnvVarsSecretName, true))
	}
	if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.HAProxy != nil && cr.Spec.HAProxy.HookScript != "" {
		vol.Volumes = append(vol.Volumes,
			app.GetConfigVolumes("hookscript", c.labels["app.kubernetes.io/instance"]+"-"+c.labels["app.kubernetes.io/component"]+"-hookscript"))
	}
	if cr.CompareVersionWith("1.13.0") >= 0 {
		vol.Volumes = append(vol.Volumes,
			corev1.Volume{
				Name: app.BinVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)
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
