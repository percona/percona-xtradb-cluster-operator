package statefulset

import (
	"context"
	"fmt"
	"hash/fnv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/config"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/pkg/errors"
)

const (
	VaultSecretVolumeName = "vault-keyring-secret"
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

func (c *Node) Name() string {
	return app.Name
}

func (c *Node) AppContainer(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster, _ []corev1.Volume) (corev1.Container, error) {
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
				Value: c.labels["app.kubernetes.io/instance"] + "-" + c.labels["app.kubernetes.io/component"] + "-unready",
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

	if cr.CompareVersionWith("1.13.0") >= 0 {
		plugin := "caching_sha2_password"
		if cr.Spec.ProxySQLEnabled() {
			plugin = "mysql_native_password"
		}
		appc.Env = append(appc.Env, corev1.EnvVar{
			Name:  "DEFAULT_AUTHENTICATION_PLUGIN",
			Value: plugin,
		})
	}

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

	return appc, nil
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
	envVarsSecret := &corev1.Secret{}
	err := cl.Get(ctx, types.NamespacedName{Name: cr.Spec.PXC.EnvVarsSecretName, Namespace: cr.Namespace}, envVarsSecret)
	if client.IgnoreNotFound(err) != nil {
		return nil, errors.Wrap(err, "get env vars secret")
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
				Value: cr.Name,
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

func (c *Node) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, vg api.CustomVolumeGetter) (*api.Volume, error) {
	vol := app.Volumes(podSpec, app.DataVolumeName)

	configVolume, err := vg(cr.Namespace, "config", config.CustomConfigMapName(cr.Name, "pxc"), true)
	if err != nil {
		return nil, err
	}

	vol.Volumes = append(
		vol.Volumes,
		app.GetTmpVolume("tmp"),
		configVolume,
		app.GetSecretVolumes("ssl-internal", podSpec.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", podSpec.SSLSecretName, cr.Spec.AllowUnsafeConfig),
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

	return vol, nil
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
