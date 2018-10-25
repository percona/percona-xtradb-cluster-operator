package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []PerconaXtraDBCluster `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PerconaXtraDBClusterSpec   `json:"spec"`
	Status            PerconaXtraDBClusterStatus `json:"status,omitempty"`
}

type PerconaXtraDBClusterSpec struct {
	Size  int32  `json:"size"`
	Image string `json:"image,omitempty"`
}
type PerconaXtraDBClusterStatus struct {
	// Fill me
}
