package statefulset

import (
	"fmt"
	"hash/fnv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	app "github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
)

const (
	DataVolumeName        = "datadir"
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
	appc := corev1.Container{
		Name:            app.Name,
		Image:           spec.Image,
		ImagePullPolicy: spec.ImagePullPolicy,
		ReadinessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: redinessDelay,
			TimeoutSeconds:      15,
			PeriodSeconds:       30,
			FailureThreshold:    5,
		}, "/usr/bin/clustercheck.sh"),
		LivenessProbe: app.Probe(&corev1.Probe{
			InitialDelaySeconds: livenessDelay,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
		}, "/usr/bin/clustercheck.sh"),
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
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      DataVolumeName,
				MountPath: "/var/lib/mysql",
			},
			{
				Name:      "config",
				MountPath: "/etc/mysql/conf.d",
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
		},
		Env: []corev1.EnvVar{
			{
				Name:  "PXC_SERVICE",
				Value: c.labels["app.kubernetes.io/instance"] + "-" + c.labels["app.kubernetes.io/component"] + "-unready",
			},
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
		},
		SecurityContext: spec.ContainerSecurityContext,
		Resources:       spec.Resources,
	}

	if cr.CompareVersionWith("1.1.0") >= 0 {
		appc.Env = append(appc.Env, corev1.EnvVar{})
		copy(appc.Env[2:], appc.Env[1:])
		appc.Env[1] = corev1.EnvVar{
			Name:  "MONITOR_HOST",
			Value: "%",
		}
	}
	if cr.CompareVersionWith("1.7.0") < 0 {
		appc.Env = append(appc.Env, corev1.EnvVar{
			Name: "CLUSTERCHECK_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "clustercheck"),
			},
		})
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "mysql-users-secret-file",
			MountPath: "/etc/mysql/mysql-users-secret",
		})
		if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Enabled {
			logEnvs := []corev1.EnvVar{
				{
					Name:  "LOG_DATA_DIR",
					Value: "/var/lib/mysql",
				},
				{
					Name:  "IS_LOGCOLLECTOR",
					Value: "yes",
				},
			}
			appc.Env = append(appc.Env, logEnvs...)
		}
	}
	if cr.CompareVersionWith("1.9.0") >= 0 {
		fvar := true
		appc.EnvFrom = append(appc.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cr.Spec.PXC.EnvVarsSecretName,
				},
				Optional: &fvar,
			},
		})
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
		appc.Env = append(appc.Env, corev1.EnvVar{
			Name:  "CLUSTER_HASH",
			Value: serverIDHashStr,
		})
	}
	if cr.CompareVersionWith("1.3.0") >= 0 {
		for k, v := range appc.VolumeMounts {
			if v.Name == "config" {
				appc.VolumeMounts[k].MountPath = "/etc/percona-xtradb-cluster.conf.d"
				break
			}
		}
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      "auto-config",
			MountPath: "/etc/my.cnf.d",
		})
	}

	if cr.CompareVersionWith("1.4.0") >= 0 {
		appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
			Name:      VaultSecretVolumeName,
			MountPath: "/etc/mysql/vault-keyring-secret",
		})
	}

	if cr.CompareVersionWith("1.5.0") >= 0 {
		appc.Args = []string{"mysqld"}
		appc.Command = []string{"/var/lib/mysql/pxc-entrypoint.sh"}
		appc.Env = append(appc.Env, corev1.EnvVar{
			Name: "OPERATOR_ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: app.SecretKeySelector(secrets, "operator"),
			},
		})
		if cr.CompareVersionWith("1.11.0") >= 0 && cr.Spec.PXC != nil && cr.Spec.PXC.HookScript != "" {
			appc.VolumeMounts = append(appc.VolumeMounts, corev1.VolumeMount{
				Name:      "hookscript",
				MountPath: "/opt/percona/hookscript",
			})
		}
	}

	if cr.CompareVersionWith("1.6.0") >= 0 {
		appc.Ports = append(
			appc.Ports,
			corev1.ContainerPort{
				ContainerPort: 33062,
				Name:          "mysql-admin",
			},
		)
		appc.ReadinessProbe.Exec.Command = []string{"/var/lib/mysql/readiness-check.sh"}
		appc.LivenessProbe.Exec.Command = []string{"/var/lib/mysql/liveness-check.sh"}
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		appc.Ports = append(
			appc.Ports,
			corev1.ContainerPort{
				ContainerPort: 33060,
				Name:          "mysqlx",
			},
		)

		appc.LivenessProbe = &spec.LivenessProbes
		appc.ReadinessProbe = &spec.ReadinessProbes
		appc.ReadinessProbe.Exec = &corev1.ExecAction{
			Command: []string{"/var/lib/mysql/readiness-check.sh"},
		}
		appc.LivenessProbe.Exec = &corev1.ExecAction{
			Command: []string{"/var/lib/mysql/liveness-check.sh"},
		}
		probsEnvs := []corev1.EnvVar{
			{
				Name:  "LIVENESS_CHECK_TIMEOUT",
				Value: fmt.Sprint(spec.LivenessProbes.TimeoutSeconds),
			},
			{
				Name:  "READINESS_CHECK_TIMEOUT",
				Value: fmt.Sprint(spec.ReadinessProbes.TimeoutSeconds),
			},
		}
		appc.Env = append(appc.Env, probsEnvs...)
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
				SecretKeyRef: app.SecretKeySelector(logRsecrets, "monitor"),
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
				Name:      DataVolumeName,
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
				Name:      DataVolumeName,
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

func (c *Node) PMMContainer(spec *api.PMMSpec, secret *corev1.Secret, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	ct := app.PMMClient(spec, secret, cr.CompareVersionWith("1.2.0") >= 0, cr.CompareVersionWith("1.7.0") >= 0)

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
				SecretKeyRef: app.SecretKeySelector(secret.Name, "monitor"),
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
		pmmAgentScriptEnv := app.PMMAgentScript("mysql")
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

	ct.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      DataVolumeName,
			MountPath: "/var/lib/mysql",
		},
	}

	return &ct, nil
}

func (c *Node) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, vg api.CustomVolumeGetter) (*api.Volume, error) {
	vol := app.Volumes(podSpec, DataVolumeName)
	ls := c.Labels()
	configVolume, err := vg(cr.Namespace, "config", ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"], true)
	if err != nil {
		return nil, err
	}
	vol.Volumes = append(
		vol.Volumes,
		app.GetTmpVolume("tmp"),
		configVolume,
		app.GetSecretVolumes("ssl-internal", podSpec.SSLInternalSecretName, true),
		app.GetSecretVolumes("ssl", podSpec.SSLSecretName, cr.Spec.AllowUnsafeConfig))
	if cr.CompareVersionWith("1.3.0") >= 0 {
		vol.Volumes = append(
			vol.Volumes,
			app.GetConfigVolumes("auto-config", "auto-"+ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"]))
	}
	if cr.CompareVersionWith("1.4.0") >= 0 {
		vol.Volumes = append(
			vol.Volumes,
			app.GetSecretVolumes(VaultSecretVolumeName, podSpec.VaultSecretName, true))
	}
	if cr.CompareVersionWith("1.7.0") >= 0 {
		vol.Volumes = append(vol.Volumes, app.GetSecretVolumes("mysql-users-secret-file", "internal-"+cr.Name, false))
		if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Configuration != "" {
			vol.Volumes = append(vol.Volumes, app.GetConfigVolumes("logcollector-config", ls["app.kubernetes.io/instance"]+"-logcollector"))
		}
	}
	if cr.CompareVersionWith("1.11.0") >= 0 {
		if cr.Spec.PXC != nil && cr.Spec.PXC.HookScript != "" {
			vol.Volumes = append(vol.Volumes,
				app.GetConfigVolumes("hookscript", ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"]+"-hookscript"))
		}

		if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.HookScript != "" {
			vol.Volumes = append(vol.Volumes,
				app.GetConfigVolumes("hookscript", ls["app.kubernetes.io/instance"]+"-logcollector-hookscript"))
		}
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
