package backup

import (
	"fmt"

	"github.com/operator-framework/operator-sdk/pkg/sdk"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

const volumeNamePostfix = "-backup"

// PVC is a PersistentVolumeClaim for backup
type PVC struct {
	obj *corev1.PersistentVolumeClaim
}

// NewPVC returns the list of PersistentVolumeClaims for the backups
func NewPVC(cr *api.PerconaXtraDBBackup) *PVC {
	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.PXCCluster + volumeNamePostfix + "." + cr.Name,
			Namespace: cr.Namespace,
		},
	}

	pvc.SetOwnerReferences(append(pvc.GetOwnerReferences(), cr.OwnerRef()))

	return &PVC{obj: pvc}
}

// Name returns volume name
func (p *PVC) Name() string {
	return p.obj.ObjectMeta.Name
}

type VolumeStatus string

const (
	VolumeNotExists VolumeStatus = "NotExists"
	VolumeBound                  = corev1.ClaimBound
	VolumePending                = corev1.ClaimPending
	VolumeLost                   = corev1.ClaimLost
)

// Status returns the volume status
func (p *PVC) Status() VolumeStatus {

	return VolumeNotExists
}

// Create creates PVC object via sdk
func (p *PVC) Create(spec *api.PXCBackupSpec) error {
	rvolStorage, err := resource.ParseQuantity(spec.Storage)
	if err != nil {
		return fmt.Errorf("wrong storage resources: %v", err)
	}

	p.obj.Spec = corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: rvolStorage,
			},
		},
	}

	return sdk.Create(p.obj)
}
