package v1

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PerconaXtraDBClusterRestoreSpec defines the desired state of PerconaXtraDBClusterRestore
type PerconaXtraDBClusterRestoreSpec struct {
	PXCCluster   string           `json:"pxcCluster"`
	BackupName   string           `json:"backupName"`
	BackupSource *PXCBackupStatus `json:"backupSource,omitempty"`
	PITR         *PITR            `json:"pitr,omitempty"`
}

// PerconaXtraDBClusterRestoreStatus defines the observed state of PerconaXtraDBClusterRestore
type PerconaXtraDBClusterRestoreStatus struct {
	State         BcpRestoreStates `json:"state,omitempty"`
	Comments      string           `json:"comments,omitempty"`
	CompletedAt   *metav1.Time     `json:"completed,omitempty"`
	LastScheduled *metav1.Time     `json:"lastscheduled,omitempty"`
}

type PITR struct {
	BackupSource *PXCBackupStatus `json:"backupSource"`
	Type         string           `json:"type"`
	Date         string           `json:"date"`
	GTIDSet      string           `json:"gtidSet"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBClusterRestore is the Schema for the perconaxtradbclusterrestores API
// +k8s:openapi-gen=true
type PerconaXtraDBClusterRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PerconaXtraDBClusterRestoreSpec   `json:"spec,omitempty"`
	Status PerconaXtraDBClusterRestoreStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBClusterRestoreList contains a list of PerconaXtraDBClusterRestore
type PerconaXtraDBClusterRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PerconaXtraDBClusterRestore `json:"items"`
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

func (cr *PerconaXtraDBClusterRestore) CheckNsetDefaults() error {
	if cr.Spec.PXCCluster == "" {
		return errors.New("pxcCluster can't be empty")
	}
	if cr.Spec.PITR != nil && cr.Spec.PITR.BackupSource != nil && cr.Spec.PITR.BackupSource.StorageName == "" && cr.Spec.PITR.BackupSource.S3 == nil {
		return errors.New("PITR.BackupSource.StorageName and PITR.BackupSource.S3 can't be empty simultaneously")
	}
	if cr.Spec.BackupName == "" && cr.Spec.BackupSource == nil {
		return errors.New("backupName and BackupSource can't be empty simultaneously")
	}

	return nil
}

func init() {
	SchemeBuilder.Register(&PerconaXtraDBClusterRestore{}, &PerconaXtraDBClusterRestoreList{})
}
