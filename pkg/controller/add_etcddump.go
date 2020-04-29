package controller

import (
	"github.com/lmdkfs/etcd-operator/pkg/controller/etcddump"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, etcddump.Add)
}
