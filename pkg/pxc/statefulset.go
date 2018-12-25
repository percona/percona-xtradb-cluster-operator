package pxc

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

// StatefulSet returns StatefulSet according for app to podSpec
func StatefulSet(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, sv *api.ServerVersion) (*appsv1.StatefulSet, error) {
	var fsgroup *int64
	if sv.Platform == api.PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}
	pod := corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			SupplementalGroups: []int64{99},
			FSGroup:            fsgroup,
		},
	}

	pod.Affinity = PodAffinity(podSpec.Affinity, sfs)

	var err error
	appC := sfs.AppContainer(podSpec, cr.Spec.SecretsName)
	appC.Resources, err = sfs.Resources(podSpec.Resources)
	if err != nil {
		return nil, err
	}
	pod.Containers = append(pod.Containers, appC)

	if cr.Spec.PMM.Enabled {
		pod.Containers = append(pod.Containers, sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName))
	}

	switch sfs.(type) {
	case *statefulset.Node:
		pod.Volumes = []corev1.Volume{
			getConfigVolumes(),
		}
	}

	ls := sfs.Lables()
	obj := sfs.StatefulSet()
	pvcs, err := sfs.PVCs(podSpec.VolumeSpec)
	if err != nil {
		return nil, err
	}

	obj.Spec = appsv1.StatefulSetSpec{
		Replicas: &podSpec.Size,
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

	return obj, nil
}

// PodAffinity returns podAffinity options for the pod
func PodAffinity(af *api.PodAffinity, app api.App) *corev1.Affinity {
	if af == nil {
		return nil
	}

	switch {
	case af.Advanced != nil:
		return af.Advanced
	case af.TopologyKey != nil:
		lables := app.Lables()
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "app",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{lables["app"]},
								},
								{
									Key:      "cluster",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{lables["cluster"]},
								},
								{
									Key:      "component",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{lables["component"]},
								},
							},
						},
						TopologyKey: *af.TopologyKey,
					},
				},
			},
		}
	}

	return nil
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
