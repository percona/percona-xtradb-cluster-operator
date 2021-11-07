package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBClusterBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []PerconaXtraDBClusterBackup `json:"items"`
}

func (list *PerconaXtraDBClusterBackupList) HasUnfinishedFinalizers() bool {
	for _, v := range list.Items {
		if v.ObjectMeta.DeletionTimestamp != nil && len(v.Finalizers) != 0 {
			return true
		}
	}

	return false
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBClusterBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PXCBackupSpec   `json:"spec"`
	Status            PXCBackupStatus `json:"status,omitempty"`
	SchedulerName     string          `json:"schedulerName,omitempty"`
	PriorityClassName string          `json:"priorityClassName,omitempty"`
}

type PXCBackupSpec struct {
	PXCCluster  string `json:"pxcCluster"`
	StorageName string `json:"storageName,omitempty"`
}

type PXCBackupStatus struct {
	State         PXCBackupState       `json:"state,omitempty"`
	CompletedAt   *metav1.Time         `json:"completed,omitempty"`
	LastScheduled *metav1.Time         `json:"lastscheduled,omitempty"`
	Destination   string               `json:"destination,omitempty"`
	StorageName   string               `json:"storageName,omitempty"`
	S3            *BackupStorageS3Spec `json:"s3,omitempty"`
}

type PXCBackupState string

const (
	BackupNew       PXCBackupState = ""
	BackupStarting  PXCBackupState = "Starting"
	BackupRunning   PXCBackupState = "Running"
	BackupFailed    PXCBackupState = "Failed"
	BackupSucceeded PXCBackupState = "Succeeded"
)

// OwnerRef returns OwnerReference to object
func (cr *PerconaXtraDBClusterBackup) OwnerRef(scheme *runtime.Scheme) (metav1.OwnerReference, error) {
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
