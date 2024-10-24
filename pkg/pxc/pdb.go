package pxc

import (
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
)

func PodDisruptionBudget(cr *api.PerconaXtraDBCluster, spec *api.PodDisruptionBudgetSpec, labels map[string]string) *policyv1.PodDisruptionBudget {
	pdb := &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels[naming.LabelAppKubernetesInstance] + "-" + labels[naming.LabelAppKubernetesComponent],
			Namespace: cr.Namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   spec.MinAvailable,
			MaxUnavailable: spec.MaxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}
	if cr.CompareVersionWith("1.16.0") >= 0 {
		pdb.Labels = labels
	}

	return pdb
}
