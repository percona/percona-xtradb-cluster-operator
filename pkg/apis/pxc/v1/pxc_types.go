// +kubebuilder:validation:Optional

package v1

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/go-ini/ini"
	"github.com/go-logr/logr"
	v "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/percona/percona-xtradb-cluster-operator/version"
)

// PerconaXtraDBClusterSpec defines the desired state of PerconaXtraDBCluster
type PerconaXtraDBClusterSpec struct {
	Platform                  version.Platform                     `json:"platform,omitempty"`
	CRVersion                 string                               `json:"crVersion,omitempty"`
	Pause                     bool                                 `json:"pause,omitempty"`
	SecretsName               string                               `json:"secretsName,omitempty"`
	VaultSecretName           string                               `json:"vaultSecretName,omitempty"`
	SSLSecretName             string                               `json:"sslSecretName,omitempty"`
	SSLInternalSecretName     string                               `json:"sslInternalSecretName,omitempty"`
	LogCollectorSecretName    string                               `json:"logCollectorSecretName,omitempty"`
	TLS                       *TLSSpec                             `json:"tls,omitempty"`
	PXC                       *PXCSpec                             `json:"pxc,omitempty"`
	ProxySQL                  *PodSpec                             `json:"proxysql,omitempty"`
	HAProxy                   *HAProxySpec                         `json:"haproxy,omitempty"`
	PMM                       *PMMSpec                             `json:"pmm,omitempty"`
	LogCollector              *LogCollectorSpec                    `json:"logcollector,omitempty"`
	Backup                    *PXCScheduledBackup                  `json:"backup,omitempty"`
	UpdateStrategy            appsv1.StatefulSetUpdateStrategyType `json:"updateStrategy,omitempty"`
	UpgradeOptions            UpgradeOptions                       `json:"upgradeOptions,omitempty"`
	AllowUnsafeConfig         bool                                 `json:"allowUnsafeConfigurations,omitempty"`
	InitImage                 string                               `json:"initImage,omitempty"`
	EnableCRValidationWebhook *bool                                `json:"enableCRValidationWebhook,omitempty"`
}

type PXCSpec struct {
	AutoRecovery        *bool                `json:"autoRecovery,omitempty"`
	ReplicationChannels []ReplicationChannel `json:"replicationChannels,omitempty"`
	Expose              ServiceExpose        `json:"expose,omitempty"`
	*PodSpec            `json:",inline"`
}

type ServiceExpose struct {
	Enabled                  bool                                    `json:"enabled,omitempty"`
	Type                     corev1.ServiceType                      `json:"type,omitempty"`
	LoadBalancerSourceRanges []string                                `json:"loadBalancerSourceRanges,omitempty"`
	Annotations              map[string]string                       `json:"annotations,omitempty"`
	TrafficPolicy            corev1.ServiceExternalTrafficPolicyType `json:"trafficPolicy,omitempty"`
}

type ReplicationChannel struct {
	Name        string                    `json:"name,omitempty"`
	IsSource    bool                      `json:"isSource,omitempty"`
	SourcesList []ReplicationSource       `json:"sourcesList,omitempty"`
	Config      *ReplicationChannelConfig `json:"configuration,omitempty"`
}

type ReplicationChannelConfig struct {
	SourceRetryCount   uint `json:"sourceRetryCount,omitempty"`
	SourceConnectRetry uint `json:"sourceConnectRetry,omitempty"`
}

type ReplicationSource struct {
	Host   string `json:"host,omitempty"`
	Port   int    `json:"port,omitempty"`
	Weight int    `json:"weight,omitempty"`
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
	BackoffLimit       *int32                        `json:"backoffLimit,omitempty"`
}

type PITRSpec struct {
	Enabled            bool                        `json:"enabled"`
	StorageName        string                      `json:"storageName"`
	Resources          corev1.ResourceRequirements `json:"resources,omitempty"`
	TimeBetweenUploads float64                     `json:"timeBetweenUploads,omitempty"`
}

type PXCScheduledBackupSchedule struct {
	Name        string `json:"name,omitempty"`
	Schedule    string `json:"schedule,omitempty"`
	Keep        int    `json:"keep,omitempty"`
	StorageName string `json:"storageName,omitempty"`
}
type AppState string

const (
	AppStateUnknown  AppState = "unknown"
	AppStateInit     AppState = "initializing"
	AppStatePaused   AppState = "paused"
	AppStateStopping AppState = "stopping"
	AppStateReady    AppState = "ready"
	AppStateError    AppState = "error"
)

// PerconaXtraDBClusterStatus defines the observed state of PerconaXtraDBCluster
type PerconaXtraDBClusterStatus struct {
	PXC                AppStatus          `json:"pxc,omitempty"`
	PXCReplication     *ReplicationStatus `json:"pxcReplication,omitempty"`
	ProxySQL           AppStatus          `json:"proxysql,omitempty"`
	HAProxy            AppStatus          `json:"haproxy,omitempty"`
	Backup             ComponentStatus    `json:"backup,omitempty"`
	PMM                ComponentStatus    `json:"pmm,omitempty"`
	LogCollector       ComponentStatus    `json:"logcollector,omitempty"`
	Host               string             `json:"host,omitempty"`
	Messages           []string           `json:"message,omitempty"`
	Status             AppState           `json:"state,omitempty"`
	Conditions         []ClusterCondition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Size               int32              `json:"size"`
	Ready              int32              `json:"ready"`
}

// TODO: add replication status(error,active and etc)
type ReplicationStatus struct {
	Channels []ReplicationChannelStatus `json:"replicationChannels,omitempty"`
}

type ReplicationChannelStatus struct {
	Name                     string `json:"name,omitempty"`
	ReplicationChannelConfig `json:",inline"`
}

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

type ClusterCondition struct {
	Status             ConditionStatus `json:"status,omitempty"`
	Type               AppState        `json:"type,omitempty"`
	LastTransitionTime metav1.Time     `json:"lastTransitionTime,omitempty"`
	Reason             string          `json:"reason,omitempty"`
	Message            string          `json:"message,omitempty"`
}

type ComponentStatus struct {
	Status            AppState `json:"status,omitempty"`
	Message           string   `json:"message,omitempty"`
	Version           string   `json:"version,omitempty"`
	Image             string   `json:"image,omitempty"`
	LabelSelectorPath string   `json:"labelSelectorPath,omitempty"`
}

type AppStatus struct {
	ComponentStatus `json:",inline"`

	Size  int32 `json:"size,omitempty"`
	Ready int32 `json:"ready,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PerconaXtraDBCluster is the Schema for the perconaxtradbclusters API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:pruning:PreserveUnknownFields
// +kubebuilder:resource:shortName="pxc";"pxcs"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.host"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="PXC",type="string",JSONPath=".status.pxc.ready",description="Ready pxc nodes"
// +kubebuilder:printcolumn:name="proxysql",type="string",JSONPath=".status.proxysql.ready",description="Ready proxysql nodes"
// +kubebuilder:printcolumn:name="haproxy",type="string",JSONPath=".status.haproxy.ready",description="Ready haproxy nodes"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
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

	if len(c.PXC.ReplicationChannels) > 0 {
		// since we do not allow multimaster
		// isSource field should be equal everywhere
		isSrc := c.PXC.ReplicationChannels[0].IsSource
		for _, channel := range c.PXC.ReplicationChannels {
			// this restrictions coming from mysql itself
			if len(channel.Name) > 64 || channel.Name == "" || channel.Name == "group_replication_applier" || channel.Name == "group_replication_recovery" {
				return errors.Errorf("invalid replication channel name %s, please see channel naming conventions", channel.Name)
			}

			if isSrc != channel.IsSource {
				return errors.New("you can specify only one type of replication please specify equal values for isSource field")
			}

			if channel.IsSource {
				continue
			}

			if len(channel.SourcesList) == 0 {
				return errors.Errorf("sources list for replication channel %s should be empty, because it's replica", channel.Name)
			}
		}
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
			_, ok := cr.Spec.Backup.Storages[cr.Spec.Backup.PITR.StorageName]
			if !ok {
				return errors.Errorf("pitr storage %s doesn't exist", cr.Spec.Backup.PITR.StorageName)
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

func (list *PerconaXtraDBClusterList) HasUnfinishedFinalizers() bool {
	for _, v := range list.Items {
		if v.ObjectMeta.DeletionTimestamp != nil && len(v.Finalizers) != 0 {
			return true
		}
	}

	return false
}

type PodSpec struct {
	Enabled                       bool                                    `json:"enabled,omitempty"`
	Size                          int32                                   `json:"size,omitempty"`
	Image                         string                                  `json:"image,omitempty"`
	Resources                     corev1.ResourceRequirements             `json:"resources,omitempty"`
	SidecarResources              corev1.ResourceRequirements             `json:"sidecarResources,omitempty"`
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
	EnvVarsSecretName             string                                  `json:"envVarsSecret,omitempty"`
	TerminationGracePeriodSeconds *int64                                  `json:"gracePeriod,omitempty"`
	ForceUnsafeBootstrap          bool                                    `json:"forceUnsafeBootstrap,omitempty"`
	ServiceType                   corev1.ServiceType                      `json:"serviceType,omitempty"`
	ReplicasServiceType           corev1.ServiceType                      `json:"replicasServiceType,omitempty"`
	ExternalTrafficPolicy         corev1.ServiceExternalTrafficPolicyType `json:"externalTrafficPolicy,omitempty"`
	ReplicasExternalTrafficPolicy corev1.ServiceExternalTrafficPolicyType `json:"replicasExternalTrafficPolicy,omitempty"`
	LoadBalancerSourceRanges      []string                                `json:"loadBalancerSourceRanges,omitempty"`
	ServiceAnnotations            map[string]string                       `json:"serviceAnnotations,omitempty"`
	ServiceLabels                 map[string]string                       `json:"serviceLabels,omitempty"`
	ReplicasServiceAnnotations    map[string]string                       `json:"replicasServiceAnnotations,omitempty"`
	ReplicasServiceLabels         map[string]string                       `json:"replicasServiceLabels,omitempty"`
	SchedulerName                 string                                  `json:"schedulerName,omitempty"`
	ReadinessInitialDelaySeconds  *int32                                  `json:"readinessDelaySec,omitempty"`
	ReadinessProbes               corev1.Probe                            `json:"readinessProbes,omitempty"`
	LivenessInitialDelaySeconds   *int32                                  `json:"livenessDelaySec,omitempty"`
	LivenessProbes                corev1.Probe                            `json:"livenessProbes,omitempty"`
	PodSecurityContext            *corev1.PodSecurityContext              `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext      *corev1.SecurityContext                 `json:"containerSecurityContext,omitempty"`
	ServiceAccountName            string                                  `json:"serviceAccountName,omitempty"`
	ImagePullPolicy               corev1.PullPolicy                       `json:"imagePullPolicy,omitempty"`
	Sidecars                      []corev1.Container                      `json:"sidecars,omitempty"`
	SidecarVolumes                []corev1.Volume                         `json:"sidecarVolumes,omitempty"`
	SidecarPVCs                   []corev1.PersistentVolumeClaim          `json:"sidecarPVCs,omitempty"`
	RuntimeClassName              *string                                 `json:"runtimeClassName,omitempty"`
	HookScript                    string                                  `json:"hookScript,omitempty"`
}

type HAProxySpec struct {
	PodSpec                `json:",inline"`
	ReplicasServiceEnabled *bool `json:"replicasServiceEnabled,omitempty"`
}

type PodDisruptionBudgetSpec struct {
	MinAvailable   *intstr.IntOrString `json:"minAvailable,omitempty"`
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

type PodAffinity struct {
	TopologyKey *string          `json:"antiAffinityTopologyKey,omitempty"`
	Advanced    *corev1.Affinity `json:"advanced,omitempty"`
}

type LogCollectorSpec struct {
	Enabled                  bool                        `json:"enabled,omitempty"`
	Image                    string                      `json:"image,omitempty"`
	Resources                corev1.ResourceRequirements `json:"resources,omitempty"`
	Configuration            string                      `json:"configuration,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext     `json:"containerSecurityContext,omitempty"`
	ImagePullPolicy          corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	RuntimeClassName         *string                     `json:"runtimeClassName,omitempty"`
	HookScript               string                      `json:"hookScript,omitempty"`
}

type PMMSpec struct {
	Enabled                  bool                        `json:"enabled,omitempty"`
	ServerHost               string                      `json:"serverHost,omitempty"`
	Image                    string                      `json:"image,omitempty"`
	ServerUser               string                      `json:"serverUser,omitempty"`
	PxcParams                string                      `json:"pxcParams,omitempty"`
	ProxysqlParams           string                      `json:"proxysqlParams,omitempty"`
	Resources                corev1.ResourceRequirements `json:"resources,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext     `json:"containerSecurityContext,omitempty"`
	ImagePullPolicy          corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	RuntimeClassName         *string                     `json:"runtimeClassName,omitempty"`
}

func (spec *PMMSpec) UseAPI(secret *corev1.Secret) bool {
	if _, ok := secret.Data["pmmserverkey"]; !ok {
		if _, ok := secret.Data["pmmserver"]; ok {
			return false
		}
	}
	return true
}

type BackupStorageSpec struct {
	Type                     BackupStorageType           `json:"type"`
	S3                       BackupStorageS3Spec         `json:"s3,omitempty"`
	Volume                   *VolumeSpec                 `json:"volume,omitempty"`
	NodeSelector             map[string]string           `json:"nodeSelector,omitempty"`
	Resources                corev1.ResourceRequirements `json:"resources,omitempty"`
	Affinity                 *corev1.Affinity            `json:"affinity,omitempty"`
	Tolerations              []corev1.Toleration         `json:"tolerations,omitempty"`
	Annotations              map[string]string           `json:"annotations,omitempty"`
	Labels                   map[string]string           `json:"labels,omitempty"`
	SchedulerName            string                      `json:"schedulerName,omitempty"`
	PriorityClassName        string                      `json:"priorityClassName,omitempty"`
	PodSecurityContext       *corev1.PodSecurityContext  `json:"podSecurityContext,omitempty"`
	ContainerSecurityContext *corev1.SecurityContext     `json:"containerSecurityContext,omitempty"`
	RuntimeClassName         *string                     `json:"runtimeClassName,omitempty"`
	VerifyTLS                *bool                       `json:"verifyTLS,omitempty"`
}

type BackupStorageType string

const (
	BackupStorageFilesystem BackupStorageType = "filesystem"
	BackupStorageS3         BackupStorageType = "s3"
)

const (
	FinalizerDeleteS3Backup string = "delete-s3-backup"
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

func ContainsVolume(vs []corev1.Volume, name string) bool {
	for _, v := range vs {
		if v.Name == name {
			return true
		}
	}
	return false
}

const WorkloadSA = "default"

type CustomVolumeGetter func(nsName, cvName, cmName string, useDefaultVolume bool) (corev1.Volume, error)

var NoCustomVolumeErr = errors.New("no custom volume found")

type App interface {
	AppContainer(spec *PodSpec, secrets string, cr *PerconaXtraDBCluster, availableVolumes []corev1.Volume) (corev1.Container, error)
	SidecarContainers(spec *PodSpec, secrets string, cr *PerconaXtraDBCluster) ([]corev1.Container, error)
	PMMContainer(spec *PMMSpec, secret *corev1.Secret, cr *PerconaXtraDBCluster) (*corev1.Container, error)
	LogCollectorContainer(spec *LogCollectorSpec, logPsecrets string, logRsecrets string, cr *PerconaXtraDBCluster) ([]corev1.Container, error)
	Volumes(podSpec *PodSpec, cr *PerconaXtraDBCluster, vg CustomVolumeGetter) (*Volume, error)
	Labels() map[string]string
}

type StatefulApp interface {
	App
	Name() string
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
func (cr *PerconaXtraDBCluster) CheckNSetDefaults(serverVersion *version.ServerVersion, logger logr.Logger) (err error) {
	workloadSA := "percona-xtradb-cluster-operator-workload"
	if cr.CompareVersionWith("1.6.0") >= 0 {
		workloadSA = WorkloadSA
	}

	_ = cr.SetVersion()
	err = cr.Validate()
	if err != nil {
		return errors.Wrap(err, "validate cr")
	}

	c := &cr.Spec

	if c.PXC != nil {
		c.PXC.VolumeSpec.reconcileOpts()

		if len(c.PXC.ImagePullPolicy) == 0 {
			c.PXC.ImagePullPolicy = corev1.PullAlways
		}

		c.PXC.VaultSecretName = c.VaultSecretName
		if len(c.PXC.VaultSecretName) == 0 {
			c.PXC.VaultSecretName = cr.Name + "-vault"
		}

		if len(c.SecretsName) == 0 {
			c.SecretsName = cr.Name + "-secrets"
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

		for chIdx, channel := range c.PXC.ReplicationChannels {
			for srcIdx, src := range channel.SourcesList {
				if src.Weight == 0 {
					c.PXC.ReplicationChannels[chIdx].SourcesList[srcIdx].Weight = 100
				}
				if src.Port == 0 {
					c.PXC.ReplicationChannels[chIdx].SourcesList[srcIdx].Port = 3306
				}
			}
			if !channel.IsSource && channel.Config == nil {
				c.PXC.ReplicationChannels[chIdx].Config = &ReplicationChannelConfig{
					SourceRetryCount:   3,
					SourceConnectRetry: 60,
				}
			}
		}

		setSafeDefaults(c, logger)

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

		if len(c.PXC.EnvVarsSecretName) == 0 {
			c.PXC.EnvVarsSecretName = cr.Name + "-env-vars-pxc"
		}

		c.PXC.reconcileAffinityOpts()

		if c.Pause {
			c.PXC.Size = 0
		}

		if cr.CompareVersionWith("1.10.0") < 0 {
			if c.PMM != nil && c.PMM.Resources.Size() == 0 {
				c.PMM.Resources = c.PXC.Resources
			}

			if c.LogCollector != nil && c.LogCollector.Resources.Size() == 0 {
				c.LogCollector.Resources = c.PXC.Resources
			}
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
		if c.HAProxy.ReplicasServiceEnabled == nil {
			t := true
			c.HAProxy.ReplicasServiceEnabled = &t
		}

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

		if len(c.HAProxy.EnvVarsSecretName) == 0 {
			c.HAProxy.EnvVarsSecretName = cr.Name + "-env-vars-haproxy"
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

		c.ProxySQL.VolumeSpec.reconcileOpts()

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

		if len(c.ProxySQL.EnvVarsSecretName) == 0 {
			c.ProxySQL.EnvVarsSecretName = cr.Name + "-env-vars-proxysql"
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
				// TODO what should we check here?
			case BackupStorageFilesystem:
				strg.Volume.reconcileOpts()
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

	cr.setProbesDefaults()
	cr.setSecurityContext()

	if cr.Spec.EnableCRValidationWebhook == nil {
		falseVal := false
		cr.Spec.EnableCRValidationWebhook = &falseVal
	}

	return nil
}

const (
	maxSafePXCSize   = 5
	minSafeProxySize = 2
)

func (cr *PerconaXtraDBCluster) setProbesDefaults() {
	if cr.Spec.PXC.LivenessInitialDelaySeconds != nil {
		cr.Spec.PXC.LivenessProbes.InitialDelaySeconds = *cr.Spec.PXC.LivenessInitialDelaySeconds
	} else if cr.Spec.PXC.LivenessProbes.InitialDelaySeconds == 0 {
		cr.Spec.PXC.LivenessProbes.InitialDelaySeconds = 300
	}

	if cr.Spec.PXC.LivenessProbes.TimeoutSeconds == 0 {
		cr.Spec.PXC.LivenessProbes.TimeoutSeconds = 5
	}

	cr.Spec.PXC.LivenessProbes.SuccessThreshold = 1

	if cr.Spec.PXC.ReadinessInitialDelaySeconds != nil {
		cr.Spec.PXC.ReadinessProbes.InitialDelaySeconds = *cr.Spec.PXC.ReadinessInitialDelaySeconds
	} else if cr.Spec.PXC.ReadinessProbes.InitialDelaySeconds == 0 {
		cr.Spec.PXC.ReadinessProbes.InitialDelaySeconds = 15
	}

	if cr.Spec.PXC.ReadinessProbes.PeriodSeconds == 0 {
		cr.Spec.PXC.ReadinessProbes.PeriodSeconds = 30
	}

	if cr.Spec.PXC.ReadinessProbes.FailureThreshold == 0 {
		cr.Spec.PXC.ReadinessProbes.FailureThreshold = 5
	}
	if cr.Spec.PXC.ReadinessProbes.TimeoutSeconds == 0 {
		cr.Spec.PXC.ReadinessProbes.TimeoutSeconds = 15
	}

	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled {
		if cr.Spec.HAProxy.ReadinessInitialDelaySeconds != nil {
			cr.Spec.HAProxy.ReadinessProbes.InitialDelaySeconds = *cr.Spec.HAProxy.ReadinessInitialDelaySeconds
		} else if cr.Spec.HAProxy.ReadinessProbes.InitialDelaySeconds == 0 {
			cr.Spec.HAProxy.ReadinessProbes.InitialDelaySeconds = 15
		}
		if cr.Spec.HAProxy.ReadinessProbes.PeriodSeconds == 0 {
			cr.Spec.HAProxy.ReadinessProbes.PeriodSeconds = 5
		}

		if cr.Spec.HAProxy.ReadinessProbes.TimeoutSeconds == 0 {
			cr.Spec.HAProxy.ReadinessProbes.TimeoutSeconds = 1
		}

		if cr.Spec.HAProxy.LivenessInitialDelaySeconds != nil {
			cr.Spec.HAProxy.LivenessProbes.InitialDelaySeconds = *cr.Spec.HAProxy.LivenessInitialDelaySeconds
		} else if cr.Spec.HAProxy.LivenessProbes.InitialDelaySeconds == 0 {
			cr.Spec.HAProxy.LivenessProbes.InitialDelaySeconds = 60
		}

		if cr.Spec.HAProxy.LivenessProbes.TimeoutSeconds == 0 {
			cr.Spec.HAProxy.LivenessProbes.TimeoutSeconds = 5
		}
		if cr.Spec.HAProxy.LivenessProbes.FailureThreshold == 0 {
			cr.Spec.HAProxy.LivenessProbes.FailureThreshold = 4
		}
		if cr.Spec.HAProxy.LivenessProbes.PeriodSeconds == 0 {
			cr.Spec.HAProxy.LivenessProbes.PeriodSeconds = 30
		}

		cr.Spec.HAProxy.LivenessProbes.SuccessThreshold = 1

	}
}

func setSafeDefaults(spec *PerconaXtraDBClusterSpec, log logr.Logger) {
	if spec.AllowUnsafeConfig {
		return
	}

	loginfo := func(msg string, args ...interface{}) {
		log.Info(fmt.Sprintf(msg, args...))
		log.Info("Set allowUnsafeConfigurations=true to disable safe configuration")
	}

	if spec.PXC.Size < 3 {
		loginfo("Cluster size will be changed from %d to %d due to safe config", spec.PXC.Size, 3)
		spec.PXC.Size = 3
	} else if spec.PXC.Size > maxSafePXCSize {
		loginfo("Cluster size will be changed from %d to %d due to safe config", spec.PXC.Size, maxSafePXCSize)
		spec.PXC.Size = maxSafePXCSize
	}

	if spec.PXC.Size%2 == 0 {
		loginfo("Cluster size will be changed from %d to %d due to safe config", spec.PXC.Size, spec.PXC.Size+1)
		spec.PXC.Size++
	}

	if spec.ProxySQL != nil && spec.ProxySQL.Enabled {
		if spec.ProxySQL.Size < minSafeProxySize {
			loginfo("ProxySQL size will be changed from %d to %d due to safe config", spec.ProxySQL.Size, minSafeProxySize)
			spec.ProxySQL.Size = minSafeProxySize
		}
	}

	if spec.HAProxy != nil && spec.HAProxy.Enabled {
		if spec.HAProxy.Size < minSafeProxySize {
			loginfo("HAProxy size will be changed from %d to %d due to safe config", spec.HAProxy.Size, minSafeProxySize)
			spec.HAProxy.Size = minSafeProxySize
		}
	}
}

// SetVersion sets the API version of a PXC resource.
// The new (semver-matching) version is determined either by the CR's API version or an API version specified via the CR's fields.
// If the CR's API version is an empty string and last-applied-configuration from k8s is empty, it returns current operator version.
func (cr *PerconaXtraDBCluster) SetVersion() bool {
	if len(cr.Spec.CRVersion) > 0 {
		return false
	}

	apiVersion := version.Version

	if lastCR, ok := cr.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		var newCR PerconaXtraDBCluster
		err := json.Unmarshal([]byte(lastCR), &newCR)
		if err != nil {
			log.Printf("failed to unmarshal cr: %v", err)
		} else if len(newCR.APIVersion) > 0 {
			apiVersion = strings.Replace(strings.TrimPrefix(newCR.APIVersion, "pxc.percona.com/v"), "-", ".", -1)
		}
	}

	cr.Spec.CRVersion = apiVersion
	return true
}

func (cr *PerconaXtraDBCluster) Version() *v.Version {
	return v.Must(v.NewVersion(cr.Spec.CRVersion))
}

// CompareVersionWith compares given version to current version.
// Returns -1, 0, or 1 if given version is smaller, equal, or larger than the current version, respectively.
func (cr *PerconaXtraDBCluster) CompareVersionWith(ver string) int {
	if len(cr.Spec.CRVersion) == 0 {
		_ = cr.SetVersion()
	}

	return cr.Version().Compare(v.Must(v.NewVersion(ver)))
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
	"topology.kubernetes.io/zone":              {},
	"topology.kubernetes.io/region":            {},
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

func (v *VolumeSpec) reconcileOpts() {
	if v.EmptyDir == nil && v.HostPath == nil && v.PersistentVolumeClaim == nil {
		v.PersistentVolumeClaim = &corev1.PersistentVolumeClaimSpec{}
	}

	if v.PersistentVolumeClaim != nil {
		if len(v.PersistentVolumeClaim.AccessModes) == 0 {
			v.PersistentVolumeClaim.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		}
	}
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

func AddSidecarContainers(logger logr.Logger, existing, sidecars []corev1.Container) []corev1.Container {
	if len(sidecars) == 0 {
		return existing
	}

	names := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		names[c.Name] = struct{}{}
	}

	for _, c := range sidecars {
		if _, ok := names[c.Name]; ok {
			logger.Info(fmt.Sprintf("Sidecar container name cannot be %s. It's skipped", c.Name))
			continue
		}

		existing = append(existing, c)
	}

	return existing
}

func AddSidecarVolumes(logger logr.Logger, existing, sidecarVolumes []corev1.Volume) []corev1.Volume {
	if len(sidecarVolumes) == 0 {
		return existing
	}

	names := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		names[c.Name] = struct{}{}
	}

	for _, c := range sidecarVolumes {
		if _, ok := names[c.Name]; ok {
			logger.Info(fmt.Sprintf("Sidecar volume name cannot be %s. It's skipped", c.Name))
			continue
		}

		existing = append(existing, c)
	}

	return existing
}

func AddSidecarPVCs(logger logr.Logger, existing, sidecarPVCs []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	if len(sidecarPVCs) == 0 {
		return existing
	}

	names := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		names[c.Name] = struct{}{}
	}

	for _, c := range sidecarPVCs {
		if _, ok := names[c.Name]; ok {
			logger.Info(fmt.Sprintf("Sidecar PVC name cannot be %s. It's skipped", c.Name))
			continue
		}

		existing = append(existing, c)
	}

	return existing
}

func (cr *PerconaXtraDBCluster) ProxySQLUnreadyServiceNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-proxysql-unready",
		Namespace: cr.Namespace,
	}
}

func (cr *PerconaXtraDBCluster) ProxySQLServiceNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-proxysql",
		Namespace: cr.Namespace,
	}
}

func (cr *PerconaXtraDBCluster) HaproxyServiceNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-haproxy",
		Namespace: cr.Namespace,
	}
}

func (cr *PerconaXtraDBCluster) HAProxyReplicasNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      cr.Name + "-haproxy-replicas",
		Namespace: cr.Namespace,
	}
}

func (cr *PerconaXtraDBCluster) HAProxyEnabled() bool {
	return cr.Spec.HAProxy != nil && cr.Spec.HAProxy.Enabled
}

func (cr *PerconaXtraDBCluster) HAProxyReplicasServiceEnabled() bool {
	return *cr.Spec.HAProxy.ReplicasServiceEnabled
}

func (cr *PerconaXtraDBCluster) ProxySQLEnabled() bool {
	return cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled
}

func (s *PerconaXtraDBClusterStatus) ClusterStatus(inProgress, deleted bool) AppState {
	switch {
	case deleted || s.PXC.Status == AppStateStopping || s.ProxySQL.Status == AppStateStopping || s.HAProxy.Status == AppStateStopping:
		return AppStateStopping
	case s.PXC.Status == AppStatePaused, !inProgress && s.PXC.Status == AppStateReady:
		if s.HAProxy.Status != "" && s.HAProxy.Status != s.PXC.Status {
			return s.HAProxy.Status
		}

		if s.ProxySQL.Status != "" && s.ProxySQL.Status != s.PXC.Status {
			return s.ProxySQL.Status
		}

		return s.PXC.Status
	default:
		return AppStateInit
	}
}

const maxStatusesQuantity = 20

func (s *PerconaXtraDBClusterStatus) AddCondition(c ClusterCondition) {
	if len(s.Conditions) == 0 {
		s.Conditions = append(s.Conditions, c)
		return
	}

	if s.Conditions[len(s.Conditions)-1].Type != c.Type {
		s.Conditions = append(s.Conditions, c)
	}

	if len(s.Conditions) > maxStatusesQuantity {
		s.Conditions = s.Conditions[len(s.Conditions)-maxStatusesQuantity:]
	}
}

func (cr *PerconaXtraDBCluster) CanBackup() error {
	if cr.Status.Status == AppStateReady {
		return nil
	}

	if !cr.Spec.AllowUnsafeConfig {
		return errors.Errorf("allowUnsafeConfigurations must be true to run backup on cluster with status %s", cr.Status.Status)
	}

	if cr.Status.PXC.Ready < int32(1) {
		return errors.New("there are no ready PXC nodes")
	}

	return nil
}
