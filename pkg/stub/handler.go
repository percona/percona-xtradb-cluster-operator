package stub

import (
	"context"
	"fmt"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func NewHandler() sdk.Handler {
	return &Handler{}
}

type Handler struct {
	// Fill me
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.PerconaXtraDBCluster:
		// Just ignore it for now
		if event.Deleted {
			return nil
		}

		nodeSet, err := newStatefulSetNode(o)
		if err != nil {
			logrus.Error(err)
			return err
		}
		err = sdk.Create(nodeSet)
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("failed to create newStatefulSetNode: %v", err)
			return err
		}

		err = sdk.Create(newServiceNodes(o))
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("failed to create PXC Service: %v", err)
			return err
		}

		proxySet, err := newStatefulSetProxySQL(o)
		if err != nil {
			logrus.Error(err)
			return err
		}
		err = sdk.Create(proxySet)
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("failed to create newStatefulSetProxySQL: %v", err)
			return err
		}

		err = sdk.Create(newServiceProxySQL(o))
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("failed to create PXC Service: %v", err)
			return err
		}
	}
	return nil
}

func newServiceNodes(cr *v1alpha1.PerconaXtraDBCluster) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pxc-nodes", //cr.Name,
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
			Labels: map[string]string{
				"app": "pxc",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 3306,
					Name: "mysql-port",
				},
			},
			ClusterIP: "None",
			Selector: map[string]string{
				"component": "pxc-nodes",
			},
		},
	}
}

func newServiceProxySQL(cr *v1alpha1.PerconaXtraDBCluster) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pxc-proxysql",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": "pxc",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:     3306,
					Name:     "mysql",
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 3306,
					},
				},
				{
					Port:     6032,
					Name:     "proxyadm",
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 6032,
					},
				},
			},
			Selector: map[string]string{
				"component": "pxc-proxysql",
			},
		},
	}
}

func newStatefulSetNode(cr *v1alpha1.PerconaXtraDBCluster) (*appsv1.StatefulSet, error) {
	ls := map[string]string{
		"app":       "pxc",
		"component": "pxc-nodes",
	}

	var fsgroup *int64
	if cr.Spec.Platform == v1alpha1.PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}

	rcpuQnt, err := resource.ParseQuantity(cr.Spec.PXC.Resources.Requests.CPU)
	if err != nil {
		return nil, fmt.Errorf("wrong CPU resources: %v", err)
	}
	rmemQnt, err := resource.ParseQuantity(cr.Spec.PXC.Resources.Requests.Memory)
	if err != nil {
		return nil, fmt.Errorf("wrong memory resources: %v", err)
	}
	lcpuQnt, err := resource.ParseQuantity(cr.Spec.PXC.Resources.Limits.CPU)
	if err != nil {
		return nil, fmt.Errorf("wrong CPU resources: %v", err)
	}
	lmemQnt, err := resource.ParseQuantity(cr.Spec.PXC.Resources.Limits.Memory)
	if err != nil {
		return nil, fmt.Errorf("wrong memory resources: %v", err)
	}

	rvolStorage, err := resource.ParseQuantity(cr.Spec.PXC.VolumeSpec.Size)
	if err != nil {
		return nil, fmt.Errorf("wrong storage resources: %v", err)
	}

	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1beta2",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pxc-node",
			Namespace: cr.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &cr.Spec.PXC.Size,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			ServiceName: "pxc-nodes",
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						SupplementalGroups: []int64{99},
						FSGroup:            fsgroup,
					},
					Containers: []corev1.Container{{
						Name:            "node",
						Image:           cr.Spec.PXC.Image,
						ImagePullPolicy: corev1.PullAlways,
						ReadinessProbe: setProbeCmd(&corev1.Probe{
							InitialDelaySeconds: 15,
							TimeoutSeconds:      15,
							PeriodSeconds:       30,
							FailureThreshold:    5,
						}, "/usr/bin/clustercheck.sh"),
						LivenessProbe: setProbeCmd(&corev1.Probe{
							InitialDelaySeconds: 300,
							TimeoutSeconds:      5,
							PeriodSeconds:       10,
						}, "/usr/bin/clustercheck.sh"),
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 3306,
								Name:          "mysql",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "datadir",
								MountPath: "/var/lib/mysql",
							},
							{
								Name:      "config-volume",
								MountPath: "/etc/mysql/conf.d/",
							},
						},
						Env: []corev1.EnvVar{
							{
								Name: "MYSQL_ROOT_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "root"),
								},
							},
							{
								Name: "CLUSTERCHECK_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "clustercheck"),
								},
							},
							{
								Name: "XTRABACKUP_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "xtrabackup"),
								},
							},
							{
								Name: "MONITOR_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "monitor"),
								},
							},
							{
								Name: "CLUSTERCHECK_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "clustercheck"),
								},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    rcpuQnt,
								corev1.ResourceMemory: rmemQnt,
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    lcpuQnt,
								corev1.ResourceMemory: lmemQnt,
							},
						},
					}},
					Volumes: getConfigVolumes(),
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "datadir",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: cr.Spec.PXC.VolumeSpec.StorageClass,
						AccessModes:      cr.Spec.PXC.VolumeSpec.AccessModes,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: rvolStorage,
							},
						},
					},
				},
			},
		},
	}, nil
}

func newStatefulSetProxySQL(cr *v1alpha1.PerconaXtraDBCluster) (*appsv1.StatefulSet, error) {
	ls := map[string]string{
		"app":       "pxc",
		"component": "pxc-proxysql",
	}

	var fsgroup *int64
	if cr.Spec.Platform == v1alpha1.PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}

	rcpuQnt, err := resource.ParseQuantity(cr.Spec.ProxySQL.Resources.Requests.CPU)
	if err != nil {
		return nil, fmt.Errorf("wrong CPU resources: %v", err)
	}
	rmemQnt, err := resource.ParseQuantity(cr.Spec.ProxySQL.Resources.Requests.Memory)
	if err != nil {
		return nil, fmt.Errorf("wrong memory resources: %v", err)
	}
	lcpuQnt, err := resource.ParseQuantity(cr.Spec.ProxySQL.Resources.Limits.CPU)
	if err != nil {
		return nil, fmt.Errorf("wrong CPU resources: %v", err)
	}
	lmemQnt, err := resource.ParseQuantity(cr.Spec.ProxySQL.Resources.Limits.Memory)
	if err != nil {
		return nil, fmt.Errorf("wrong memory resources: %v", err)
	}

	rvolStorage, err := resource.ParseQuantity(cr.Spec.ProxySQL.VolumeSpec.Size)
	if err != nil {
		return nil, fmt.Errorf("wrong storage resources: %v", err)
	}

	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pxc-proxysql",
			Namespace: cr.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &cr.Spec.ProxySQL.Size,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			ServiceName: "pxc-proxysql",
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						SupplementalGroups: []int64{99},
						FSGroup:            fsgroup,
					},
					Containers: []corev1.Container{{
						Name:            "node",
						Image:           cr.Spec.ProxySQL.Image,
						ImagePullPolicy: corev1.PullAlways,
						ReadinessProbe: setProbeCmd(&corev1.Probe{
							InitialDelaySeconds: 15,
							TimeoutSeconds:      15,
							PeriodSeconds:       30,
							FailureThreshold:    5,
						}, "/usr/bin/clustercheck.sh"),
						LivenessProbe: setProbeCmd(&corev1.Probe{
							InitialDelaySeconds: 300,
							TimeoutSeconds:      5,
							PeriodSeconds:       10,
						}, "/usr/bin/clustercheck.sh"),
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
								Name:      "proxydata",
								MountPath: "/var/lib/proxysql",
								SubPath:   "data",
							},
						},
						Env: []corev1.EnvVar{
							{
								Name: "MYSQL_ROOT_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "root"),
								},
							},
							{
								Name:  "MYSQL_PROXY_USER",
								Value: "proxyuser",
							},
							{
								Name: "MYSQL_PROXY_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "proxyuser"),
								},
							},
							{
								Name: "MONITOR_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "monitor"),
								},
							},
							{
								Name:  "PXCSERVICE",
								Value: "pxc-nodes",
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    rcpuQnt,
								corev1.ResourceMemory: rmemQnt,
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    lcpuQnt,
								corev1.ResourceMemory: lmemQnt,
							},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "proxydata",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: cr.Spec.PXC.VolumeSpec.StorageClass,
						AccessModes:      cr.Spec.PXC.VolumeSpec.AccessModes,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: rvolStorage,
							},
						},
					},
				},
			},
		},
	}, nil
}

func secretKeySelector(name, key string) *corev1.SecretKeySelector {
	evs := &corev1.SecretKeySelector{}
	evs.Name = name
	evs.Key = key

	return evs
}

func setProbeCmd(pb *corev1.Probe, cmd ...string) *corev1.Probe {
	pb.Exec = &corev1.ExecAction{
		Command: cmd,
	}
	return pb
}

func getConfigVolumes() []corev1.Volume {
	vol1 := corev1.Volume{
		Name: "config-volume",
	}

	vol1.ConfigMap = &corev1.ConfigMapVolumeSource{}
	vol1.ConfigMap.Name = "pxc"
	t := true
	vol1.ConfigMap.Optional = &t

	volumes := []corev1.Volume{}
	return append(volumes, vol1)
}
