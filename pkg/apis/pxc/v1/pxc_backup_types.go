package v1

import (
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
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
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type",description="Backup type"
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

// PXCBackupType is the kind of backup taken.
type PXCBackupType string

const (
	// BackupTypeFull is a physical backup of the whole dataset, taken with xtrabackup.
	BackupTypeFull PXCBackupType = "full"
	// BackupTypeLogical is a SQL-level dump of the user schemas, taken with mydumper
	// and restored with myloader into a running cluster.
	BackupTypeLogical PXCBackupType = "logical"
)

type PXCBackupSpec struct {
	PXCCluster  string `json:"pxcCluster"`
	StorageName string `json:"storageName,omitempty"`
	// Type of the backup: "full" is a physical backup taken with xtrabackup,
	// "logical" is a SQL-level dump taken with mydumper.
	// Must be one of "full" or "logical". Defaults to "full".
	// Logical backups are supported only on s3 storage.
	// +optional
	// +kubebuilder:validation:Enum=full;logical
	// +kubebuilder:default:=full
	Type                     PXCBackupType           `json:"type,omitempty"`
	ContainerOptions         *BackupContainerOptions `json:"containerOptions,omitempty"`
	ActiveDeadlineSeconds    *int64                  `json:"activeDeadlineSeconds,omitempty"`
	StartingDeadlineSeconds  *int64                  `json:"startingDeadlineSeconds,omitempty"`
	SuspendedDeadlineSeconds *int64                  `json:"suspendedDeadlineSeconds,omitempty"`
	// RunningDeadlineSeconds is the number of seconds to wait for the backup to transition to the 'Running' state.
	// Once this threshold is reached, the backup will be marked as failed.
	// When unspecified, uses the value from the parent cluster's .spec.backup.runningDeadlineSeconds (which defaults to 5m).
	RunningDeadlineSeconds *int64 `json:"runningDeadlineSeconds,omitempty"`
}

type PXCBackupStatus struct {
	// Type is the kind of backup that was taken. Empty means "full".
	// +optional
	// +kubebuilder:validation:Enum=full;logical
	Type PXCBackupType `json:"type,omitempty"`
	// Size is the total size of the backup's objects in the storage, in bytes.
	// It is set for logical backups only, and is 0 when it could not be determined.
	// +optional
	Size                  int64                             `json:"size,omitempty"`
	State                 PXCBackupState                    `json:"state,omitempty"`
	Error                 string                            `json:"error,omitempty"`
	CompletedAt           *metav1.Time                      `json:"completed,omitempty"`
	LastScheduled         *metav1.Time                      `json:"lastscheduled,omitempty"`
	Destination           PXCBackupDestination              `json:"destination,omitempty"`
	StorageName           string                            `json:"storageName,omitempty"`
	S3                    *BackupStorageS3Spec              `json:"s3,omitempty"`
	Azure                 *BackupStorageAzureSpec           `json:"azure,omitempty"`
	PVC                   *corev1.PersistentVolumeClaimSpec `json:"pvc,omitempty"`
	StorageType           BackupStorageType                 `json:"storage_type"`
	Image                 string                            `json:"image,omitempty"`
	SSLSecretName         string                            `json:"sslSecretName,omitempty"`
	SSLInternalSecretName string                            `json:"sslInternalSecretName,omitempty"`
	VaultSecretName       string                            `json:"vaultSecretName,omitempty"`
	Conditions            []metav1.Condition                `json:"conditions,omitempty"`
	VerifyTLS             *bool                             `json:"verifyTLS,omitempty"`
	LatestRestorableTime  *metav1.Time                      `json:"latestRestorableTime,omitempty"`
}

// IsLogical reports whether the backup is a logical one.
// A nil status and an empty type are not logical.
func (status *PXCBackupStatus) IsLogical() bool {
	return status != nil && status.Type == BackupTypeLogical
}

type PXCBackupDestination string

func (dest *PXCBackupDestination) set(value string) {
	if dest == nil {
		return
	}
	*dest = PXCBackupDestination(value)
}

func (dest *PXCBackupDestination) SetPVCDestination(backupName string) {
	dest.set(PVCStoragePrefix + backupName)
}

func (dest *PXCBackupDestination) SetS3Destination(bucket, backupName string) {
	dest.set(AwsBlobStoragePrefix + bucket + "/" + backupName)
}

func (dest *PXCBackupDestination) SetAzureDestination(container, backupName string) {
	dest.set(AzureBlobStoragePrefix + container + "/" + backupName)
}

func (dest *PXCBackupDestination) String() string {
	if dest == nil {
		return ""
	}
	return string(*dest)
}

func (dest *PXCBackupDestination) StorageTypePrefix() string {
	for _, p := range []string{AwsBlobStoragePrefix, AzureBlobStoragePrefix, PVCStoragePrefix} {
		if strings.HasPrefix(dest.String(), p) {
			return p
		}
	}
	return ""
}

func (dest *PXCBackupDestination) PathWithoutBucket() string {
	_, prefix := dest.BucketAndPrefix()
	return path.Join(prefix, dest.BackupName())
}

func (dest *PXCBackupDestination) BucketAndPrefix() (string, string) {
	d := strings.TrimPrefix(dest.String(), dest.StorageTypePrefix())
	bucket, left, _ := strings.Cut(d, "/")

	spl := strings.Split(left, "/")
	prefix := ""
	if len(spl) > 1 {
		prefix = path.Join(spl[:len(spl)-1]...)
		prefix = strings.TrimSuffix(prefix, "/")
		prefix += "/"
	}
	return bucket, prefix
}

func (dest *PXCBackupDestination) BackupName() string {
	if dest.StorageTypePrefix() == PVCStoragePrefix {
		return strings.TrimPrefix(dest.String(), dest.StorageTypePrefix())
	}
	bucket, prefix := dest.BucketAndPrefix()
	backupName := strings.TrimPrefix(dest.String(), dest.StorageTypePrefix()+path.Join(bucket, prefix))
	backupName = strings.TrimPrefix(backupName, "/")
	return backupName
}

func (status *PXCBackupStatus) GetStorageType(cluster *PerconaXtraDBCluster) BackupStorageType {
	if status.StorageType != "" {
		return status.StorageType
	}

	if cluster != nil && cluster.Spec.Backup != nil {
		storage, ok := cluster.Spec.Backup.Storages[status.StorageName]
		if ok {
			return storage.Type
		}
	}

	switch {
	case status.S3 != nil:
		return BackupStorageS3
	case status.Azure != nil:
		return BackupStorageAzure
	case status.PVC != nil:
		return BackupStorageFilesystem
	}

	return ""
}

const (
	BackupConditionPITRReady = "PITRReady"
)

type PXCBackupState string

const (
	BackupNew       PXCBackupState = ""
	BackupSuspended PXCBackupState = "Suspended"
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

func (cr *PerconaXtraDBClusterBackup) SetFailedStatusWithError(err error) {
	cr.Status.State = BackupFailed
	cr.Status.Error = err.Error()
}

func (status *PXCBackupStatus) SetFsPvcFromPVC(pvc *corev1.PersistentVolumeClaim) {
	if status == nil || pvc == nil {
		return
	}

	status.PVC = pvc.Spec.DeepCopy()
}
