package pxc

import (
	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func PodDisruptionBudget(spec *policyv1beta1.PodDisruptionBudgetSpec, app api.StatefulApp, namespace string) *policyv1beta1.PodDisruptionBudget {
	labels := app.Labels()
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}

	return &policyv1beta1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy/v1beta1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      labels["component"],
			Namespace: namespace,
		},
		Spec: *spec,
	}

}
