package statefulset

import (
	"errors"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	proxyName             = "proxysql"
	proxyDataVolumeName   = "proxydata"
	proxyConfigVolumeName = "config"
)

type Proxy struct {
	sfs     *appsv1.StatefulSet
	labels  map[string]string
	service string
}

func NewProxy(cr *api.PerconaXtraDBCluster) *Proxy {
	sfs := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + proxyName,
			Namespace: cr.Namespace,
		},
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "percona-xtradb-cluster",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/component":  proxyName,
		"app.kubernetes.io/managed-by": "percona-xtradb-cluster-operator",
		"app.kubernetes.io/part-of":    "percona-xtradb-cluster",
	}

	return &Proxy{
		sfs:     sfs,
		labels:  labels,
		service: cr.Name + "-proxysql-unready",
	}
}

func (c *Proxy) Name() string {
	return proxyName
}

func (c *Proxy) AppContainer(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster,
	availableVolumes []corev1.Volume) (corev1.Container, error) {
	appc := corev1.Container{
		Name:            proxyName,
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
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
			},
			{
				Name:      "ssl",
				MountPath: "/etc/proxysql/ssl",
			},
			{
				Name:      "ssl-internal",
				MountPath: "/etc/proxysql/ssl-internal",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "PXC_SERVICE",
				Value: c.labels["app.kubernetes.io/instance"] + "-pxc",
			},
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
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
				},
			},
		},
		SecurityContext: spec.ContainerSecurityContext,
		Resources:       spec.Resources,
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		fvar := true
		appc.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.ProxySQL.EnvVarsSecretName,
					},
					Optional: &fvar,
				},
			},
		}

	}

	if api.ContainsVolume(availableVolumes, proxyConfigVolumeName) {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      proxyConfigVolumeName,
			MountPath: "/etc/proxysql/",
		})
	}

	if cr.CompareVersionWith("1.5.0") >= 0 {
		appc.Env[1] = corev1.EnvVar{
			Name: "OPERATOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "operator"),
			},
		}
	}

	if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.HookScript != "" {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "hookscript",
			MountPath: "/opt/percona/hookscript",
		})
	}
	return appc, nil
}

func (c *Proxy) SidecarContainers(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	pxcMonit := corev1.Container{
		Name:            "pxc-monit",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Args: []string{
			"/usr/bin/peer-list",
			"-on-change=/usr/bin/add_pxc_nodes.sh",
			"-service=$(PXC_SERVICE)",
		},
		Resources: spec.SidecarResources,
		Env: []corev1.EnvVar{
			{
				Name:  "PXC_SERVICE",
				Value: c.labels["app.kubernetes.io/instance"] + "-pxc",
			},
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
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
				},
			},
		},
	}

	proxysqlMonit := corev1.Container{
		Name:            "proxysql-monit",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Args: []string{
			"/usr/bin/peer-list",
			"-on-change=/usr/bin/add_proxysql_nodes.sh",
			"-service=$(PROXYSQL_SERVICE)",
		},
		Resources: spec.SidecarResources,
		Env: []corev1.EnvVar{
			{
				Name:  "PROXYSQL_SERVICE",
				Value: c.labels["app.kubernetes.io/instance"] + "-proxysql-unready",
			},
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
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
				},
			},
		},
	}
	if cr.Spec.AllowUnsafeConfig && (cr.Spec.TLS == nil || cr.Spec.TLS.IssuerConf == nil) {
		pxcMonit.Env = append(pxcMonit.Env, corev1.EnvVar{
			Name:  "SSL_DIR",
			Value: "/dev/null",
		})
		proxysqlMonit.Env = append(proxysqlMonit.Env, corev1.EnvVar{
			Name:  "SSL_DIR",
			Value: "/dev/null",
		})
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		fvar := true
		envFrom := corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cr.Spec.ProxySQL.EnvVarsSecretName,
				},
				Optional: &fvar,
			},
		}
		pxcMonit.EnvFrom = append(pxcMonit.EnvFrom, envFrom)
		proxysqlMonit.EnvFrom = append(proxysqlMonit.EnvFrom, envFrom)
	}
	if cr.CompareVersionWith("1.5.0") >= 0 {
		operEnv := corev1.EnvVar{
			Name: "OPERATOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "operator"),
			},
		}
		pxcMonit.Env[1] = operEnv
		proxysqlMonit.Env[1] = operEnv
	}

	return []corev1.Container{pxcMonit, proxysqlMonit}, nil
}

func (c *Proxy) LogCollectorContainer(_ *api.LogCollectorSpec, _ string, _ string, _ *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	return nil, nil
}

func (c *Proxy) PMMContainer(spec *api.PMMSpec, secret *corev1.Secret, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	ct := app.PMMClient(spec, secret, cr.CompareVersionWith("1.2.0") >= 0, cr.CompareVersionWith("1.7.0") >= 0)

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
				SecretKeyRef: app.SecretKeySelector(secret.Name, "monitor"),
			},
		},
	}

	dbEnvs := []corev1.EnvVar{
		{
			Name:  "DB_USER",
			Value: "monitor",
		},
		{
			Name: "DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secret.Name, "monitor"),
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
			Value: "6032",
		},
	}

	dbArgsEnv := []corev1.EnvVar{
		{
			Name:  "DB_ARGS",
			Value: "--dsn $(MONITOR_USER):$(MONITOR_PASSWORD)@tcp(localhost:6032)/",
		},
	}

	ct.Env = append(ct.Env, pmmEnvs...)
	if cr.CompareVersionWith("1.2.0") >= 0 {
		ct.Env = append(ct.Env, dbEnvs...)
		ct.Resources = spec.Resources
	} else {
		ct.Env = append(ct.Env, dbArgsEnv...)
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		fvar := true
		ct.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.ProxySQL.EnvVarsSecretName,
					},
					Optional: &fvar,
				},
			},
		}
	}

	if cr.CompareVersionWith("1.7.0") >= 0 {
		PmmProxysqlParams := ""
		if spec.ProxysqlParams != "" {
			PmmProxysqlParams = spec.ProxysqlParams
		}
		clusterPmmEnvs := []corev1.EnvVar{
			{
				Name:  "CLUSTER_NAME",
				Value: cr.Name,
			},
			{
				Name:  "PMM_ADMIN_CUSTOM_PARAMS",
				Value: PmmProxysqlParams,
			},
		}
		ct.Env = append(ct.Env, clusterPmmEnvs...)
		pmmAgentScriptEnv := app.PMMAgentScript("proxysql")
		ct.Env = append(ct.Env, pmmAgentScriptEnv...)
	}

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

	return &ct, nil
}

func (c *Proxy) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, vg api.CustomVolumeGetter) (*api.Volume, error) {
	ls := c.Labels()

	vol := app.Volumes(podSpec, proxyDataVolumeName)
	vol.Volumes = append(
		vol.Volumes,
		app.GetSecretVolumes("ssl-internal", podSpec.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", podSpec.SSLSecretName, cr.Spec.AllowUnsafeConfig),
	)

	configVolume, err := vg(cr.Namespace, proxyConfigVolumeName, ls["app.kubernetes.io/instance"]+"-proxysql", false)
	if err != nil && !errors.Is(err, api.NoCustomVolumeErr) {
		return nil, err
	}
	if err == nil {
		vol.Volumes = append(vol.Volumes, configVolume)
	}
	if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.HookScript != "" {
		vol.Volumes = append(vol.Volumes,
			app.GetConfigVolumes("hookscript", ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"]+"-hookscript"))
	}
	return vol, nil
}

func (c *Proxy) StatefulSet() *appsv1.StatefulSet {
	return c.sfs
}

func (c *Proxy) Labels() map[string]string {
	return c.labels
}

func (c *Proxy) Service() string {
	return c.service
}

func (c *Proxy) UpdateStrategy(cr *api.PerconaXtraDBCluster) appsv1.StatefulSetUpdateStrategy {
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
