package backup

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

type Backup struct {
	cluster            string
	namespace          string
	image              string
	imagePullSecrets   []corev1.LocalObjectReference
	imagePullPolicy    corev1.PullPolicy
	serviceAccountName string
}

func New(cr *api.PerconaXtraDBCluster) *Backup {
	return &Backup{
		cluster:            cr.Name,
		namespace:          cr.Namespace,
		image:              cr.Spec.Backup.Image,
		imagePullSecrets:   cr.Spec.Backup.ImagePullSecrets,
		imagePullPolicy:    cr.Spec.Backup.ImagePullPolicy,
		serviceAccountName: cr.Spec.Backup.ServiceAccountName,
	}
}
