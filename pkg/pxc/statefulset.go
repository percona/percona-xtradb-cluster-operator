package pxc

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/features"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

// StatefulSet returns StatefulSet according to app to podSpec provided.
func StatefulSet(
	ctx context.Context,
	cl client.Client,
	sfs api.StatefulApp,
	podSpec *api.PodSpec,
	cr *api.PerconaXtraDBCluster,
	secret *corev1.Secret,
	initImageName string,
	vg api.CustomVolumeGetter,
) (*appsv1.StatefulSet, error) {
	log := logf.FromContext(ctx)

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

	pod.ServiceAccountName = podSpec.ServiceAccountName
	secrets := secret.Name
	pod.Affinity = PodAffinity(podSpec.Affinity, sfs)
	pod.TopologySpreadConstraints = PodTopologySpreadConstraints(podSpec.TopologySpreadConstraints, sfs.Labels())

	if sfs.Labels()[naming.LabelAppKubernetesComponent] == "haproxy" && cr.CompareVersionWith("1.7.0") == -1 {
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

	if features.Enabled(ctx, features.BackupXtrabackup) {
		pod.Volumes = append(pod.Volumes, corev1.Volume{
			Name: "backup-logs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	appC, err := sfs.AppContainer(podSpec, secrets, cr, pod.Volumes)
	if err != nil {
		return nil, errors.Wrap(err, "app container")
	}

	xbC, err := sfs.XtrabackupContainer(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "xtrabackup container")
	}
	if xbC != nil {
		pod.Containers = append(pod.Containers, *xbC)
	}

	pmmC, err := sfs.PMMContainer(ctx, cl, cr.Spec.PMM, secret, cr)
	if err != nil {
		log.Info(`"pmm container error"`, "secrets", cr.Spec.SecretsName, "internalSecrets", "internal-"+cr.Name, "error", err)
	}
	if pmmC != nil {
		pod.Containers = append(pod.Containers, *pmmC)
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

	initContainers := sfs.InitContainers(cr, initImageName)
	if len(initContainers) > 0 {
		pod.InitContainers = append(pod.InitContainers, initContainers...)
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

	customAnnotations := podSpec.Annotations
	if cr.CompareVersionWith("1.17.0") >= 0 {
		if customAnnotations == nil {
			customAnnotations = make(map[string]string)
		}
		customAnnotations["kubectl.kubernetes.io/default-container"] = sfs.Labels()[naming.LabelAppKubernetesComponent]
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
				Annotations: customAnnotations,
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

func PodTopologySpreadConstraints(tscs []corev1.TopologySpreadConstraint, ls map[string]string) []corev1.TopologySpreadConstraint {
	result := make([]corev1.TopologySpreadConstraint, 0, len(tscs))

	for _, tsc := range tscs {
		if tsc.LabelSelector == nil && tsc.MatchLabelKeys == nil && len(ls) > 0 {
			tsc.LabelSelector = &metav1.LabelSelector{
				MatchLabels: ls,
			}
		}
		if tsc.MaxSkew == 0 {
			tsc.MaxSkew = 1
		}
		if tsc.TopologyKey == "" {
			tsc.TopologyKey = api.DefaultAffinityTopologyKey
		}
		if tsc.WhenUnsatisfiable == "" {
			tsc.WhenUnsatisfiable = corev1.ScheduleAnyway
		}

		result = append(result, tsc)
	}
	return result
}
