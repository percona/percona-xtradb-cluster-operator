package statefulset

import (
	"context"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/config"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

const (
	VaultSecretVolumeName = "vault-keyring-secret"
)

type Node struct {
	cr *api.PerconaXtraDBCluster
}

func NewNode(cr *api.PerconaXtraDBCluster) api.StatefulApp {
	return &Node{
		cr: cr.DeepCopy(),
	}
}

func (c *Node) Name() string {
	return app.Name
}

func (c *Node) InitContainers(cr *api.PerconaXtraDBCluster, initImageName string) []corev1.Container {
	inits := []corev1.Container{
		EntrypointInitContainer(cr, initImageName, app.DataVolumeName),
	}
	return inits
}

func (c *Node) AppContainer(ctx context.Context, cl client.Client, spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster, _ []corev1.Volume) (corev1.Container, error) {
	redinessDelay := int32(15)
	if spec.ReadinessInitialDelaySeconds != nil {
		redinessDelay = *spec.ReadinessInitialDelaySeconds
	}
	livenessDelay := int32(300)
	if spec.LivenessInitialDelaySeconds != nil {
		livenessDelay = *spec.LivenessInitialDelaySeconds
	}
	tvar := true

	serverIDHash := fnv.New32()
	serverIDHash.Write([]byte(string(cr.UID)))

	// we cut first 3 symbols to give a space for hostname(actially, pod number)
	// which is appended to all server ids. If we do not do this, it
	// can cause a int32 overflow
	// P.S max value is 4294967295
	serverIDHashStr := fmt.Sprint(serverIDHash.Sum32())
	if len(serverIDHashStr) > 7 {
		serverIDHashStr = serverIDHashStr[:7]
	}

	appc := corev1.Container{
		Name:            app.Name,
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		ReadinessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: redinessDelay,
			TimeoutSeconds:      15,
			PeriodSeconds:       30,
			FailureThreshold:    5,
		}, "/var/lib/mysql/readiness-check.sh"),
		LivenessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: livenessDelay,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
		}, "/var/lib/mysql/liveness-check.sh"),
		Args:    []string{"mysqld"},
		Command: []string{"/var/lib/mysql/pxc-entrypoint.sh"},
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
			{
				ContainerPort: 33062,
				Name:          "mysql-admin",
			},
			{
				ContainerPort: 33060,
				Name:          "mysqlx",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
			{
				Name:      "config",
				MountPath: "/etc/percona-xtradb-cluster.conf.d",
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
			{
				Name:      "mysql-users-secret-file",
				MountPath: "/etc/mysql/mysql-users-secret",
			},
			{
				Name:      "auto-config",
				MountPath: "/etc/my.cnf.d",
			},
			{
				Name:      VaultSecretVolumeName,
				MountPath: "/etc/mysql/vault-keyring-secret",
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "PXC_SERVICE",
				Value: c.Labels()[naming.LabelAppKubernetesInstance] + "-" + c.Labels()[naming.LabelAppKubernetesComponent] + "-unready",
			},
			{
				Name:  "MONITOR_HOST",
				Value: "%",
			},
			{
				Name: "MYSQL_ROOT_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Root),
				},
			},
			{
				Name: "XTRABACKUP_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Xtrabackup),
				},
			},
			{
				Name: "MONITOR_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: app.SecretKeySelector(secrets, users.Monitor),
				},
			},
		},
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.PXC.EnvVarsSecretName,
					},
					Optional: &tvar,
				},
			},
		},
		SecurityContext: spec.ContainerSecurityContext,
		Resources:       spec.Resources,
	}

	if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.PXC != nil && cr.Spec.PXC.HookScript != "" {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "hookscript",
			MountPath: "/opt/percona/hookscript",
		})
	}

	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Enabled {
		appc.Env = append(appc.Env, []corev1.EnvVar{
			{
				Name:  "LOG_DATA_DIR",
				Value: "/var/lib/mysql",
			},
			{
				Name:  "IS_LOGCOLLECTOR",
				Value: "yes",
			},
		}...)
	}

	appc.Env = append(appc.Env, []corev1.EnvVar{
		{
			Name:  "CLUSTER_HASH",
			Value: serverIDHashStr,
		},
		{
			Name: "OPERATOR_ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, users.Operator),
			},
		},
		{
			Name:  "LIVENESS_CHECK_TIMEOUT",
			Value: fmt.Sprint(spec.LivenessProbes.TimeoutSeconds),
		},
		{
			Name:  "READINESS_CHECK_TIMEOUT",
			Value: fmt.Sprint(spec.ReadinessProbes.TimeoutSeconds),
		},
	}...)

	plugin := "caching_sha2_password"
	if cr.CompareVersionWith("1.19.0") < 0 {
		if cr.Spec.ProxySQLEnabled() {
			plugin = "mysql_native_password"
		}

	}
	appc.Env = append(appc.Env, corev1.EnvVar{
		Name:  "DEFAULT_AUTHENTICATION_PLUGIN",
		Value: plugin,
	})

	if cr.CompareVersionWith("1.14.0") >= 0 {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "mysql-init-file",
			MountPath: "/etc/mysql/init-file",
		})

		appc.ReadinessProbe = app.Probe(&cr.Spec.PXC.ReadinessProbes, "/var/lib/mysql/readiness-check.sh")
		appc.LivenessProbe = app.Probe(&cr.Spec.PXC.LivenessProbes, "/var/lib/mysql/liveness-check.sh")
	}

	if cr.Spec.PXC != nil && (cr.Spec.PXC.Lifecycle.PostStart != nil || cr.Spec.PXC.Lifecycle.PreStop != nil) {
		appc.Lifecycle = &cr.Spec.PXC.Lifecycle
	}

	if cr.CompareVersionWith("1.16.0") >= 0 {
		appc.Env = append(appc.Env, []corev1.EnvVar{
			{
				Name:  "MYSQL_NOTIFY_SOCKET",
				Value: "/var/lib/mysql/notify.sock",
			},
			{
				Name:  "MYSQL_STATE_FILE",
				Value: "/var/lib/mysql/mysql.state",
			},
		}...)
	}

	if cr.CompareVersionWith("1.19.0") >= 0 {
		setLDPreloadEnv(ctx, cl, cr, &appc)
	}

	return appc, nil
}

func setLDPreloadEnv(
	ctx context.Context,
	cl client.Client,
	cr *api.PerconaXtraDBCluster,
	appc *corev1.Container,
) {
	const (
		ldPreloadKey    = "LD_PRELOAD"
		libJemallocPath = "/usr/lib64/libjemalloc.so.1"
		libTcmallocPath = "/usr/lib64/libtcmalloc.so"
	)

	ldPreloadValue := ""

	// Determine the allocator
	switch strings.ToLower(cr.Spec.PXC.MySQLAllocator) {
	case "jemalloc":
		ldPreloadValue += ":" + libJemallocPath
	case "tcmalloc":
		ldPreloadValue += ":" + libTcmallocPath
	}

	// Set LD_PRELOAD via appc.Env always. It takes precedence over EnvFrom.
	// This ensures we're not breaking existing deployments with LD_PRELOAD set.
	envVarsSecret := &corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{
		Name:      cr.Spec.PXC.EnvVarsSecretName,
		Namespace: cr.Namespace}, envVarsSecret)
	if client.IgnoreNotFound(err) == nil {
		// Env vars are set via secret. Check if LD_PRELOAD is set.
		if val, ok := envVarsSecret.Data[ldPreloadKey]; ok {
			ldPreloadValue = string(val)
		}
	}

	if ldPreloadValue != "" {
		// prefix/suffix and consecutive : (colons) don't do any harm
		// but remove them for sanity.
		re := regexp.MustCompile(":+")
		ldPreloadValue = re.ReplaceAllString(ldPreloadValue, ":")
		ldPreloadValue = strings.Trim(ldPreloadValue, ":")

		appc.Env = append(appc.Env, corev1.EnvVar{
			Name:  ldPreloadKey,
			Value: ldPreloadValue,
		})
	}
}

func (c *Node) SidecarContainers(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	return nil, nil
}

func (c *Node) LogCollectorContainer(spec *api.LogCollectorSpec, logPsecrets string, logRsecrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	logProcEnvs := []corev1.EnvVar{
		{
			Name:  "LOG_DATA_DIR",
			Value: "/var/lib/mysql",
		},
		{
			Name: "POD_NAMESPASE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
	}

	logRotEnvs := []corev1.EnvVar{
		{
			Name:  "SERVICE_TYPE",
			Value: "mysql",
		},
		{
			Name: "MONITOR_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(logRsecrets, users.Monitor),
			},
		},
	}

	fvar := true
	logProcContainer := corev1.Container{
		Name:            "logs",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Env:             logProcEnvs,
		SecurityContext: spec.ContainerSecurityContext,
		Resources:       spec.Resources,
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: logPsecrets,
					},
					Optional: &fvar,
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
		},
	}

	logRotContainer := corev1.Container{
		Name:            "logrotate",
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		Env:             logRotEnvs,
		SecurityContext: spec.ContainerSecurityContext,
		Resources:       spec.Resources,
		Args: []string{
			"logrotate",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      app.DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
		},
	}

	if cr.Spec.LogCollector != nil {
		if cr.Spec.LogCollector.Configuration != "" {
			logProcContainer.VolumeMounts = append(logProcContainer.VolumeMounts, corev1.VolumeMount{
				Name:      "logcollector-config",
				MountPath: "/etc/fluentbit/custom",
			})
		}

		if cr.Spec.LogCollector.HookScript != "" {
			logProcContainer.VolumeMounts = append(logProcContainer.VolumeMounts, corev1.VolumeMount{
				Name:      "hookscript",
				MountPath: "/opt/percona/hookscript",
			})
		}
	}

	return []corev1.Container{logProcContainer, logRotContainer}, nil
}

func (c *Node) PMMContainer(ctx context.Context, cl client.Client, spec *api.PMMSpec, secret *corev1.Secret, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
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

		pmm3Container.Env = append(pmm3Container.Env, pmm3PXCNodeEnvVars(cr.Spec.PMM.PxcParams)...)

		pmm3Container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      app.DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
		}

		pBool := true
		pmm3Container.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.PXC.EnvVarsSecretName,
					},
					Optional: &pBool,
				},
			},
		}

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
			Value: "mysql",
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
			Name:  "DB_ARGS",
			Value: "--query-source=perfschema",
		},
	}

	ct.Env = append(ct.Env, pmmEnvs...)

	if cr.CompareVersionWith("1.2.0") >= 0 {
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
		ct.Env = append(ct.Env, clusterEnvs...)
		ct.Resources = spec.Resources
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		for k, v := range ct.Env {
			if v.Name == "DB_PORT" {
				ct.Env[k].Value = "33062"
				break
			}
		}
		PmmPxcParams := ""
		if spec.PxcParams != "" {
			PmmPxcParams = spec.PxcParams
		}
		clusterPmmEnvs := []corev1.EnvVar{
			{
				Name:  "CLUSTER_NAME",
				Value: clusterName,
			},
			{
				Name:  "PMM_ADMIN_CUSTOM_PARAMS",
				Value: PmmPxcParams,
			},
		}
		ct.Env = append(ct.Env, clusterPmmEnvs...)
		pmmAgentScriptEnv := app.PMMAgentScript(cr, "mysql")
		ct.Env = append(ct.Env, pmmAgentScriptEnv...)
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		fvar := true
		ct.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.PXC.EnvVarsSecretName,
					},
					Optional: &fvar,
				},
			},
		}

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

	ct.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      app.DataVolumeName,
			MountPath: "/var/lib/mysql",
		},
	}

	return &ct, nil
}

// pmm3PXCNodeEnvVars returns a list of environment variables to configure the PMM3 container for monitoring pxc node.
func pmm3PXCNodeEnvVars(PmmPxcParams string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DB_PORT",
			Value: "33062",
		},
		{
			Name:  "DB_TYPE",
			Value: "mysql",
		},
		{
			Name:  "DB_ARGS",
			Value: "--query-source=perfschema",
		},
		{
			Name:  "PMM_ADMIN_CUSTOM_PARAMS",
			Value: PmmPxcParams,
		},
	}
}

func (c *Node) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, vg api.CustomVolumeGetter) (*api.Volume, error) {
	vol := app.Volumes(podSpec, app.DataVolumeName)

	configVolume, err := vg(cr.Namespace, "config", config.CustomConfigMapName(cr.Name, "pxc"), true)
	if err != nil {
		return nil, err
	}

	sslVolume := app.GetSecretVolumes("ssl", podSpec.SSLSecretName, !cr.TLSEnabled())
	if cr.CompareVersionWith("1.15.0") < 0 {
		sslVolume = app.GetSecretVolumes("ssl", podSpec.SSLSecretName, cr.Spec.AllowUnsafeConfig)
	}

	vol.Volumes = append(
		vol.Volumes,
		app.GetTmpVolume("tmp"),
		configVolume,
		app.GetSecretVolumes("ssl-internal", podSpec.SSLInternalSecretName, true),
		sslVolume,
		app.GetConfigVolumes("auto-config", config.AutoTuneConfigMapName(cr.Name, app.Name)),
		app.GetSecretVolumes(VaultSecretVolumeName, podSpec.VaultSecretName, true),
		app.GetSecretVolumes("mysql-users-secret-file", "internal-"+cr.Name, false),
	)

	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Configuration != "" {
		vol.Volumes = append(vol.Volumes,
			app.GetConfigVolumes("logcollector-config", config.CustomConfigMapName(cr.Name, "logcollector")))
	}

	if cr.CompareVersionWith("1.11.0") >= 0 {
		if cr.Spec.PXC != nil && cr.Spec.PXC.HookScript != "" {
			vol.Volumes = append(vol.Volumes,
				app.GetConfigVolumes("hookscript", config.HookScriptConfigMapName(cr.Name, "pxc")))
		}

		if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.HookScript != "" {
			vol.Volumes = append(vol.Volumes,
				app.GetConfigVolumes("hookscript", config.HookScriptConfigMapName(cr.Name, "logcollector")))
		}
	}

	if cr.CompareVersionWith("1.14.0") >= 0 {
		vol.Volumes = append(vol.Volumes, app.GetSecretVolumes("mysql-init-file", cr.Name+"-mysql-init", true))
	}

	if cr.CompareVersionWith("1.16.0") >= 0 {
		for i := range vol.PVCs {
			vol.PVCs[i].Labels = c.Labels()
		}
	}

	return vol, nil
}

// StatefulSet returns a new statefulset object with empty spec.
func (c *Node) StatefulSet() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.cr.Name + "-" + app.Name,
			Namespace: c.cr.Namespace,
		},
	}
}

func (c *Node) Labels() map[string]string {
	return naming.LabelsPXC(c.cr)
}

func (c *Node) Service() string {
	return c.cr.Name + "-" + app.Name
}

func (c *Node) UpdateStrategy(cr *api.PerconaXtraDBCluster) appsv1.StatefulSetUpdateStrategy {
	switch cr.Spec.UpdateStrategy {
	case appsv1.OnDeleteStatefulSetStrategyType:
		return appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType}
	case api.SmartUpdateStatefulSetStrategyType:
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
