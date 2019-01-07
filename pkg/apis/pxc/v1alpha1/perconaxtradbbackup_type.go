package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
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
	PXCCluster string      `json:"pxcCluster"`
	Volume     *VolumeSpec `json:"volume,omitempty"`
}

type PXCBackupStatus struct {
	State         PXCBackupState `json:"state,omitempty"`
	CompletedAt   *metav1.Time   `json:"completed,omitempty"`
	LastScheduled *metav1.Time   `json:"lastscheduled,omitempty"`
	Volume        string         `json:"volume,omitempty"`
}

type PXCBackupState string

const (
	BackupStarting  PXCBackupState = "Starting"
	BackupRunning                  = "Running"
	BackupFailed                   = "Failed"
	BackupSucceeded                = "Succeeded"
)

// OwnerRef returns OwnerReference to object
func (cr *PerconaXtraDBBackup) OwnerRef(scheme *runtime.Scheme) (metav1.OwnerReference, error) {
	gvk, err := apiutil.GVKForObject(cr, scheme)
	if err != nil {
		return metav1.OwnerReference{}, err
	}

	trueVar := true

	return metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       cr.GetName(),
		UID:        cr.GetUID(),
		Controller: &trueVar,
	}, nil
}

// CheckNSetDefaults sets defaults options and overwrites wrong settings
// and checks if other options' values are allowable
func (cr *PerconaXtraDBBackup) CheckNSetDefaults() error {
	if cr.Spec.Volume == nil {
		return fmt.Errorf("Volume can't be empty")
	}

	err := cr.Spec.Volume.reconcileOpts()
	if err != nil {
		return fmt.Errorf("Volume: %v", err)
	}

	return nil
}
