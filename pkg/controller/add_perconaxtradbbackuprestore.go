package controller

import (
	"github.com/percona/percona-xtradb-cluster-operator/pkg/controller/perconaxtradbbackuprestore"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, perconaxtradbbackuprestore.Add)
}
