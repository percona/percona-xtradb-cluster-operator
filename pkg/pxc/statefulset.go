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
func StatefulSet(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, initContainers []corev1.Container) (*appsv1.StatefulSet, error) {
	pod := corev1.PodSpec{
		SecurityContext:               podSpec.PodSecurityContext,
		NodeSelector:                  podSpec.NodeSelector,
		Tolerations:                   podSpec.Tolerations,
		SchedulerName:                 podSpec.SchedulerName,
		PriorityClassName:             podSpec.PriorityClassName,
		ImagePullSecrets:              podSpec.ImagePullSecrets,
		TerminationGracePeriodSeconds: podSpec.TerminationGracePeriodSeconds,
	}

	pod.Affinity = PodAffinity(podSpec.Affinity, sfs)

	sfsVolume, err := sfs.Volumes(podSpec, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to get volumes %v", err)
	}
	pod.Volumes = sfsVolume.Volumes

	appC, err := sfs.AppContainer(podSpec, cr.Spec.SecretsName, cr)
	if err != nil {
		return nil, errors.Wrap(err, "app container")
	}

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		pmmC, err := sfs.PMMContainer(cr.Spec.PMM, cr.Spec.SecretsName, cr)
		if err != nil {
			return nil, fmt.Errorf("pmm container error: %v", err)
		}
		pod.Containers = append(pod.Containers, pmmC)
	}

	if len(initContainers) > 0 {
		pod.InitContainers = append(pod.InitContainers, initContainers...)
	}

	if podSpec.ForceUnsafeBootstrap {
		ic := appC.DeepCopy()
		ic.Name = ic.Name + "-init-unsafe"
		ic.ReadinessProbe = nil
		ic.LivenessProbe = nil
		ic.Command = []string{"/var/lib/mysql/unsafe-bootstrap.sh"}
		pod.InitContainers = append(pod.InitContainers, *ic)
	}

	sideC, err := sfs.SidecarContainers(podSpec, cr.Spec.SecretsName)
	if err != nil {
		return nil, errors.Wrap(err, "sidecar container")
	}
	pod.Containers = append(pod.Containers, appC)
	pod.Containers = append(pod.Containers, sideC...)

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
		UpdateStrategy: sfs.UpdateStrategy(cr),
	}

	if sfsVolume.PVCs != nil {
		obj.Spec.VolumeClaimTemplates = sfsVolume.PVCs
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
