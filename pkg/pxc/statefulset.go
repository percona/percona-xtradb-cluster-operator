package pxc

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
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
			SupplementalGroups: []int64{1001},
			FSGroup:            fsgroup,
		},
		NodeSelector:                  podSpec.NodeSelector,
		Tolerations:                   podSpec.Tolerations,
		SchedulerName:                 podSpec.SchedulerName,
		PriorityClassName:             podSpec.PriorityClassName,
		ImagePullSecrets:              podSpec.ImagePullSecrets,
		TerminationGracePeriodSeconds: podSpec.TerminationGracePeriodSeconds,
	}

	pod.Affinity = PodAffinity(podSpec.Affinity, sfs)
	sfsVolume := sfs.Volumes(podSpec)
	pod.Volumes = sfsVolume.Volumes

	var err error
	appC := sfs.AppContainer(podSpec, cr.Spec.SecretsName)
	appC.Resources, err = sfs.Resources(podSpec.Resources)
	if err != nil {
		return nil, err
	}

	if podSpec.ForceUnsafeBootstrap {
		ic := appC.DeepCopy()
		ic.Name = ic.Name + "-init"
		ic.ReadinessProbe = nil
		ic.LivenessProbe = nil
		ic.Command = []string{"/unsafe-bootstrap.sh"}
		pod.InitContainers = append(pod.InitContainers, *ic)
	}

	pod.Containers = append(pod.Containers, appC)
	pod.Containers = append(pod.Containers, sfs.SidecarContainers(podSpec, cr.Spec.SecretsName)...)

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		pmmC := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName, !cr.VersionLessThan120())
		if !cr.VersionLessThan120() {
			res, err := sfs.Resources(cr.Spec.PMM.Resources)
			if err != nil {
				return nil, fmt.Errorf("pmm container error: create resources error: %v", err)
			}
			pmmC.Resources = res
		}
		pod.Containers = append(pod.Containers, pmmC)
	}

	ls := sfs.Labels()
	for k, v := range podSpec.Labels {
		if _, ok := ls[k]; !ok {
			ls[k] = v
		}
	}

	obj := sfs.StatefulSet()
	obj.Spec = appsv1.StatefulSetSpec{
		Replicas: &podSpec.Size,
		Selector: &metav1.LabelSelector{
			MatchLabels: ls,
		},
		ServiceName: sfs.Service(),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      ls,
				Annotations: podSpec.Annotations,
			},
			Spec: pod,
		},
	}

	if sfsVolume.PVCs != nil {
		obj.Spec.VolumeClaimTemplates = sfsVolume.PVCs
	}

	obj.Spec.UpdateStrategy.Type = cr.Spec.UpdateStrategy

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
		if strings.ToLower(*af.TopologyKey) == api.AffinityTopologyKeyOff {
			return nil
		}
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: app.Labels(),
						},
						TopologyKey: *af.TopologyKey,
					},
				},
			},
		}
	}

	return nil
}
