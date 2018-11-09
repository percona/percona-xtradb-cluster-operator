package pxc

import (
	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

const appName = "pxc"

type PXC struct {
	serverVersion api.ServerVersion
}

func New(sv api.ServerVersion) *PXC {
	return &PXC{
		serverVersion: sv,
	}
}
