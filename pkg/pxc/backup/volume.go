package backup

// import (
// 	"fmt"
// 	"time"

// 	"github.com/operator-framework/operator-sdk/pkg/sdk"
// 	corev1 "k8s.io/api/core/v1"
// 	"k8s.io/apimachinery/pkg/api/errors"
// 	"k8s.io/apimachinery/pkg/api/resource"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// 	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
// )

// const volumeNamePostfix = "-backup"

// // PVC is a PersistentVolumeClaim for backup
// type PVC struct {
// 	obj *corev1.PersistentVolumeClaim
// }

// // NewPVC returns the list of PersistentVolumeClaims for the backups
// func NewPVC(cr *api.PerconaXtraDBBackup) *PVC {
// 	pvc := &corev1.PersistentVolumeClaim{
// 		TypeMeta: metav1.TypeMeta{
// 			APIVersion: "v1",
// 			Kind:       "PersistentVolumeClaim",
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      cr.Spec.PXCCluster + volumeNamePostfix + "." + cr.Name,
// 			Namespace: cr.Namespace,
// 		},
// 	}

// 	pvc.SetOwnerReferences(append(pvc.GetOwnerReferences(), cr.OwnerRef()))

// 	return &PVC{obj: pvc}
// }

// // Name returns volume name
// func (p *PVC) Name() string {
// 	return p.obj.ObjectMeta.Name
// }

// type VolumeStatus string

// const (
// 	VolumeNotExists VolumeStatus = "NotExists"
// 	VolumeBound                  = VolumeStatus(corev1.ClaimBound)
// 	VolumePending                = VolumeStatus(corev1.ClaimPending)
// 	VolumeLost                   = VolumeStatus(corev1.ClaimLost)
// )

// // Status returns the volume status
// func (p *PVC) Status() (VolumeStatus, error) {
// 	err := sdk.Get(p.obj)
// 	if err != nil {
// 		return VolumeNotExists, err
// 	}

// 	return VolumeStatus(p.obj.Status.Phase), nil
// }

// // Create creates PVC object via sdk
// func (p *PVC) Create(spec api.PXCBackupVolume) (VolumeStatus, error) {
// 	status := VolumeNotExists

// 	rvolStorage, err := resource.ParseQuantity(spec.Size)
// 	if err != nil {
// 		return status, fmt.Errorf("wrong storage resources: %v", err)
// 	}

// 	p.obj.Spec = corev1.PersistentVolumeClaimSpec{
// 		StorageClassName: spec.StorageClass,
// 		AccessModes: []corev1.PersistentVolumeAccessMode{
// 			corev1.ReadWriteOnce,
// 		},
// 		Resources: corev1.ResourceRequirements{
// 			Requests: corev1.ResourceList{
// 				corev1.ResourceStorage: rvolStorage,
// 			},
// 		},
// 	}

// 	err = sdk.Create(p.obj)
// 	if err != nil && !errors.IsAlreadyExists(err) {
// 		return status, fmt.Errorf("sdk create: %v", err)
// 	}

// 	for i := time.Duration(1); i <= 5; i++ {
// 		status, err = p.Status()
// 		if err != nil {
// 			return status, fmt.Errorf("get status: %v", err)
// 		}

// 		if status != VolumePending {
// 			return status, nil
// 		}

// 		time.Sleep(time.Second * i)
// 	}

// 	return status, nil
// }
