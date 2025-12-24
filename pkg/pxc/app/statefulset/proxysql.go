package statefulset

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const (
	proxyDataVolumeName   = "proxydata"
	proxyConfigVolumeName = "config"
	SchedulerConfigPath   = "/tmp/scheduler-config.toml"
)

type Proxy struct {
	cr *api.PerconaXtraDBCluster
}

func NewProxy(cr *api.PerconaXtraDBCluster) api.StatefulApp {
	return &Proxy{
		cr: cr.DeepCopy(),
	}
}

func (c *Proxy) Name() string {
	return naming.ComponentProxySQL
}

func (c *Proxy) InitContainers(cr *api.PerconaXtraDBCluster, initImageName string) []corev1.Container {
	inits := proxyInitContainers(cr, initImageName)

	if cr.CompareVersionWith("1.15.0") >= 0 {
		inits = append(inits, ProxySQLEntrypointInitContainer(cr, initImageName))
	}

	return inits
}

func proxyInitContainers(cr *api.PerconaXtraDBCluster, initImageName string) []corev1.Container {
	inits := []corev1.Container{}
	if cr.CompareVersionWith("1.13.0") >= 0 {
		inits = []corev1.Container{
			EntrypointInitContainer(cr, initImageName, app.BinVolumeName),
		}
	}

	return inits
}

func (c *Proxy) AppContainer(ctx context.Context, _ client.Client, spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster,
	availableVolumes []corev1.Volume,
) (corev1.Container, error) {
	appc := corev1.Container{
		Name:            naming.ComponentProxySQL,
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
				Value: c.Labels()[naming.LabelAppKubernetesInstance] + "-pxc",
			},
			{
				Name: "MYSQL_ROOT_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Root),
				},
			},
			{
				Name:  "PROXY_ADMIN_USER",
				Value: users.ProxyAdmin,
			},
			{
				Name: "PROXY_ADMIN_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.ProxyAdmin),
				},
			},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Monitor),
				},
			},
		},
		SecurityContext: spec.ContainerSecurityContext,
		Resources:       spec.Resources,
	}

	if cr.CompareVersionWith("1.17.0") >= 0 {
		appc.Ports = append(
			appc.Ports,
			corev1.ContainerPort{
				ContainerPort: 6070,
				Name:          "stats",
			},
		)
	}

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

	proxyConfigMountPath := "/etc/proxysql"
	if cr.CompareVersionWith("1.19.0") >= 0 {
		proxyConfigMountPath = "/etc/proxysql/custom"
	}
	if api.ContainsVolume(availableVolumes, proxyConfigVolumeName) {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      proxyConfigVolumeName,
			MountPath: proxyConfigMountPath,
		})
	}

	if cr.CompareVersionWith("1.15.0") >= 0 {
		appc.Command = []string{"/opt/percona/proxysql-entrypoint.sh"}
		appc.Args = []string{"proxysql", "-f", "-c", "/etc/proxysql/proxysql.cnf", "--reload"}
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      app.BinVolumeName,
			MountPath: app.BinVolumeMountPath,
		})
	}

	appc.Env[1] = corev1.EnvVar{
		Name: "OPERATOR_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: app.SecretKeySelector(secrets, users.Operator),
		},
	}

	if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.HookScript != "" {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "hookscript",
			MountPath: "/opt/percona/hookscript",
		})
	}

	if cr.Spec.ProxySQL != nil && (cr.Spec.ProxySQL.Lifecycle.PostStart != nil || cr.Spec.ProxySQL.Lifecycle.PreStop != nil) {
		appc.Lifecycle = &cr.Spec.ProxySQL.Lifecycle
	}

	if cr.CompareVersionWith("1.19.0") >= 0 {
		scheduler := cr.Spec.ProxySQL.Scheduler
		if scheduler.Enabled {
			appc.Env = append(appc.Env, schedulerEnvVariables(scheduler)...)
			appc.Env = append(appc.Env, corev1.EnvVar{
				Name:  "SCHEDULER_ENABLED",
				Value: "true",
			})
		}

		extraMounts := api.ExtraPVCVolumeMounts(ctx, spec.ExtraPVCs)
		appc.VolumeMounts = append(appc.VolumeMounts, extraMounts...)
	}

	return appc, nil
}

func (c *Proxy) XtrabackupContainer(ctx context.Context, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	return nil, nil
}

func (c *Proxy) SidecarContainers(ctx context.Context, cl client.Client, spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	pxcMonit := corev1.Container{
		Name:            "pxc-monit",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Args: []string{
			"/opt/percona/peer-list",
			"-on-change=/opt/percona/proxysql_add_pxc_nodes.sh",
			"-service=$(PXC_SERVICE)",
		},
		Resources: spec.SidecarResources,
		Env: []corev1.EnvVar{
			{
				Name:  "PXC_SERVICE",
				Value: c.Labels()[naming.LabelAppKubernetesInstance] + "-pxc",
			},
			{
				Name: "MYSQL_ROOT_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Root),
				},
			},
			{
				Name:  "PROXY_ADMIN_USER",
				Value: users.ProxyAdmin,
			},
			{
				Name: "PROXY_ADMIN_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.ProxyAdmin),
				},
			},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Monitor),
				},
			},
		},
	}

	if cr.CompareVersionWith("1.15.0") >= 0 {
		pxcMonit.VolumeMounts = append(pxcMonit.VolumeMounts, corev1.VolumeMount{
			Name:      app.BinVolumeName,
			MountPath: app.BinVolumeMountPath,
		})
	}

	if cr.CompareVersionWith("1.15.0") < 0 {
		pxcMonit.Args = []string{
			"/usr/bin/peer-list",
			"-on-change=/usr/bin/add_pxc_nodes.sh",
			"-service=$(PXC_SERVICE)",
		}
	}

	if cr.CompareVersionWith("1.18.0") >= 0 {
		// PEER_LIST_SRV_PROTOCOL is configured through the secret: EnvVarsSecretName
		pxcMonit.Args = append(pxcMonit.Args, "-protocol=$(PEER_LIST_SRV_PROTOCOL)")
	}

	proxysqlMonit := corev1.Container{
		Name:            "proxysql-monit",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Args: []string{
			"/opt/percona/peer-list",
			"-on-change=/opt/percona/proxysql_add_proxysql_nodes.sh",
			"-service=$(PROXYSQL_SERVICE)",
		},
		Resources: spec.SidecarResources,
		Env: []corev1.EnvVar{
			{
				Name:  "PROXYSQL_SERVICE",
				Value: c.Labels()[naming.LabelAppKubernetesInstance] + "-proxysql-unready",
			},
			{
				Name: "MYSQL_ROOT_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Root),
				},
			},
			{
				Name:  "PROXY_ADMIN_USER",
				Value: users.ProxyAdmin,
			},
			{
				Name: "PROXY_ADMIN_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.ProxyAdmin),
				},
			},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Monitor),
				},
			},
		},
	}

	if cr.CompareVersionWith("1.15.0") >= 0 {
		proxysqlMonit.VolumeMounts = append(proxysqlMonit.VolumeMounts, corev1.VolumeMount{
			Name:      app.BinVolumeName,
			MountPath: app.BinVolumeMountPath,
		})
	}

	if cr.CompareVersionWith("1.15.0") < 0 {
		proxysqlMonit.Args = []string{
			"/usr/bin/peer-list",
			"-on-change=/usr/bin/add_proxysql_nodes.sh",
			"-service=$(PROXYSQL_SERVICE)",
		}
	}

	if !cr.TLSEnabled() {
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
				SecretKeyRef: app.SecretKeySelector(secrets, users.Operator),
			},
		}
		pxcMonit.Env[1] = operEnv
		proxysqlMonit.Env[1] = operEnv
	}

	if cr.CompareVersionWith("1.18.0") >= 0 {
		// PEER_LIST_SRV_PROTOCOL is configured through the secret: EnvVarsSecretName
		proxysqlMonit.Args = append(proxysqlMonit.Args, "-protocol=$(PEER_LIST_SRV_PROTOCOL)")
	}

	if cr.CompareVersionWith("1.19.0") >= 0 {
		pxcMonit.VolumeMounts = append(pxcMonit.VolumeMounts, []corev1.VolumeMount{
			{
				Name:      "ssl",
				MountPath: "/etc/proxysql/ssl",
			},
			{
				Name:      "ssl-internal",
				MountPath: "/etc/proxysql/ssl-internal",
			},
		}...)

		if cr.Spec.ProxySQL.Scheduler.Enabled {
			pxcMonit.Env = append(pxcMonit.Env, schedulerEnvVariables(cr.Spec.ProxySQL.Scheduler)...)
			pxcMonit.Env = append(pxcMonit.Env, corev1.EnvVar{
				Name:  "SCHEDULER_ENABLED",
				Value: "true",
			})
		}

		pxcMonit.Command = []string{"/opt/percona/proxysql-entrypoint.sh"}
		proxysqlMonit.Command = []string{"/opt/percona/proxysql-entrypoint.sh"}
	}

	containers := []corev1.Container{pxcMonit}
	// we are disabling ProxySQL cluster mode in case scheduler is enabled
	// therefore we don't need proxysqlMonit container
	if !cr.Spec.ProxySQL.Scheduler.Enabled {
		containers = append(containers, proxysqlMonit)
	}

	return containers, nil
}

func schedulerEnvVariables(scheduler api.ProxySQLSchedulerSpec) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "SCHEDULER_CHECKTIMEOUT",
			Value: strconv.FormatInt(int64(scheduler.CheckTimeoutMilliseconds), 10),
		},
		{
			Name: "SCHEDULER_WRITERALSOREADER",
			Value: func() string {
				if scheduler.WriterIsAlsoReader {
					return "1"
				}
				return "0"
			}(),
		},
		{
			Name:  "SCHEDULER_RETRYUP",
			Value: strconv.FormatInt(int64(scheduler.SuccessThreshold), 10),
		},
		{
			Name:  "SCHEDULER_RETRYDOWN",
			Value: strconv.FormatInt(int64(scheduler.FailureThreshold), 10),
		},
		{
			Name:  "SCHEDULER_PINGTIMEOUT",
			Value: strconv.FormatInt(int64(scheduler.PingTimeoutMilliseconds), 10),
		},
		{
			Name:  "SCHEDULER_NODECHECKINTERVAL",
			Value: strconv.FormatInt(int64(scheduler.NodeCheckIntervalMilliseconds), 10),
		},
		{
			Name:  "SCHEDULER_MAXCONNECTIONS",
			Value: strconv.FormatInt(int64(scheduler.MaxConnections), 10),
		},
		{
			Name:  "PERCONA_SCHEDULER_CFG",
			Value: SchedulerConfigPath,
		},
	}

}

func (c *Proxy) LogCollectorContainer(_ *api.LogCollectorSpec, _ string, _ string, _ *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	return nil, nil
}

func (c *Proxy) PMMContainer(ctx context.Context, cl client.Client, spec *api.PMMSpec, secret *corev1.Secret, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	if cr.Spec.PMM == nil || !cr.Spec.PMM.Enabled {
		return nil, nil
	}

	envVarsSecret := &corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{Name: cr.Spec.PXC.EnvVarsSecretName, Namespace: cr.Namespace}, envVarsSecret)
	if client.IgnoreNotFound(err) != nil {
		return nil, errors.Wrap(err, "get env vars secret")
	}

	if v, exists := secret.Data[users.PMMServerToken]; exists && len(v) != 0 {
		pmm3Container, err := app.PMM3Client(cr, secret, envVarsSecret)
		if err != nil {
			return nil, errors.Wrap(err, "get pmm3 container")
		}

		pmm3Container.Env = append(pmm3Container.Env, pmm3ProxySQLEnvVars(spec.ProxysqlParams)...)

		return &pmm3Container, nil
	}

	clusterName := cr.Name
	if cr.CompareVersionWith("1.18.0") >= 0 && cr.Spec.PMM.CustomClusterName != "" {
		clusterName = cr.Spec.PMM.CustomClusterName
	}

	// Checking the secret to determine if the PMM2 container can be constructed.
	if !cr.Spec.PMM.HasSecret(secret) {
		return nil, errors.New("can't enable PMM2: either pmmserverkey key doesn't exist in the secrets, or secrets and internal secrets are out of sync")
	}

	ct := app.PMMClient(cr, spec, secret, envVarsSecret)

	pmmEnvs := []corev1.EnvVar{
		{
			Name:  "DB_TYPE",
			Value: "proxysql",
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
	}

	dbEnvs := []corev1.EnvVar{
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
				Value: clusterName,
			},
			{
				Name:  "PMM_ADMIN_CUSTOM_PARAMS",
				Value: PmmProxysqlParams,
			},
		}
		ct.Env = append(ct.Env, clusterPmmEnvs...)
		pmmAgentScriptEnv := app.PMMAgentScript(cr, "proxysql")
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
	}

	return &ct, nil
}

// pmm3ProxySQLEnvVars returns a list of environment variables to configure the PMM3 container for monitoring proxysql.
func pmm3ProxySQLEnvVars(pmmProxysqlParams string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DB_TYPE",
			Value: "proxysql",
		},
		{
			Name:  "PMM_ADMIN_CUSTOM_PARAMS",
			Value: pmmProxysqlParams,
		},
		{
			Name:  "DB_ARGS",
			Value: "--dsn $(MONITOR_USER):$(MONITOR_PASSWORD)@tcp(localhost:6032)/",
		},
		{
			Name:  "DB_PORT",
			Value: "6032",
		},
	}
}

func (c *Proxy) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, vg api.CustomVolumeGetter) (*api.Volume, error) {
	ls := c.Labels()

	sslVolume := app.GetSecretVolumes("ssl", podSpec.SSLSecretName, !cr.TLSEnabled())
	if cr.CompareVersionWith("1.15.0") < 0 {
		sslVolume = app.GetSecretVolumes("ssl", podSpec.SSLSecretName, cr.Spec.AllowUnsafeConfig)
	}

	vol := app.Volumes(podSpec, proxyDataVolumeName)
	vol.Volumes = append(
		vol.Volumes,
		app.GetSecretVolumes("ssl-internal", podSpec.SSLInternalSecretName, true),
		sslVolume,
	)

	configVolume, err := vg(cr.Namespace, proxyConfigVolumeName, ls[naming.LabelAppKubernetesInstance]+"-proxysql", false)
	if err != nil && !errors.Is(err, api.NoCustomVolumeErr) {
		return nil, err
	}
	if err == nil {
		vol.Volumes = append(vol.Volumes, configVolume)
	}
	if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.HookScript != "" {
		vol.Volumes = append(vol.Volumes,
			app.GetConfigVolumes("hookscript", ls[naming.LabelAppKubernetesInstance]+"-"+ls[naming.LabelAppKubernetesComponent]+"-hookscript"))
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

	if cr.CompareVersionWith("1.16.0") >= 0 {
		for i := range vol.PVCs {
			vol.PVCs[i].Labels = ls
		}
	}

	return vol, nil
}

// StatefulSet returns a new statefulset object with empty spec.
func (c *Proxy) StatefulSet() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.cr.Name + "-" + naming.ComponentProxySQL,
			Namespace: c.cr.Namespace,
		},
	}
}

func (c *Proxy) Labels() map[string]string {
	return naming.LabelsProxySQL(c.cr)
}

func (c *Proxy) Service() string {
	return c.cr.Name + "-proxysql-unready"
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
