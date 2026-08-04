// Package heat contains heat service functionality and configuration.
package heat

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// GetOwningHeatName - Given a HeatAPI, HeatCfnAPI, HeatEngine
// object, returning the parent Heat object that created it (if any)
func GetOwningHeatName(instance client.Object) string {
	for _, ownerRef := range instance.GetOwnerReferences() {
		if ownerRef.Kind == "Heat" {
			return ownerRef.Name
		}
	}

	return ""
}

// FormatProbes creates a probe configuration for the specified port
func FormatProbes(port int32) *corev1.Probe {

	return &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       5,
		InitialDelaySeconds: 5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: HealthCheckPath,
				Port: intstr.IntOrString{Type: intstr.Int, IntVal: port},
			},
		},
	}
}
