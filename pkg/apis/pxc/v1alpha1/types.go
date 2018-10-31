package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/version"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []PerconaXtraDBCluster `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PerconaXtraDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PerconaXtraDBClusterSpec   `json:"spec"`
	Status            PerconaXtraDBClusterStatus `json:"status,omitempty"`
}

type PerconaXtraDBClusterSpec struct {
	SecretsName string   `json:"secretsName,omitempty"`
	PXC         *PodSpec `json:"pxc,omitempty"`
	ProxySQL    *PodSpec `json:"proxysql,omitempty"`
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
	PMM      *PMMSpec       `json:"pmm,omitempty"`
}
type PMMSpec struct {
	Enabled bool   `json:"enabled,omitempty"`
	Service string `json:"monitoring-service,omitempty"`
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

type PerconaXtraDBClusterStatus struct {
	// Fill me
}

type Platform string

const (
	PlatformUndef      Platform = ""
	PlatformKubernetes Platform = "kubernetes"
	PlatformOpenshift  Platform = "openshift"
)

// ServerVersion represents info about k8s / openshift server version
type ServerVersion struct {
	Platform Platform
	Info     k8sversion.Info
}
