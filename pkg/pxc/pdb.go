package pxc

import (
	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewPodDistributedBudget(cr *api.PerconaXtraDBCluster, pdbSpec *policyv1beta1.PodDisruptionBudgetSpec, componentName string) *policyv1beta1.PodDisruptionBudget {
	component := cr.Name + "-" + appName + componentName

	pdbSpec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       appName,
			"cluster":   cr.Name,
			"component": component,
		},
	}

	return &policyv1beta1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy/v1beta1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      component,
			Namespace: cr.Namespace,
		},
		Spec: *pdbSpec,
	}

}
