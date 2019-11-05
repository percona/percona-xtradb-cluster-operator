package pxc

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
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
	v, err := cr.CompareVersionWith("1.3.0")
	if err != nil {
		return nil, errors.Wrap(err, "compare version")
	}

	sfsVolume := sfs.Volumes(podSpec, v)
	pod.Volumes = sfsVolume.Volumes

	appC := sfs.AppContainer(podSpec, cr.Spec.SecretsName, v)
	appC.Resources, err = sfs.Resources(podSpec.Resources)
	if err != nil {
		return nil, err
	}

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		var versionGreaterOrEqual120 bool
		compare, err := cr.CompareVersionWith("1.2.0")
		if err != nil {
			return nil, fmt.Errorf("compare version: %v", err)
		}
		if compare >= 1 {
			versionGreaterOrEqual120 = true
		}
		pmmC, err := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName, versionGreaterOrEqual120)
		if err != nil {
			return nil, fmt.Errorf("pmm container error: %v", err)
		}
		pod.Containers = append(pod.Containers, pmmC)
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
