package v1

import (
	"encoding/json"
	"fmt"
	"strings"

	v "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sversion "k8s.io/apimachinery/pkg/version"
)

// PerconaXtraDBClusterSpec defines the desired state of PerconaXtraDBCluster
type PerconaXtraDBClusterSpec struct {
	Platform              *Platform                            `json:"platform,omitempty"`
	Pause                 bool                                 `json:"pause,omitempty"`
	SecretsName           string                               `json:"secretsName,omitempty"`
	VaultSecretName       string                               `json:"vaultSecretName,omitempty"`
	SSLSecretName         string                               `json:"sslSecretName,omitempty"`
	SSLInternalSecretName string                               `json:"sslInternalSecretName,omitempty"`
	PXC                   *PodSpec                             `json:"pxc,omitempty"`
	ProxySQL              *PodSpec                             `json:"proxysql,omitempty"`
	PMM                   *PMMSpec                             `json:"pmm,omitempty"`
	Backup                *PXCScheduledBackup                  `json:"backup,omitempty"`
	UpdateStrategy        appsv1.StatefulSetUpdateStrategyType `json:"updateStrategy,omitempty"`
	AllowUnsafeConfig     bool                                 `json:"allowUnsafeConfigurations,omitempty"`
}

const (
	SmartUpdateStatefulSetStrategyType appsv1.StatefulSetUpdateStrategyType = "SmartUpdate"
)

type PXCScheduledBackup struct {
	Image              string                        `json:"image,omitempty"`
	ImagePullSecrets   []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Schedule           []PXCScheduledBackupSchedule  `json:"schedule,omitempty"`
	Storages           map[string]*BackupStorageSpec `json:"storages,omitempty"`
	ServiceAccountName string                        `json:"serviceAccountName,omitempty"`
}

type PXCScheduledBackupSchedule struct {
	Name        string `json:"name,omitempty"`
	Schedule    string `json:"schedule,omitempty"`
	Keep        int    `json:"keep,omitempty"`
	StorageName string `json:"storageName,omitempty"`
}
type AppState string

const (
	AppStateUnknown AppState = "unknown"
	AppStateInit    AppState = "initializing"
	AppStateReady   AppState = "ready"
	AppStateError   AppState = "error"
)

// PerconaXtraDBClusterStatus defines the observed state of PerconaXtraDBCluster
type PerconaXtraDBClusterStatus struct {
	PXC                AppStatus          `json:"pxc,omitempty"`
	ProxySQL           AppStatus          `json:"proxysql,omitempty"`
	Host               string             `json:"host,omitempty"`
	Messages           []string           `json:"message,omitempty"`
	Status             AppState           `json:"state,omitempty"`
	Conditions         []ClusterCondition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse                   = "False"
	ConditionUnknown                 = "Unknown"
)

type ClusterConditionType string

const (
	ClusterReady      ClusterConditionType = "Ready"
	ClusterInit                            = "Initializing"
	ClusterPXCReady                        = "PXCReady"
	ClusterProxyReady                      = "ProxySQLReady"
	ClusterError                           = "Error"
)

type ClusterCondition struct {
	Status             ConditionStatus      `json:"status"`
	Type               ClusterConditionType `json:"type"`
	LastTransitionTime metav1.Time          `json:"lastTransitionTime,omitempty"`
	Reason             string               `json:"reason,omitempty"`
	Message            string               `json:"message,omitempty"`
}

type AppStatus struct {
	Size    int32    `json:"size,omitempty"`
	Ready   int32    `json:"ready,omitempty"`
	Status  AppState `json:"status,omitempty"`
	Message string   `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBCluster is the Schema for the perconaxtradbclusters API
// +k8s:openapi-gen=true
type PerconaXtraDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec    PerconaXtraDBClusterSpec   `json:"spec,omitempty"`
	Status  PerconaXtraDBClusterStatus `json:"status,omitempty"`
	version *v.Version
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBClusterList contains a list of PerconaXtraDBCluster
type PerconaXtraDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PerconaXtraDBCluster `json:"items"`
}

type PodSpec struct {
	Enabled                       bool                          `json:"enabled,omitempty"`
	Size                          int32                         `json:"size,omitempty"`
	Image                         string                        `json:"image,omitempty"`
	Resources                     *PodResources                 `json:"resources,omitempty"`
	SidecarResources              *PodResources                 `json:"sidecarResources,omitempty"`
	VolumeSpec                    *VolumeSpec                   `json:"volumeSpec,omitempty"`
	Affinity                      *PodAffinity                  `json:"affinity,omitempty"`
	NodeSelector                  map[string]string             `json:"nodeSelector,omitempty"`
	Tolerations                   []corev1.Toleration           `json:"tolerations,omitempty"`
	PriorityClassName             string                        `json:"priorityClassName,omitempty"`
	Annotations                   map[string]string             `json:"annotations,omitempty"`
	Labels                        map[string]string             `json:"labels,omitempty"`
	ImagePullSecrets              []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Configuration                 string                        `json:"configuration,omitempty"`
	PodDisruptionBudget           *PodDisruptionBudgetSpec      `json:"podDisruptionBudget,omitempty"`
	VaultSecretName               string                        `json:"vaultSecretName,omitempty"`
	SSLSecretName                 string                        `json:"sslSecretName,omitempty"`
	SSLInternalSecretName         string                        `json:"sslInternalSecretName,omitempty"`
	TerminationGracePeriodSeconds *int64                        `json:"gracePeriod,omitempty"`
	ForceUnsafeBootstrap          bool                          `json:"forceUnsafeBootstrap,omitempty"`
	ServiceType                   corev1.ServiceType            `json:"serviceType,omitempty"`
	SchedulerName                 string                        `json:"schedulerName,omitempty"`
	ReadinessInitialDelaySeconds  *int32                        `json:"readinessDelaySec,omitempty"`
	LivenessInitialDelaySeconds   *int32                        `json:"livenessDelaySec,omitempty"`
	PodSecurityContext            *corev1.PodSecurityContext    `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext      *corev1.SecurityContext       `json:"containerSecurityContext,omitempty"`
}

type PodDisruptionBudgetSpec struct {
	MinAvailable   *intstr.IntOrString `json:"minAvailable,omitempty"`
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
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
	Enabled                  bool                    `json:"enabled,omitempty"`
	ServerHost               string                  `json:"serverHost,omitempty"`
	Image                    string                  `json:"image,omitempty"`
	ServerUser               string                  `json:"serverUser,omitempty"`
	Resources                *PodResources           `json:"resources,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`
}

type ResourcesList struct {
	Memory           string `json:"memory,omitempty"`
	CPU              string `json:"cpu,omitempty"`
	EphemeralStorage string `json:"ephemeral-storage,omitempty"`
}

type BackupStorageSpec struct {
	Type                     BackupStorageType          `json:"type"`
	S3                       BackupStorageS3Spec        `json:"s3,omitempty"`
	Volume                   *VolumeSpec                `json:"volume,omitempty"`
	NodeSelector             map[string]string          `json:"nodeSelector,omitempty"`
	Resources                *PodResources              `json:"resources,omitempty"`
	Affinity                 *corev1.Affinity           `json:"affinity,omitempty"`
	Tolerations              []corev1.Toleration        `json:"tolerations,omitempty"`
	Annotations              map[string]string          `json:"annotations,omitempty"`
	Labels                   map[string]string          `json:"labels,omitempty"`
	SchedulerName            string                     `json:"schedulerName,omitempty"`
	PriorityClassName        string                     `json:"priorityClassName,omitempty"`
	PodSecurityContext       *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext    `json:"containerSecurityContext,omitempty"`
}

type BackupStorageType string

const (
	BackupStorageFilesystem BackupStorageType = "filesystem"
	BackupStorageS3         BackupStorageType = "s3"
)

type BackupStorageS3Spec struct {
	Bucket            string `json:"bucket"`
	CredentialsSecret string `json:"credentialsSecret"`
	Region            string `json:"region,omitempty"`
	EndpointURL       string `json:"endpointUrl,omitempty"`
}

type VolumeSpec struct {
	// EmptyDir to use as data volume for mysql. EmptyDir represents a temporary
	// directory that shares a pod's lifetime.
	// +optional
	EmptyDir *corev1.EmptyDirVolumeSource `json:"emptyDir,omitempty"`

	// HostPath to use as data volume for mysql. HostPath represents a
	// pre-existing file or directory on the host machine that is directly
	// exposed to the container.
	// +optional
	HostPath *corev1.HostPathVolumeSource `json:"hostPath,omitempty"`

	// PersistentVolumeClaim to specify PVC spec for the volume for mysql data.
	// It has the highest level of precedence, followed by HostPath and
	// EmptyDir. And represents the PVC specification.
	// +optional
	PersistentVolumeClaim *corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaim,omitempty"`
}

type Volume struct {
	PVCs    []corev1.PersistentVolumeClaim
	Volumes []corev1.Volume
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
	AppContainer(spec *PodSpec, secrets string, cr *PerconaXtraDBCluster) (corev1.Container, error)
	SidecarContainers(spec *PodSpec, secrets string) ([]corev1.Container, error)
	PMMContainer(spec *PMMSpec, secrets string, cr *PerconaXtraDBCluster) (corev1.Container, error)
	Volumes(podSpec *PodSpec, cr *PerconaXtraDBCluster) (*Volume, error)
	Labels() map[string]string
}

type StatefulApp interface {
	App
	StatefulSet() *appsv1.StatefulSet
	Service() string
	UpdateStrategy(cr *PerconaXtraDBCluster) appsv1.StatefulSetUpdateStrategy
}

const clusterNameMaxLen = 22

var defaultPXCGracePeriodSec int64 = 600
var livenessInitialDelaySeconds int32 = 300

// ErrClusterNameOverflow upspring when the cluster name is longer than acceptable
var ErrClusterNameOverflow = fmt.Errorf("cluster (pxc) name too long, must be no more than %d characters", clusterNameMaxLen)

func (cr *PerconaXtraDBCluster) setSecurityContext(serverVersion *ServerVersion) {
	if cr.Spec.Platform != nil {
		serverVersion.Platform = *cr.Spec.Platform
	}
	var fsgroup *int64
	if serverVersion.Platform == PlatformKubernetes {
		var tp int64 = 1001
		fsgroup = &tp
	}
	sc := &corev1.PodSecurityContext{
		SupplementalGroups: []int64{1001},
		FSGroup:            fsgroup,
	}

	if cr.Spec.PXC.PodSecurityContext == nil {
		cr.Spec.PXC.PodSecurityContext = sc
	}
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.PodSecurityContext == nil {
		cr.Spec.ProxySQL.PodSecurityContext = sc
	}
	if cr.Spec.Backup != nil {
		for k := range cr.Spec.Backup.Storages {
			if cr.Spec.Backup.Storages[k].PodSecurityContext == nil {
				cr.Spec.Backup.Storages[k].PodSecurityContext = sc
			}
		}
	}
}

// CheckNSetDefaults sets defaults options and overwrites wrong settings
// and checks if other options' values are allowable
// returned "changed" means CR should be updated on cluster
func (cr *PerconaXtraDBCluster) CheckNSetDefaults(serverVersion *ServerVersion) (changed bool, err error) {
	err = cr.setVersion()
	if err != nil {
		return false, errors.Wrap(err, "set version")
	}

	if len(cr.Name) > clusterNameMaxLen {
		return false, ErrClusterNameOverflow
	}

	c := cr.Spec
	if c.PXC == nil {
		return false, fmt.Errorf("spec.pxc section is not specified. Please check %s cluster settings", cr.Name)
	}

	if c.PXC != nil {
		if c.PXC.VolumeSpec == nil {
			return false, fmt.Errorf("PXC: volumeSpec should be specified")
		}
		changed, err = c.PXC.VolumeSpec.reconcileOpts()
		if err != nil {
			return false, fmt.Errorf("PXC.Volume: %v", err)
		}

		c.PXC.VaultSecretName = c.VaultSecretName
		if len(c.PXC.VaultSecretName) == 0 {
			c.PXC.VaultSecretName = cr.Name + "-vault"
		}

		if len(c.SSLSecretName) > 0 {
			c.PXC.SSLSecretName = c.SSLSecretName
		} else {
			c.PXC.SSLSecretName = cr.Name + "-ssl"
		}

		if len(c.SSLInternalSecretName) > 0 {
			c.PXC.SSLInternalSecretName = c.SSLInternalSecretName
		} else {
			c.PXC.SSLInternalSecretName = cr.Name + "-ssl-internal"
		}

		// pxc replicas shouldn't be less than 3 for safe configuration
		if c.PXC.Size < 3 && !c.AllowUnsafeConfig {
			c.PXC.Size = 3
		}

		// number of pxc replicas should be an odd
		if c.PXC.Size%2 == 0 && !c.AllowUnsafeConfig {
			c.PXC.Size++
		}

		// Set maxUnavailable = 1 by default for PodDisruptionBudget-PXC.
		// It's a description of the number of pods from that set that can be unavailable after the eviction.
		if c.PXC.PodDisruptionBudget == nil {
			defaultMaxUnavailable := intstr.FromInt(1)
			c.PXC.PodDisruptionBudget = &PodDisruptionBudgetSpec{MaxUnavailable: &defaultMaxUnavailable}
		}

		if c.PXC.TerminationGracePeriodSeconds == nil {
			c.PXC.TerminationGracePeriodSeconds = &defaultPXCGracePeriodSec
		}

		if c.PXC.LivenessInitialDelaySeconds == nil {
			c.PXC.LivenessInitialDelaySeconds = &livenessInitialDelaySeconds
		}

		c.PXC.reconcileAffinityOpts()

		if c.Pause {
			c.PXC.Size = 0
		}

		if c.PMM != nil && c.PMM.Resources == nil {
			c.PMM.Resources = c.PXC.Resources
		}
	}

	if c.ProxySQL != nil && c.ProxySQL.Enabled {
		if c.ProxySQL.VolumeSpec == nil {
			return false, fmt.Errorf("ProxySQL: volumeSpec should be specified")
		}
		changed, err = c.ProxySQL.VolumeSpec.reconcileOpts()
		if err != nil {
			return false, fmt.Errorf("ProxySQL.Volume: %v", err)
		}

		if len(c.SSLSecretName) > 0 {
			c.ProxySQL.SSLSecretName = c.SSLSecretName
		} else {
			c.ProxySQL.SSLSecretName = cr.Name + "-ssl"
		}

		if len(c.SSLInternalSecretName) > 0 {
			c.ProxySQL.SSLInternalSecretName = c.SSLInternalSecretName
		} else {
			c.ProxySQL.SSLInternalSecretName = cr.Name + "-ssl-internal"
		}

		// Set maxUnavailable = 1 by default for PodDisruptionBudget-ProxySQL.
		if c.ProxySQL.PodDisruptionBudget == nil {
			defaultMaxUnavailable := intstr.FromInt(1)
			c.ProxySQL.PodDisruptionBudget = &PodDisruptionBudgetSpec{MaxUnavailable: &defaultMaxUnavailable}
		}

		if c.ProxySQL.TerminationGracePeriodSeconds == nil {
			graceSec := int64(30)
			c.ProxySQL.TerminationGracePeriodSeconds = &graceSec
		}

		c.ProxySQL.reconcileAffinityOpts()

		if c.Pause {
			c.ProxySQL.Size = 0
		}
	}

	if c.Backup != nil {
		if c.Backup.Image == "" {
			return false, fmt.Errorf("backup.Image can't be empty")
		}

		for _, sch := range c.Backup.Schedule {
			strg, ok := cr.Spec.Backup.Storages[sch.StorageName]
			if !ok {
				return false, fmt.Errorf("storage %s doesn't exist", sch.StorageName)
			}
			switch strg.Type {
			case BackupStorageS3:
				//TODO what should we check here?
			case BackupStorageFilesystem:
				if strg.Volume == nil {
					return false, fmt.Errorf("backup storage %s: volume should be specified", sch.StorageName)
				}
				changed, err = strg.Volume.reconcileOpts()
				if err != nil {
					return false, fmt.Errorf("backup.Volume: %v", err)
				}
			}
		}
	}

	if cr.Spec.UpdateStrategy == SmartUpdateStatefulSetStrategyType && !cr.Spec.ProxySQL.Enabled {
		return false, fmt.Errorf("ProxySQL should be enabled if SmartUpdate set")
	}

	cr.setSecurityContext(serverVersion)

	return changed, nil
}

// setVersion sets the API version of a PXC resource.
// The new (semver-matching) version is determined either by the CR's API version or an API version specified via the CR's annotations.
// If the CR's API version is an empty string, it returns "v1"
func (cr *PerconaXtraDBCluster) setVersion() error {
	apiVersion := cr.APIVersion
	if lastCR, ok := cr.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		var newCR PerconaXtraDBCluster
		err := json.Unmarshal([]byte(lastCR), &newCR)
		if err != nil {
			return errors.Wrap(err, "unmarshal cr")
		}
		apiVersion = newCR.APIVersion
	}
	crVersion := strings.Replace(strings.TrimLeft(apiVersion, "pxc.percona.com/v"), "-", ".", -1)
	if len(crVersion) == 0 {
		crVersion = "v1"
	}
	version, err := v.NewVersion(crVersion)
	if err != nil {
		return errors.Wrap(err, "new version")
	}
	cr.version = version

	return nil
}

// CompareVersionWith compares given version to current version. Returns -1, 0, or 1 if given version is smaller, equal, or larger than the current version, respectively.
func (cr *PerconaXtraDBCluster) CompareVersionWith(version string) int {
	//using Must because "version" must be right format
	return cr.version.Compare(v.Must(v.NewVersion(version)))
}

const AffinityTopologyKeyOff = "none"

var affinityValidTopologyKeys = map[string]struct{}{
	AffinityTopologyKeyOff:                     {},
	"kubernetes.io/hostname":                   {},
	"failure-domain.beta.kubernetes.io/zone":   {},
	"failure-domain.beta.kubernetes.io/region": {},
}

var defaultAffinityTopologyKey = "kubernetes.io/hostname"

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

	case p.Affinity != nil && p.Affinity.TopologyKey != nil:
		if _, ok := affinityValidTopologyKeys[*p.Affinity.TopologyKey]; !ok {
			p.Affinity.TopologyKey = &defaultAffinityTopologyKey
		}
	}
}

func (v *VolumeSpec) reconcileOpts() (changed bool, err error) {
	if v.EmptyDir == nil && v.HostPath == nil && v.PersistentVolumeClaim == nil {
		v.PersistentVolumeClaim = &corev1.PersistentVolumeClaimSpec{}
	}

	if v.PersistentVolumeClaim != nil {
		_, ok := v.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			return changed, fmt.Errorf("volume.resources.storage can't be empty")
		}

		if v.PersistentVolumeClaim.AccessModes == nil || len(v.PersistentVolumeClaim.AccessModes) == 0 {
			v.PersistentVolumeClaim.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
			changed = true
		}
	}

	return changed, nil
}
