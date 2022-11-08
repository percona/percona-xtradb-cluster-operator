package pxc

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

// StatefulSet returns StatefulSet according for app to podSpec
func StatefulSet(sfs api.StatefulApp, podSpec *api.PodSpec, cr *api.PerconaXtraDBCluster, secret *corev1.Secret,
	initContainers []corev1.Container, log logr.Logger, vg api.CustomVolumeGetter) (*appsv1.StatefulSet, error) {

	pod := corev1.PodSpec{
		SecurityContext:               podSpec.PodSecurityContext,
		NodeSelector:                  podSpec.NodeSelector,
		Tolerations:                   podSpec.Tolerations,
		SchedulerName:                 podSpec.SchedulerName,
		PriorityClassName:             podSpec.PriorityClassName,
		ImagePullSecrets:              podSpec.ImagePullSecrets,
		TerminationGracePeriodSeconds: podSpec.TerminationGracePeriodSeconds,
		RuntimeClassName:              podSpec.RuntimeClassName,
	}
	if cr.CompareVersionWith("1.5.0") >= 0 {
		pod.ServiceAccountName = podSpec.ServiceAccountName
	}
	secrets := secret.Name
	pod.Affinity = PodAffinity(podSpec.Affinity, sfs)

	if sfs.Labels()["app.kubernetes.io/component"] == "haproxy" && cr.CompareVersionWith("1.7.0") == -1 {
		t := true
		pod.ShareProcessNamespace = &t
	}

	sfsVolume, err := sfs.Volumes(podSpec, cr, vg)
	if err != nil {
		return nil, fmt.Errorf("failed to get volumes %v", err)
	}

	if sfsVolume != nil && sfsVolume.Volumes != nil {
		pod.Volumes = sfsVolume.Volumes
	}

	appC, err := sfs.AppContainer(podSpec, secrets, cr, pod.Volumes)
	if err != nil {
		return nil, errors.Wrap(err, "app container")
	}

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		if !cr.Spec.PMM.HasSecret(secret) {
			log.Info(fmt.Sprintf(`Can't enable PMM: either "pmmserverkey" key doesn't exist in the %s, or %s and %s secrets are out of sync`, cr.Spec.SecretsName, cr.Spec.SecretsName, "internal-"+cr.Name))
		} else {
			pmmC, err := sfs.PMMContainer(cr.Spec.PMM, secret, cr)
			if err != nil {
				return nil, errors.Wrap(err, "pmm container error")
			}
			if pmmC != nil {
				pod.Containers = append(pod.Containers, *pmmC)
			}
		}
	}

	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.Enabled && cr.CompareVersionWith("1.7.0") >= 0 {
		logCollectorC, err := sfs.LogCollectorContainer(cr.Spec.LogCollector, cr.Spec.LogCollectorSecretName, secrets, cr)
		if err != nil {
			return nil, fmt.Errorf("logcollector container error: %v", err)
		}
		if logCollectorC != nil {
			pod.Containers = append(pod.Containers, logCollectorC...)
		}
	}

	if len(initContainers) > 0 {
		pod.InitContainers = append(pod.InitContainers, initContainers...)
	}

	if podSpec.ForceUnsafeBootstrap && cr.CompareVersionWith("1.10.0") < 0 {
		ic := appC.DeepCopy()
		ic.Name = ic.Name + "-init-unsafe"
		ic.Resources = podSpec.Resources
		ic.ReadinessProbe = nil
		ic.LivenessProbe = nil
		ic.Command = []string{"/var/lib/mysql/unsafe-bootstrap.sh"}
		pod.InitContainers = append(pod.InitContainers, *ic)
	}

	sideC, err := sfs.SidecarContainers(podSpec, secrets, cr)
	if err != nil {
		return nil, errors.Wrap(err, "sidecar container")
	}
	pod.Containers = append(pod.Containers, appC)
	pod.Containers = append(pod.Containers, sideC...)
	pod.Containers = api.AddSidecarContainers(log, pod.Containers, podSpec.Sidecars)
	pod.Volumes = api.AddSidecarVolumes(log, pod.Volumes, podSpec.SidecarVolumes)

	ls := sfs.Labels()

	customLabels := make(map[string]string, len(ls))
	for k, v := range ls {
		customLabels[k] = v
	}

	for k, v := range podSpec.Labels {
		if _, ok := customLabels[k]; !ok {
			customLabels[k] = v
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
				Labels:      customLabels,
				Annotations: podSpec.Annotations,
			},
			Spec: pod,
		},
		UpdateStrategy: sfs.UpdateStrategy(cr),
	}

	if sfsVolume != nil && sfsVolume.PVCs != nil {
		obj.Spec.VolumeClaimTemplates = sfsVolume.PVCs
	}
	obj.Spec.VolumeClaimTemplates = api.AddSidecarPVCs(log, obj.Spec.VolumeClaimTemplates, podSpec.SidecarPVCs)

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

func MergeTemplateAnnotations(sfs *appsv1.StatefulSet, annotations map[string]string) {
	if len(annotations) == 0 {
		return
	}
	if sfs.Spec.Template.Annotations == nil {
		sfs.Spec.Template.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		sfs.Spec.Template.Annotations[k] = v
	}
}
