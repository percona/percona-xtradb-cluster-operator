package controller

import (
	"github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/controller/perconaxtradbbackup"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, perconaxtradbbackup.Add)
}
