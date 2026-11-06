package v1

import (
	"context"
	"errors"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type",description="Backup type"
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

type PXCBackupType string

const (
	// BackupTypeFull is a standalone backup of the whole dataset, can be restored on its own.
	BackupTypeFull PXCBackupType = "full"
	// BackupTypeIncremental holds only the pages changed since the previous
	// backup in its chain. It can only be restored together with its base
	// (full) backup and every incremental taken between the two.
	BackupTypeIncremental PXCBackupType = "incremental"
)

// +kubebuilder:validation:XValidation:rule="!has(self.incrementalBaseBackupName) || self.incrementalBaseBackupName == \"\" || self.type == 'incremental'",message="Invalid configuration: incrementalBaseBackupName is only allowed for incremental backups"
type PXCBackupSpec struct {
	PXCCluster  string `json:"pxcCluster"`
	StorageName string `json:"storageName,omitempty"`
	// Type of the backup: "full" copies the whole dataset,
	// "incremental" copies only the pages changed since the previous backup in
	// the chain.
	// Must be one of "full" or "incremental". Defaults to "full".
	// +kubebuilder:validation:Enum=full;incremental
	// +kubebuilder:default:=full
	Type PXCBackupType `json:"type,omitempty"`
	// IncrementalBaseBackupName pins this incremental backup to a specific base
	// (full) backup, by the name of the PerconaXtraDBClusterBackup object.
	// Only used for incremental backups.
	// When set, the incremental backup will be deleted if the base backup is deleted.
	// If unset, the latest full backup in the same storage is used as the base.
	// +optional
	IncrementalBaseBackupName *string                 `json:"incrementalBaseBackupName,omitempty"`
	ContainerOptions          *BackupContainerOptions `json:"containerOptions,omitempty"`
	ActiveDeadlineSeconds     *int64                  `json:"activeDeadlineSeconds,omitempty"`
	StartingDeadlineSeconds   *int64                  `json:"startingDeadlineSeconds,omitempty"`
	SuspendedDeadlineSeconds  *int64                  `json:"suspendedDeadlineSeconds,omitempty"`
	// RunningDeadlineSeconds is the number of seconds to wait for the backup to transition to the 'Running' state.
	// Once this threshold is reached, the backup will be marked as failed.
	// When unspecified, uses the value from the parent cluster's .spec.backup.runningDeadlineSeconds (which defaults to 5m).
	RunningDeadlineSeconds *int64 `json:"runningDeadlineSeconds,omitempty"`
	// The source pod from which the backup is taken.
	// Used only when XTRABACKUP_SIDECAR feature gate is set
	SourcePod string `json:"sourcePod,omitempty"`
}

type PXCBackupStatus struct {
	Type                  PXCBackupType                     `json:"type,omitempty"`
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
	SourcePod             string                            `json:"sourcePod,omitempty"`
}

func (status PXCBackupStatus) Equals(other PXCBackupStatus) bool {
	return status.Type == other.Type &&
		status.State == other.State &&
		status.Error == other.Error &&
		status.CompletedAt.Equal(other.CompletedAt) &&
		status.LastScheduled.Equal(other.LastScheduled) &&
		status.Destination == other.Destination &&
		status.StorageName == other.StorageName &&
		status.StorageType == other.StorageType &&
		status.Image == other.Image &&
		status.SSLSecretName == other.SSLSecretName &&
		status.SSLInternalSecretName == other.SSLInternalSecretName &&
		status.VaultSecretName == other.VaultSecretName &&
		status.LatestRestorableTime.Equal(other.LatestRestorableTime) &&
		status.SourcePod == other.SourcePod &&
		ptr.Deref(status.VerifyTLS, false) == ptr.Deref(other.VerifyTLS, false) &&
		status.Azure.equals(other.Azure) &&
		status.S3.equals(other.S3)
	// todo: PVC
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

const (
	IncrementalDirSuffix  = ".incr"
	IncrementalLeafSuffix = "-incr"
)

// IsIncremental reports whether the destination points at an incremental backup.
func (dest *PXCBackupDestination) IsIncremental() bool {
	segments := strings.Split(dest.PathWithoutBucket(), "/")
	if len(segments) < 2 {
		return false
	}

	leaf := segments[len(segments)-1]
	chainDir := segments[len(segments)-2]

	return strings.HasSuffix(leaf, IncrementalLeafSuffix) && strings.HasSuffix(chainDir, IncrementalDirSuffix)
}

// IncrementalBaseDestination returns the full destination of the base (full)
// backup this incremental backup is chained to. The destination is returned
// unchanged when it is not an incremental one, so it is safe to call on any
// destination.
func (dest *PXCBackupDestination) IncrementalBaseDestination() PXCBackupDestination {
	if dest == nil {
		return ""
	}
	if !dest.IsIncremental() {
		return *dest
	}

	// For an incremental destination the chain directory is the last element of
	// the prefix, so stripping ".incr" from it yields the base backup's own path.
	bucket, prefix := dest.BucketAndPrefix()
	basePath := strings.TrimSuffix(strings.TrimSuffix(prefix, "/"), IncrementalDirSuffix)

	return PXCBackupDestination(dest.StorageTypePrefix() + path.Join(bucket, basePath))
}

// IncrementalsDir returns the directory prefix that holds every incremental
// backup chained to the same base, with a trailing slash. It returns "" when
// the destination is not an incremental one.
func (dest *PXCBackupDestination) IncrementalsDir() string {
	if !dest.IsIncremental() {
		return ""
	}

	// BucketAndPrefix returns everything above the leaf, already slash-terminated,
	// which for an incremental destination is exactly the chain directory.
	_, prefix := dest.BucketAndPrefix()

	return prefix
}

func (cr *PerconaXtraDBClusterBackup) GetType() PXCBackupType {
	if cr.Status.Type == "" {
		return BackupTypeFull
	}
	return cr.Status.Type
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

func (s PXCBackupState) IsTerminal() bool {
	return s == BackupSucceeded || s == BackupFailed
}

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

func (status *PXCBackupStatus) IntoBackupStorageSpec() *BackupStorageSpec {
	if status == nil {
		return nil
	}

	spec := &BackupStorageSpec{
		Type:  status.StorageType,
		S3:    status.S3,
		Azure: status.Azure,
	}
	if status.PVC != nil {
		spec.Volume = &VolumeSpec{PersistentVolumeClaim: status.PVC}
	}

	return spec
}

func (bcp *PerconaXtraDBClusterBackup) IsUsingSameStorage(
	ctx context.Context,
	cl client.Client,
	storage *BackupStorageSpec,
) (bool, error) {
	if bcp == nil || storage == nil {
		return false, nil
	}

	recorded := bcp.Status.IntoBackupStorageSpec()
	if storage.Type == BackupStorageAzure && recorded.Type == BackupStorageAzure {
		// Do not modify the originally provided pointers
		recorded, storage = recorded.DeepCopy(), storage.DeepCopy()

		if err := resolveAzureStorage(ctx, cl, bcp.Namespace, recorded.Azure); err != nil {
			return false, err
		}
		if err := resolveAzureStorage(ctx, cl, bcp.Namespace, storage.Azure); err != nil {
			return false, err
		}
	}
	return recorded.Equals(storage), nil
}

func resolveAzureStorage(
	ctx context.Context,
	cl client.Client,
	namespace string,
	spec *BackupStorageAzureSpec,
) error {
	if spec == nil || spec.azureStorageAccount != "" {
		return nil
	}

	if spec.CredentialsSecret == "" {
		return errors.New("azure storage has no credentialsSecret, so its storage account cannot be resolved")
	}

	secret := &corev1.Secret{}
	if err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: spec.CredentialsSecret}, secret); err != nil {
		return err
	}

	acc := string(secret.Data[AzureStorageAccountNameKey])
	if acc == "" {
		return errors.New("azure storage account name is not found in the credentials secret")
	}

	spec.azureStorageAccount = acc
	return nil
}
