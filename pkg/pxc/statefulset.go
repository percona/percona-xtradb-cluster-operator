package pxc

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func (h *PXC) StatefulSet(sfs api.StatefulApp, cr *api.PerconaXtraDBCluster) (*appsv1.StatefulSet, error) {
	var fsgroup *int64
	if h.serverVersion.Platform == api.PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}
	pod := corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			SupplementalGroups: []int64{99},
			FSGroup:            fsgroup,
		},
	}

	var err error
	appC := sfs.AppContainer(cr.Spec.PXC, cr.Spec.SecretsName)
	appC.Resources, err = sfs.Resources(cr.Spec.PXC.Resources)
	if err != nil {
		return nil, err
	}
	pod.Containers = append(pod.Containers, appC)

	if cr.Spec.PMM.Enabled {
		pod.Containers = append(pod.Containers, sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName))
	}

	pod.Volumes = []corev1.Volume{
		getConfigVolumes(),
	}

	ls := sfs.Lables()
	obj := sfs.StatefulSet()
	pvcs, err := sfs.PVCs(cr.Spec.PXC.VolumeSpec)
	if err != nil {
		return nil, err
	}

	obj.Spec = appsv1.StatefulSetSpec{
		Replicas: &cr.Spec.PXC.Size,
		Selector: &metav1.LabelSelector{
			MatchLabels: ls,
		},
		ServiceName: ls["component"],
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: ls,
			},
			Spec: pod,
		},
		VolumeClaimTemplates: pvcs,
	}

	addOwnerRefToObject(obj, asOwner(cr))

	return obj, nil
}

// func (h *PXC) newStatefulSetNode(cr *api.PerconaXtraDBCluster) (*appsv1.StatefulSet, error) {
// 	ls := map[string]string{
// 		"app":       appName,
// 		"component": cr.Name + "-" + appName + "-nodes",
// 		"cluster":   cr.Name,
// 	}

// 	var fsgroup *int64
// 	if h.serverVersion.Platform == api.PlatformKubernetes {
// 		var tp int64 = 1001
// 		fsgroup = &tp
// 	}

// 	resources, err := createResources(cr.Spec.PXC.Resources)
// 	if err != nil {
// 		return nil, fmt.Errorf("resources: %v", err)
// 	}

// 	rvolStorage, err := resource.ParseQuantity(cr.Spec.PXC.VolumeSpec.Size)
// 	if err != nil {
// 		return nil, fmt.Errorf("wrong storage resources: %v", err)
// 	}

// 	cfgPV := getConfigVolumes()
// 	podObj := corev1.PodSpec{
// 		SecurityContext: &corev1.PodSecurityContext{
// 			SupplementalGroups: []int64{99},
// 			FSGroup:            fsgroup,
// 		},
// 		Containers: []corev1.Container{
// 			{
// 				Name:            "node",
// 				Image:           cr.Spec.PXC.Image,
// 				ImagePullPolicy: corev1.PullAlways,
// 				ReadinessProbe: setProbeCmd(&corev1.Probe{
// 					InitialDelaySeconds: 15,
// 					TimeoutSeconds:      15,
// 					PeriodSeconds:       30,
// 					FailureThreshold:    5,
// 				}, "/usr/bin/clustercheck.sh"),
// 				LivenessProbe: setProbeCmd(&corev1.Probe{
// 					InitialDelaySeconds: 300,
// 					TimeoutSeconds:      5,
// 					PeriodSeconds:       10,
// 				}, "/usr/bin/clustercheck.sh"),
// 				Ports: []corev1.ContainerPort{
// 					{
// 						ContainerPort: 3306,
// 						Name:          "mysql",
// 					},
// 				},
// 				VolumeMounts: []corev1.VolumeMount{
// 					{
// 						Name:      "datadir",
// 						MountPath: "/var/lib/mysql",
// 					},
// 					{
// 						Name:      "config-volume",
// 						MountPath: "/etc/mysql/conf.d/",
// 					},
// 				},
// 				Env: []corev1.EnvVar{
// 					{
// 						Name: "MYSQL_ROOT_PASSWORD",
// 						ValueFrom: &corev1.EnvVarSource{
// 							SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "root"),
// 						},
// 					},
// 					{
// 						Name: "CLUSTERCHECK_PASSWORD",
// 						ValueFrom: &corev1.EnvVarSource{
// 							SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "clustercheck"),
// 						},
// 					},
// 					{
// 						Name: "XTRABACKUP_PASSWORD",
// 						ValueFrom: &corev1.EnvVarSource{
// 							SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "xtrabackup"),
// 						},
// 					},
// 					{
// 						Name: "MONITOR_PASSWORD",
// 						ValueFrom: &corev1.EnvVarSource{
// 							SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "monitor"),
// 						},
// 					},
// 					{
// 						Name: "CLUSTERCHECK_PASSWORD",
// 						ValueFrom: &corev1.EnvVarSource{
// 							SecretKeyRef: secretKeySelector(cr.Spec.SecretsName, "clustercheck"),
// 						},
// 					},
// 				},
// 				Resources: resources,
// 			},
// 		},
// 		Volumes: []corev1.Volume{
// 			cfgPV,
// 		},
// 	}

// 	if cr.Spec.PMM.Enabled {
// 		podObj.Containers = append(podObj.Containers, pmmNodeContainer(cr))
// 	}

// 	pvcObj := corev1.PersistentVolumeClaim{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name: "datadir",
// 		},
// 		Spec: corev1.PersistentVolumeClaimSpec{
// 			StorageClassName: cr.Spec.PXC.VolumeSpec.StorageClass,
// 			AccessModes:      cr.Spec.PXC.VolumeSpec.AccessModes,
// 			Resources: corev1.ResourceRequirements{
// 				Requests: corev1.ResourceList{
// 					corev1.ResourceStorage: rvolStorage,
// 				},
// 			},
// 		},
// 	}

// 	obj := h.NewStatefulSet("node", cr)
// 	obj.Spec = appsv1.StatefulSetSpec{
// 		Replicas: &cr.Spec.PXC.Size,
// 		Selector: &metav1.LabelSelector{
// 			MatchLabels: ls,
// 		},
// 		ServiceName: cr.Name + "-" + appName + "-nodes",
// 		Template: corev1.PodTemplateSpec{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Labels: ls,
// 			},
// 			Spec: podObj,
// 		},
// 		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
// 			pvcObj,
// 		},
// 	}
// 	addOwnerRefToObject(obj, asOwner(cr))

// 	return obj, nil
// }

func (h *PXC) newStatefulSetProxySQL(cr *api.PerconaXtraDBCluster) (*appsv1.StatefulSet, error) {
	ls := map[string]string{
		"app":       appName,
		"component": cr.Name + "-" + appName + "-proxysql",
		"cluster":   cr.Name,
	}

	var fsgroup *int64
	if h.serverVersion.Platform == api.PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}

	resources, err := createResources(cr.Spec.ProxySQL.Resources)
	if err != nil {
		return nil, fmt.Errorf("resources: %v", err)
	}

	rvolStorage, err := resource.ParseQuantity(cr.Spec.ProxySQL.VolumeSpec.Size)
	if err != nil {
		return nil, fmt.Errorf("wrong storage resources: %v", err)
	}

	obj := h.NewStatefulSet("proxysql", cr)

	obj.Spec = appsv1.StatefulSetSpec{
		Replicas: &cr.Spec.ProxySQL.Size,
		Selector: &metav1.LabelSelector{
			MatchLabels: ls,
		},
		ServiceName: cr.Name + "-" + appName + "-proxysql",
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: ls,
			},
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					SupplementalGroups: []int64{99},
					FSGroup:            fsgroup,
				},
				Containers: []corev1.Container{
					{
						Name:            "proxysql",
						Image:           cr.Spec.ProxySQL.Image,
						ImagePullPolicy: corev1.PullAlways,
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
								Value: cr.Name + "-" + appName + "-nodes",
							},
						},
						Resources: resources,
					},
				},
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
	}

	if cr.Spec.PMM.Enabled {
		obj.Spec.Template.Spec.Containers = append(
			obj.Spec.Template.Spec.Containers,
			pmmProxySQLContainer(cr),
		)
	}

	addOwnerRefToObject(obj, asOwner(cr))
	return obj, nil
}

// NewStatefulSet returns a new stateful set of a given object type (node/proxysql/etc)
func (*PXC) NewStatefulSet(objType string, cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-" + appName + "-" + objType,
			Namespace: cr.Namespace,
		},
	}
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

func getConfigVolumes() corev1.Volume {
	vol1 := corev1.Volume{
		Name: "config-volume",
	}

	vol1.ConfigMap = &corev1.ConfigMapVolumeSource{}
	vol1.ConfigMap.Name = appName
	t := true
	vol1.ConfigMap.Optional = &t
	return vol1
}
