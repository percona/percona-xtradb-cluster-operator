package v1

import (
	"encoding/json"
	"strings"

	"github.com/go-ini/ini"
	cmmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	"github.com/percona/percona-xtradb-cluster-operator/version"

	v "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// PerconaXtraDBClusterSpec defines the desired state of PerconaXtraDBCluster
type PerconaXtraDBClusterSpec struct {
	Platform               version.Platform                     `json:"platform,omitempty"`
	CRVersion              string                               `json:"crVersion,omitempty"`
	Pause                  bool                                 `json:"pause,omitempty"`
	SecretsName            string                               `json:"secretsName,omitempty"`
	VaultSecretName        string                               `json:"vaultSecretName,omitempty"`
	SSLSecretName          string                               `json:"sslSecretName,omitempty"`
	SSLInternalSecretName  string                               `json:"sslInternalSecretName,omitempty"`
	LogCollectorSecretName string                               `json:"logCollectorSecretName,omitempty"`
	TLS                    *TLSSpec                             `json:"tls,omitempty"`
	PXC                    *PXCSpec                             `json:"pxc,omitempty"`
	ProxySQL               *PodSpec                             `json:"proxysql,omitempty"`
	HAProxy                *PodSpec                             `json:"haproxy,omitempty"`
	PMM                    *PMMSpec                             `json:"pmm,omitempty"`
	LogCollector           *LogCollectorSpec                    `json:"logcollector,omitempty"`
	Backup                 *PXCScheduledBackup                  `json:"backup,omitempty"`
	UpdateStrategy         appsv1.StatefulSetUpdateStrategyType `json:"updateStrategy,omitempty"`
	UpgradeOptions         UpgradeOptions                       `json:"upgradeOptions,omitempty"`
	AllowUnsafeConfig      bool                                 `json:"allowUnsafeConfigurations,omitempty"`
	InitImage              string                               `json:"initImage,omitempty"`
	DisableHookValidation  bool                                 `json:"disableHookValidation,omitempty"`
}

type PXCSpec struct {
	AutoRecovery *bool `json:"autoRecovery,omitempty"`
	*PodSpec
}

type TLSSpec struct {
	SANs       []string                `json:"SANs,omitempty"`
	IssuerConf *cmmeta.ObjectReference `json:"issuerConf,omitempty"`
}

type UpgradeOptions struct {
	VersionServiceEndpoint string `json:"versionServiceEndpoint,omitempty"`
	Apply                  string `json:"apply,omitempty"`
	Schedule               string `json:"schedule,omitempty"`
}

const (
	SmartUpdateStatefulSetStrategyType appsv1.StatefulSetUpdateStrategyType = "SmartUpdate"
)

type PXCScheduledBackup struct {
	Image              string                        `json:"image,omitempty"`
	ImagePullSecrets   []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	ImagePullPolicy    corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	Schedule           []PXCScheduledBackupSchedule  `json:"schedule,omitempty"`
	Storages           map[string]*BackupStorageSpec `json:"storages,omitempty"`
	ServiceAccountName string                        `json:"serviceAccountName,omitempty"`
	Annotations        map[string]string             `json:"annotations,omitempty"`
	PITR               PITRSpec                      `json:"pitr,omitempty"`
}

type PITRSpec struct {
	Enabled            bool          `json:"enabled"`
	StorageName        string        `json:"storageName"`
	Resources          *PodResources `json:"resources,omitempty"`
	TimeBetweenUploads int64         `json:"timeBetweenUploads,omitempty"`
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
	HAProxy            AppStatus          `json:"haproxy,omitempty"`
	Backup             AppStatus          `json:"backup,omitempty"`
	PMM                AppStatus          `json:"pmm,omitempty"`
	LogCollector       AppStatus          `json:"logcollector,omitempty"`
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
	ClusterReady        ClusterConditionType = "Ready"
	ClusterInit                              = "Initializing"
	ClusterPXCReady                          = "PXCReady"
	ClusterProxyReady                        = "ProxySQLReady"
	ClusterHAProxyReady                      = "HAProxyReady"
	ClusterError                             = "Error"
)

type ClusterCondition struct {
	Status             ConditionStatus      `json:"status,omitempty"`
	Type               ClusterConditionType `json:"type,omitempty"`
	LastTransitionTime metav1.Time          `json:"lastTransitionTime,omitempty"`
	Reason             string               `json:"reason,omitempty"`
	Message            string               `json:"message,omitempty"`
}

type AppStatus struct {
	Size    int32    `json:"size,omitempty"`
	Ready   int32    `json:"ready,omitempty"`
	Status  AppState `json:"status,omitempty"`
	Message string   `json:"message,omitempty"`
	Version string   `json:"version,omitempty"`
	Image   string   `json:"image,omitempty"`
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

func (cr *PerconaXtraDBCluster) Validate() error {
	if len(cr.Name) > clusterNameMaxLen {
		return errors.Errorf("cluster name (%s) too long, must be no more than %d characters", cr.Name, clusterNameMaxLen)
	}

	c := cr.Spec

	if c.PXC == nil {
		return errors.Errorf("spec.pxc section is not specified. Please check %s cluster settings", cr.Name)
	}
	if c.PXC.AutoRecovery == nil {
		boolVar := true
		c.PXC.AutoRecovery = &boolVar
	}

	if c.PXC.Image == "" {
		return errors.New("pxc.Image can't be empty")
	}

	if c.PMM != nil && c.PMM.Enabled {
		if c.PMM.Image == "" {
			return errors.New("pmm.Image can't be empty")
		}
	}

	if c.PXC.VolumeSpec == nil {
		return errors.New("PXC: volumeSpec should be specified")
	}

	if err := c.PXC.VolumeSpec.validate(); err != nil {
		return errors.Wrap(err, "PXC: validate volume spec")
	}

	if c.HAProxy != nil && c.HAProxy.Enabled &&
		c.ProxySQL != nil && c.ProxySQL.Enabled {
		return errors.New("can't enable both HAProxy and ProxySQL please only select one of them")
	}

	if c.HAProxy != nil && c.HAProxy.Enabled {
		if c.HAProxy.Image == "" {
			return errors.New("haproxy.Image can't be empty")
		}
	}

	if c.ProxySQL != nil && c.ProxySQL.Enabled {
		if c.ProxySQL.Image == "" {
			return errors.New("proxysql.Image can't be empty")
		}
		if c.ProxySQL.VolumeSpec == nil {
			return errors.New("ProxySQL: volumeSpec should be specified")
		}

		if err := c.ProxySQL.VolumeSpec.validate(); err != nil {
			return errors.Wrap(err, "ProxySQL: validate volume spec")
		}
	}

	if c.Backup != nil {
		if c.Backup.Image == "" {
			return errors.New("backup.Image can't be empty")
		}
		if cr.Spec.Backup.PITR.Enabled {
			if len(cr.Spec.Backup.PITR.StorageName) == 0 {
				return errors.Errorf("backup.PITR.StorageName can't be empty")
			}
		}
		for _, sch := range c.Backup.Schedule {
			strg, ok := cr.Spec.Backup.Storages[sch.StorageName]
			if !ok {
				return errors.Errorf("storage %s doesn't exist", sch.StorageName)
			}
			if strg.Type == BackupStorageFilesystem {
				if strg.Volume == nil {
					return errors.Errorf("backup storage %s: volume should be specified", sch.StorageName)
				}

				if err := strg.Volume.validate(); err != nil {
					return errors.Wrap(err, "Backup: validate volume spec")
				}
			}
		}
	}

	if c.UpdateStrategy == SmartUpdateStatefulSetStrategyType &&
		(c.ProxySQL == nil || !c.ProxySQL.Enabled) &&
		(c.HAProxy == nil || !c.HAProxy.Enabled) {
		return errors.Errorf("ProxySQL or HAProxy should be enabled if SmartUpdate set")
	}

	return nil
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBClusterList contains a list of PerconaXtraDBCluster
type PerconaXtraDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PerconaXtraDBCluster `json:"items"`
}

type PodSpec struct {
	Enabled                       bool                                    `json:"enabled,omitempty"`
	Size                          int32                                   `json:"size,omitempty"`
	Image                         string                                  `json:"image,omitempty"`
	Resources                     *PodResources                           `json:"resources,omitempty"`
	SidecarResources              *PodResources                           `json:"sidecarResources,omitempty"`
	VolumeSpec                    *VolumeSpec                             `json:"volumeSpec,omitempty"`
	Affinity                      *PodAffinity                            `json:"affinity,omitempty"`
	NodeSelector                  map[string]string                       `json:"nodeSelector,omitempty"`
	Tolerations                   []corev1.Toleration                     `json:"tolerations,omitempty"`
	PriorityClassName             string                                  `json:"priorityClassName,omitempty"`
	Annotations                   map[string]string                       `json:"annotations,omitempty"`
	Labels                        map[string]string                       `json:"labels,omitempty"`
	ImagePullSecrets              []corev1.LocalObjectReference           `json:"imagePullSecrets,omitempty"`
	Configuration                 string                                  `json:"configuration,omitempty"`
	PodDisruptionBudget           *PodDisruptionBudgetSpec                `json:"podDisruptionBudget,omitempty"`
	VaultSecretName               string                                  `json:"vaultSecretName,omitempty"`
	SSLSecretName                 string                                  `json:"sslSecretName,omitempty"`
	SSLInternalSecretName         string                                  `json:"sslInternalSecretName,omitempty"`
	TerminationGracePeriodSeconds *int64                                  `json:"gracePeriod,omitempty"`
	ForceUnsafeBootstrap          bool                                    `json:"forceUnsafeBootstrap,omitempty"`
	ServiceType                   corev1.ServiceType                      `json:"serviceType,omitempty"`
	ReplicasServiceType           corev1.ServiceType                      `json:"replicasServiceType,omitempty"`
	ExternalTrafficPolicy         corev1.ServiceExternalTrafficPolicyType `json:"externalTrafficPolicy,omitempty"`
	ReplicasExternalTrafficPolicy corev1.ServiceExternalTrafficPolicyType `json:"replicasExternalTrafficPolicy,omitempty"`
	LoadBalancerSourceRanges      []string                                `json:"loadBalancerSourceRanges,omitempty"`
	ServiceAnnotations            map[string]string                       `json:"serviceAnnotations,omitempty"`
	SchedulerName                 string                                  `json:"schedulerName,omitempty"`
	ReadinessInitialDelaySeconds  *int32                                  `json:"readinessDelaySec,omitempty"`
	LivenessInitialDelaySeconds   *int32                                  `json:"livenessDelaySec,omitempty"`
	PodSecurityContext            *corev1.PodSecurityContext              `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext      *corev1.SecurityContext                 `json:"containerSecurityContext,omitempty"`
	ServiceAccountName            string                                  `json:"serviceAccountName,omitempty"`
	ImagePullPolicy               corev1.PullPolicy                       `json:"imagePullPolicy,omitempty"`
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

type LogCollectorSpec struct {
	Enabled                  bool                    `json:"enabled,omitempty"`
	Image                    string                  `json:"image,omitempty"`
	Resources                *PodResources           `json:"resources,omitempty"`
	Configuration            string                  `json:"configuration,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`
	ImagePullPolicy          corev1.PullPolicy       `json:"imagePullPolicy,omitempty"`
}

type PMMSpec struct {
	Enabled                  bool                    `json:"enabled,omitempty"`
	ServerHost               string                  `json:"serverHost,omitempty"`
	Image                    string                  `json:"image,omitempty"`
	ServerUser               string                  `json:"serverUser,omitempty"`
	PxcParams                string                  `json:"pxcParams,omitempty"`
	ProxysqlParams           string                  `json:"proxysqlParams,omitempty"`
	Resources                *PodResources           `json:"resources,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`
	ImagePullPolicy          corev1.PullPolicy       `json:"imagePullPolicy,omitempty"`
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

const WorkloadSA = "default"

type App interface {
	AppContainer(spec *PodSpec, secrets string, cr *PerconaXtraDBCluster) (corev1.Container, error)
	SidecarContainers(spec *PodSpec, secrets string, cr *PerconaXtraDBCluster) ([]corev1.Container, error)
	PMMContainer(spec *PMMSpec, secrets string, cr *PerconaXtraDBCluster) (*corev1.Container, error)
	LogCollectorContainer(spec *LogCollectorSpec, logPsecrets string, logRsecrets string, cr *PerconaXtraDBCluster) ([]corev1.Container, error)
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

func (cr *PerconaXtraDBCluster) setSecurityContext() {
	var fsgroup *int64
	if cr.Spec.Platform != version.PlatformOpenshift {
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

func (cr *PerconaXtraDBCluster) ShouldWaitForTokenIssue() bool {
	_, ok := cr.Annotations["percona.com/issue-vault-token"]
	return ok
}

// CheckNSetDefaults sets defaults options and overwrites wrong settings
// and checks if other options' values are allowable
// returned "changed" means CR should be updated on cluster
func (cr *PerconaXtraDBCluster) CheckNSetDefaults(serverVersion *version.ServerVersion) (changed bool, err error) {
	workloadSA := "percona-xtradb-cluster-operator-workload"
	if cr.CompareVersionWith("1.6.0") >= 0 {
		workloadSA = WorkloadSA
	}

	CRVerChanged, err := cr.setVersion()
	if err != nil {
		return false, errors.Wrap(err, "set version")
	}

	err = cr.Validate()
	if err != nil {
		return false, errors.Wrap(err, "validate cr")
	}

	c := &cr.Spec

	if c.PXC != nil {
		changed = c.PXC.VolumeSpec.reconcileOpts()

		if len(c.PXC.ImagePullPolicy) == 0 {
			c.PXC.ImagePullPolicy = corev1.PullAlways
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

		if len(c.PXC.ServiceAccountName) == 0 {
			c.PXC.ServiceAccountName = workloadSA
		}

		c.PXC.reconcileAffinityOpts()

		if c.Pause {
			c.PXC.Size = 0
		}

		if c.PMM != nil && c.PMM.Resources == nil {
			c.PMM.Resources = c.PXC.Resources
		}

		if c.LogCollector != nil && c.LogCollector.Resources == nil {
			c.LogCollector.Resources = c.PXC.Resources
		}

		if len(c.LogCollectorSecretName) == 0 {
			c.LogCollectorSecretName = cr.Name + "-log-collector"
		}
	}

	if c.PMM != nil && c.PMM.Enabled {
		if len(c.PMM.ImagePullPolicy) == 0 {
			c.PMM.ImagePullPolicy = corev1.PullAlways
		}
	}

	if c.LogCollector != nil && c.LogCollector.Enabled {
		if len(c.LogCollector.ImagePullPolicy) == 0 {
			c.LogCollector.ImagePullPolicy = corev1.PullAlways
		}
	}

	if c.HAProxy != nil && c.HAProxy.Enabled {
		if len(c.HAProxy.ImagePullPolicy) == 0 {
			c.HAProxy.ImagePullPolicy = corev1.PullAlways
		}

		// Set maxUnavailable = 1 by default for PodDisruptionBudget-HAProxy.
		if c.HAProxy.PodDisruptionBudget == nil {
			defaultMaxUnavailable := intstr.FromInt(1)
			c.HAProxy.PodDisruptionBudget = &PodDisruptionBudgetSpec{MaxUnavailable: &defaultMaxUnavailable}
		}

		if c.HAProxy.TerminationGracePeriodSeconds == nil {
			graceSec := int64(30)
			c.HAProxy.TerminationGracePeriodSeconds = &graceSec
		}

		if len(c.HAProxy.ServiceAccountName) == 0 {
			c.HAProxy.ServiceAccountName = workloadSA
		}

		c.HAProxy.reconcileAffinityOpts()

		if c.Pause {
			c.HAProxy.Size = 0
		}
	}

	if c.ProxySQL != nil && c.ProxySQL.Enabled {
		if len(c.ProxySQL.ImagePullPolicy) == 0 {
			c.ProxySQL.ImagePullPolicy = corev1.PullAlways
		}

		changed = c.ProxySQL.VolumeSpec.reconcileOpts()

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

		if len(c.ProxySQL.ServiceAccountName) == 0 {
			c.ProxySQL.ServiceAccountName = workloadSA
		}

		c.ProxySQL.reconcileAffinityOpts()

		if c.Pause {
			c.ProxySQL.Size = 0
		}
	}

	if c.Backup != nil {

		if len(c.Backup.ImagePullPolicy) == 0 {
			c.Backup.ImagePullPolicy = corev1.PullAlways
		}
		if cr.Spec.Backup.PITR.Enabled {
			if cr.Spec.Backup.PITR.TimeBetweenUploads == 0 {
				cr.Spec.Backup.PITR.TimeBetweenUploads = 60
			}
		}

		for _, sch := range c.Backup.Schedule {
			strg := c.Backup.Storages[sch.StorageName]
			switch strg.Type {
			case BackupStorageS3:
				//TODO what should we check here?
			case BackupStorageFilesystem:
				changed = strg.Volume.reconcileOpts()
			}
		}
	}

	if len(c.Platform) == 0 {
		if len(serverVersion.Platform) > 0 {
			c.Platform = serverVersion.Platform
		} else {
			c.Platform = version.PlatformKubernetes
		}
	}

	cr.setSecurityContext()

	return CRVerChanged || changed, nil
}

// setVersion sets the API version of a PXC resource.
// The new (semver-matching) version is determined either by the CR's API version or an API version specified via the CR's fields.
// If the CR's API version is an empty string and last-applied-configuration from k8s is empty, it returns current operator version.
func (cr *PerconaXtraDBCluster) setVersion() (bool, error) {
	if len(cr.Spec.CRVersion) > 0 {
		return false, nil
	}
	apiVersion := version.Version
	if lastCR, ok := cr.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		var newCR PerconaXtraDBCluster
		err := json.Unmarshal([]byte(lastCR), &newCR)
		if err != nil {
			return false, errors.Wrap(err, "unmarshal cr")
		}
		if len(newCR.APIVersion) > 0 {
			apiVersion = strings.Replace(strings.TrimPrefix(newCR.APIVersion, "pxc.percona.com/v"), "-", ".", -1)
		}
	}

	cr.Spec.CRVersion = apiVersion
	return true, nil
}

func (cr *PerconaXtraDBCluster) Version() *v.Version {
	return v.Must(v.NewVersion(cr.Spec.CRVersion))
}

// CompareVersionWith compares given version to current version. Returns -1, 0, or 1 if given version is smaller, equal, or larger than the current version, respectively.
func (cr *PerconaXtraDBCluster) CompareVersionWith(version string) int {
	if len(cr.Spec.CRVersion) == 0 {
		cr.setVersion()
	}

	//using Must because "version" must be right format
	return cr.Version().Compare(v.Must(v.NewVersion(version)))
}

// ConfigHasKey check if cr.Spec.PXC.Configuration has given key in given section
func (cr *PerconaXtraDBCluster) ConfigHasKey(section, key string) (bool, error) {
	file, err := ini.LoadSources(ini.LoadOptions{AllowBooleanKeys: true}, []byte(cr.Spec.PXC.Configuration))
	if err != nil {
		return false, errors.Wrap(err, "load configuration")
	}
	s, err := file.GetSection(section)
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "get section")
	}

	return s.HasKey(key), nil
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

func (v *VolumeSpec) reconcileOpts() (changed bool) {
	if v.EmptyDir == nil && v.HostPath == nil && v.PersistentVolumeClaim == nil {
		v.PersistentVolumeClaim = &corev1.PersistentVolumeClaimSpec{}
	}

	if v.PersistentVolumeClaim != nil {
		if len(v.PersistentVolumeClaim.AccessModes) == 0 {
			v.PersistentVolumeClaim.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
			changed = true
		}
	}

	return changed
}

func (v *VolumeSpec) validate() error {
	if v.EmptyDir == nil && v.HostPath == nil && v.PersistentVolumeClaim == nil {
		v.PersistentVolumeClaim = &corev1.PersistentVolumeClaimSpec{}
	}

	if v.PersistentVolumeClaim != nil {
		_, ok := v.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			return errors.New("volume.resources.storage can't be empty")
		}
	}
	return nil
}
