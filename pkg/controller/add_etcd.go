package controller

import (
	"github.com/lmdkfs/etcd-operator/pkg/controller/etcd"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, etcd.Add)
}
