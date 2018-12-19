package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// PerconaXtraDBClusterSpec defines the desired state of PerconaXtraDBCluster
type PerconaXtraDBClusterSpec struct {
	Platform    *Platform             `json:"platform,omitempty"`
	SecretsName string                `json:"secretsName,omitempty"`
	PXC         *PodSpec              `json:"pxc,omitempty"`
	ProxySQL    *PodSpec              `json:"proxysql,omitempty"`
	PMM         *PMMSpec              `json:"pmm,omitempty"`
	Backup      *[]PXCScheduledBackup `json:"backup,omitempty"`
}

type PXCScheduledBackup struct {
	Name     string          `json:"name,omitempty"`
	Schedule string          `json:"schedule,omitempty"`
	Keep     *int32          `json:"keep,omitempty"`
	Volume   PXCBackupVolume `json:"volume,omitempty"`
}

type ClusterState string

const (
	ClusterStateInit    ClusterState = ""
	ClusterStateRunning              = "running"
)

// PerconaXtraDBClusterStatus defines the observed state of PerconaXtraDBCluster
type PerconaXtraDBClusterStatus struct {
	State ClusterState
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBCluster is the Schema for the perconaxtradbclusters API
// +k8s:openapi-gen=true
type PerconaXtraDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PerconaXtraDBClusterSpec   `json:"spec,omitempty"`
	Status PerconaXtraDBClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBClusterList contains a list of PerconaXtraDBCluster
type PerconaXtraDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PerconaXtraDBCluster `json:"items"`
}

type PodSpec struct {
	Enabled    bool           `json:"enabled,omitempty"`
	Size       int32          `json:"size,omitempty"`
	Image      string         `json:"image,omitempty"`
	Resources  *PodResources  `json:"resources,omitempty"`
	VolumeSpec *PodVolumeSpec `json:"volumeSpec,omitempty"`
}

type PodResources struct {
	Requests *ResourcesList `json:"requests,omitempty"`
	Limits   *ResourcesList `json:"limits,omitempty"`
}

type PMMSpec struct {
	Enabled    bool   `json:"enabled,omitempty"`
	ServerHost string `json:"serverHost,omitempty"`
	Image      string `json:"image,omitempty"`
	ServerUser string `json:"serverUser,omitempty"`
}

type ResourcesList struct {
	Memory string `json:"memory,omitempty"`
	CPU    string `json:"cpu,omitempty"`
}

type PodVolumeSpec struct {
	AccessModes  []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	Size         string                              `json:"size,omitempty"`
	StorageClass *string                             `json:"storageClass,omitempty"`
}

type Platform string

const (
	PlatformUndef      Platform = ""
	PlatformKubernetes          = "kubernetes"
	PlatformOpenshift           = "openshift"
)

// ServerVersion represents info about k8s / openshift server version
type ServerVersion struct {
	Platform Platform
	Info     k8sversion.Info
}

type App interface {
	AppContainer(spec *PodSpec, secrets string) corev1.Container
	PMMContainer(spec *PMMSpec, secrets string) corev1.Container
	PVCs(spec *PodVolumeSpec) ([]corev1.PersistentVolumeClaim, error)
	// Resize(size int32) bool
	Resources(spec *PodResources) (corev1.ResourceRequirements, error)
	Lables() map[string]string
}

type StatefulApp interface {
	App
	StatefulSet() *appsv1.StatefulSet
}

// SetDefaults sets defaults options and overwrites obviously wrong settings
func (c *PerconaXtraDBClusterSpec) SetDefaults() {
	// pxc replicas shouldn't be less than 3
	if c.PXC.Size < 3 {
		c.PXC.Size = 3
	}

	// number of pxc replicas should be an odd
	if c.PXC.Size%2 == 0 {
		c.PXC.Size++
	}
}

// OwnerRef returns OwnerReference to object
func (cr *PerconaXtraDBCluster) OwnerRef(scheme *runtime.Scheme) (metav1.OwnerReference, error) {
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
