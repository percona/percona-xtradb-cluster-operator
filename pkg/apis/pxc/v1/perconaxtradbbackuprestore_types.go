package v1

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PerconaXtraDBBackupRestoreSpec defines the desired state of PerconaXtraDBBackupRestore
type PerconaXtraDBBackupRestoreSpec struct {
	PXCCluster string `json:"pxcCluster"`
	BackupName string `json:"backupName"`
}

// PerconaXtraDBBackupRestoreStatus defines the observed state of PerconaXtraDBBackupRestore
type PerconaXtraDBBackupRestoreStatus struct {
	State         BcpRestoreStates `json:"state,omitempty"`
	Comments      string           `json:"comments,omitempty"`
	CompletedAt   *metav1.Time     `json:"completed,omitempty"`
	LastScheduled *metav1.Time     `json:"lastscheduled,omitempty"`
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

type BcpRestoreStates string

const (
	RestoreNew          BcpRestoreStates = ""
	RestoreStarting                      = "Starting"
	RestoreStopCluster                   = "Stopping Cluster"
	RestoreRestore                       = "Restoring"
	RestoreStartCluster                  = "Starting Cluster"
	RestoreFailed                        = "Failed"
	RestoreSucceeded                     = "Succeeded"
)

func (cr *PerconaXtraDBBackupRestore) CheckNsetDefaults() error {
	if cr.Spec.PXCCluster == "" {
		return errors.New("pxcCluster can't be empty")
	}

	if cr.Spec.BackupName == "" {
		return errors.New("backupName can't be empty")
	}

	return nil
}

func init() {
	SchemeBuilder.Register(&PerconaXtraDBBackupRestore{}, &PerconaXtraDBBackupRestoreList{})
}
