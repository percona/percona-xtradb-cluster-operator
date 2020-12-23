package statefulset

import (
	"fmt"

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

func (c *Node) AppContainer(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) (corev1.Container, error) {
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

	res, err := app.CreateResources(spec.Resources)
	if err != nil {
		return appc, fmt.Errorf("create resources error: %v", err)
	}
	appc.Resources = res

	return appc, nil
}

func (c *Node) SidecarContainers(spec *api.PodSpec, secrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	return nil, nil
}

func (c *Node) LogCollectorContainer(spec *api.LogCollectorSpec, logPsecrets string, logRsecrets string, cr *api.PerconaXtraDBCluster) ([]corev1.Container, error) {
	res, err := app.CreateResources(spec.Resources)
	if err != nil {
		return nil, fmt.Errorf("create resources error: %v", err)
	}

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
		Resources:       res,
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
		Resources:       res,
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

	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Configuration != "" {
		logProcContainer.VolumeMounts = append(logProcContainer.VolumeMounts, corev1.VolumeMount{
			Name:      "logcollector-config",
			MountPath: "/etc/fluentbit/custom",
		})
	}

	return []corev1.Container{logProcContainer, logRotContainer}, nil
}

func (c *Node) PMMContainer(spec *api.PMMSpec, secrets string, cr *api.PerconaXtraDBCluster) (*corev1.Container, error) {
	ct := app.PMMClient(spec, secrets, cr.CompareVersionWith("1.2.0") >= 0, cr.CompareVersionWith("1.7.0") >= 0)

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
				SecretKeyRef: app.SecretKeySelector(secrets, "monitor"),
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
		res, err := app.CreateResources(spec.Resources)
		if err != nil {
			return nil, fmt.Errorf("create resources error: %v", err)
		}
		ct.Resources = res
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

	ct.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      DataVolumeName,
			MountPath: "/var/lib/mysql",
		},
	}

	return &ct, nil
}

func (c *Node) Volumes(podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster) (*api.Volume, error) {
	vol := app.Volumes(podSpec, DataVolumeName)
	ls := c.Labels()
	vol.Volumes = append(
		vol.Volumes,
		app.GetTmpVolume("tmp"),
		app.GetConfigVolumes("config", ls["app.kubernetes.io/instance"]+"-"+ls["app.kubernetes.io/component"]),
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
