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

// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName="pxc-backup";"pxc-backups"
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.pxcCluster",description="Cluster name"
// +kubebuilder:printcolumn:name="Storage",type="string",JSONPath=".status.storageName",description="Storage name from pxc spec"
// +kubebuilder:printcolumn:name="Destination",type="string",JSONPath=".status.destination",description="Backup destination"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="Job status"
// +kubebuilder:printcolumn:name="Completed",type="date",JSONPath=".status.completed",description="Completed time"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
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
	State                 PXCBackupState          `json:"state,omitempty"`
	CompletedAt           *metav1.Time            `json:"completed,omitempty"`
	LastScheduled         *metav1.Time            `json:"lastscheduled,omitempty"`
	Destination           string                  `json:"destination,omitempty"`
	StorageName           string                  `json:"storageName,omitempty"`
	S3                    *BackupStorageS3Spec    `json:"s3,omitempty"`
	Azure                 *BackupStorageAzureSpec `json:"azure,omitempty"`
	StorageType           BackupStorageType       `json:"storage_type"`
	Image                 string                  `json:"image,omitempty"`
	SSLSecretName         string                  `json:"sslSecretName,omitempty"`
	SSLInternalSecretName string                  `json:"sslInternalSecretName,omitempty"`
	VaultSecretName       string                  `json:"vaultSecretName,omitempty"`
	Conditions            []metav1.Condition      `json:"conditions,omitempty"`
}

const (
	BackupConditionPITRReady = "PITRReady"
)

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
