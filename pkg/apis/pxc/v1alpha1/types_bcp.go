package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []PerconaXtraDBBackup `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PXCBackupSpec   `json:"spec"`
	Status            PXCBackupStatus `json:"status,omitempty"`
}

type PXCBackupSpec struct {
	Storage string `json:"storage,omitempty"`
}

type PXCBackupStatus struct {
}
