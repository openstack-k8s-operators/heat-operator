package controller

import (
	"github.com/openstack-k8s-operators/heat-operator/pkg/controller/heat"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, heat.Add)
}
