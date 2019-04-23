package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PerconaXtraDBBackupRestoreSpec defines the desired state of PerconaXtraDBBackupRestore
type PerconaXtraDBBackupRestoreSpec struct {
	PXCCluster string `json:"pxcCluster"`
	BackupName string `json:"backupName"`
}

// PerconaXtraDBBackupRestoreStatus defines the observed state of PerconaXtraDBBackupRestore
type PerconaXtraDBBackupRestoreStatus struct {
	State         PXCBackupState `json:"state,omitempty"`
	CompletedAt   *metav1.Time   `json:"completed,omitempty"`
	LastScheduled *metav1.Time   `json:"lastscheduled,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBBackupRestore is the Schema for the perconaxtradbbackuprestores API
// +k8s:openapi-gen=true
type PerconaXtraDBBackupRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PerconaXtraDBBackupRestoreSpec   `json:"spec,omitempty"`
	Status PerconaXtraDBBackupRestoreStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBBackupRestoreList contains a list of PerconaXtraDBBackupRestore
type PerconaXtraDBBackupRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PerconaXtraDBBackupRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PerconaXtraDBBackupRestore{}, &PerconaXtraDBBackupRestoreList{})
}
