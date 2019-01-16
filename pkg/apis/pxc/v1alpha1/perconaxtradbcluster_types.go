package v1alpha1

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// PerconaXtraDBClusterSpec defines the desired state of PerconaXtraDBCluster
type PerconaXtraDBClusterSpec struct {
	Platform    *Platform           `json:"platform,omitempty"`
	SecretsName string              `json:"secretsName,omitempty"`
	PXC         *PodSpec            `json:"pxc,omitempty"`
	ProxySQL    *PodSpec            `json:"proxysql,omitempty"`
	PMM         *PMMSpec            `json:"pmm,omitempty"`
	Backup      *PXCScheduledBackup `json:"backup,omitempty"`
}

type PXCScheduledBackup struct {
	Image            string                        `json:"image,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Schedule         []PXCScheduledBackupSchedule  `json:"schedule,omitempty"`
}

type PXCScheduledBackupSchedule struct {
	Name     string      `json:"name,omitempty"`
	Schedule string      `json:"schedule,omitempty"`
	Keep     int         `json:"keep,omitempty"`
	Volume   *VolumeSpec `json:"volume,omitempty"`
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
	Enabled           bool                          `json:"enabled,omitempty"`
	Size              int32                         `json:"size,omitempty"`
	Image             string                        `json:"image,omitempty"`
	Resources         *PodResources                 `json:"resources,omitempty"`
	VolumeSpec        VolumeSpec                    `json:"volumeSpec,omitempty"`
	Affinity          *PodAffinity                  `json:"affinity,omitempty"`
	NodeSelector      map[string]string             `json:"nodeSelector,omitempty"`
	Tolerations       []corev1.Toleration           `json:"tolerations,omitempty"`
	PriorityClassName string                        `json:"priorityClassName,omitempty"`
	Annotations       map[string]string             `json:"annotations,omitempty"`
	Labels            map[string]string             `json:"labels,omitempty"`
	ImagePullSecrets  []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

type PodAffinity struct {
	TopologyKey *string          `json:"antiAffinityTopologyKey,omitempty"`
	Advanced    *corev1.Affinity `json:"advanced,omitempty"`
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

type VolumeSpec struct {
	AccessModes  []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	Size         string                              `json:"size,omitempty"`
	SizeParsed   resource.Quantity                   `json:"-"`
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
	PVCs(spec *VolumeSpec) []corev1.PersistentVolumeClaim
	Resources(spec *PodResources) (corev1.ResourceRequirements, error)
	Lables() map[string]string
}

type StatefulApp interface {
	App
	StatefulSet() *appsv1.StatefulSet
}

const clusterNameMaxLen = 22

// ErrClusterNameOverflow upspring when the cluster name is longer than acceptable
var ErrClusterNameOverflow = fmt.Errorf("cluster (pxc) name too long, must be no more than %d characters", clusterNameMaxLen)

// CheckNSetDefaults sets defaults options and overwrites wrong settings
// and checks if other options' values are allowable
func (cr *PerconaXtraDBCluster) CheckNSetDefaults() error {
	if len(cr.Name) > clusterNameMaxLen {
		return ErrClusterNameOverflow
	}

	c := cr.Spec
	if c.PXC != nil {
		err := c.PXC.VolumeSpec.reconcileOpts()
		if err != nil {
			return fmt.Errorf("PXC.Volume: %v", err)
		}

		// pxc replicas shouldn't be less than 3
		if c.PXC.Size < 3 {
			c.PXC.Size = 3
		}

		// number of pxc replicas should be an odd
		if c.PXC.Size%2 == 0 {
			c.PXC.Size++
		}

		c.PXC.reconcileAffinityOpts()
	}

	if c.ProxySQL != nil && c.ProxySQL.Enabled {
		err := c.ProxySQL.VolumeSpec.reconcileOpts()
		if err != nil {
			return fmt.Errorf("ProxySQL.Volume: %v", err)
		}

		c.ProxySQL.reconcileAffinityOpts()
	}

	if c.Backup != nil {
		if c.Backup.Image == "" {
			return fmt.Errorf("backup.Image can't be empty")
		}

		for _, sch := range c.Backup.Schedule {
			err := sch.Volume.reconcileOpts()
			if err != nil {
				return fmt.Errorf("backup.Volume: %v", err)
			}
		}
	}

	return nil
}

var affinityValidTopologyKeys = map[string]struct{}{
	"kubernetes.io/hostname":                   struct{}{},
	"failure-domain.beta.kubernetes.io/zone":   struct{}{},
	"failure-domain.beta.kubernetes.io/region": struct{}{},
}

var defaultAffinityTopologyKey = "kubernetes.io/hostname"

const affinityOff = "none"

// reconcileAffinityOpts ensures that the affinity is set to the valid values.
// - if the affinity doesn't set at all - set topology key to `defaultAffinityTopologyKey`
// - if topology key is set and the value not the one of `affinityValidTopologyKeys` - set to `defaultAffinityTopologyKey`
// - if topology key set to valuse of `affinityOff` - disable the affinity at all
// - if `Advanced` affinity is set - leave everything as it is and set topology key to nil (Advanced options has a higher priority)
func (p *PodSpec) reconcileAffinityOpts() {
	switch {
	case p.Affinity == nil:
		p.Affinity = &PodAffinity{
			TopologyKey: &defaultAffinityTopologyKey,
		}

	case p.Affinity.TopologyKey == nil:
		p.Affinity.TopologyKey = &defaultAffinityTopologyKey

	case p.Affinity.Advanced != nil:
		p.Affinity.TopologyKey = nil

	case strings.ToLower(*p.Affinity.TopologyKey) == affinityOff:
		p.Affinity = nil

	case p.Affinity != nil && p.Affinity.TopologyKey != nil:
		if _, ok := affinityValidTopologyKeys[*p.Affinity.TopologyKey]; !ok {
			p.Affinity.TopologyKey = &defaultAffinityTopologyKey
		}
	}
}

func (v *VolumeSpec) reconcileOpts() error {
	if v.Size == "" {
		return fmt.Errorf("volume.Size can't be empty")
	}

	var err error
	v.SizeParsed, err = resource.ParseQuantity(v.Size)
	if err != nil {
		return fmt.Errorf("wrong volume size value %q: %v", v.Size, err)
	}

	if v.AccessModes == nil || len(v.AccessModes) == 0 {
		v.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	return nil
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
