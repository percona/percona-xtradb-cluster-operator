package backup

import (
	corev1 "k8s.io/api/core/v1"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

type Backup struct {
	cluster          string
	namespace        string
	image            string
	imagePullSecrets []corev1.LocalObjectReference
}

func New(cr *api.PerconaXtraDBCluster, spec *api.PXCScheduledBackup) *Backup {
	return &Backup{
		cluster:          cr.Name,
		namespace:        cr.Namespace,
		image:            spec.Image,
		imagePullSecrets: spec.ImagePullSecrets,
	}
}
