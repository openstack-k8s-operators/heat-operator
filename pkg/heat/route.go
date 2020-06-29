package heat

import (
	routev1 "github.com/openshift/api/route/v1"
	comv1 "github.com/openstack-k8s-operators/heat-operator/pkg/apis/heat/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// Route func
func Route(cr *comv1.Heat, cmName string) *routev1.Route {

	labels := map[string]string{
		"app": "heat-api",
	}
	serviceRef := routev1.RouteTargetReference{
		Kind: "Service",
		Name: "heat",
	}
	routePort := &routev1.RoutePort{
		TargetPort: intstr.FromInt(8004),
	}
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: routev1.RouteSpec{
			To:   serviceRef,
			Port: routePort,
		},
	}
	return route
}
