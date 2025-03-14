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

func GetHeatDBSecurityContext() *corev1.SecurityContext {
	trueVal := true
	falseVal := false
	runAsUser := int64(HeatUID)
	runAsGroup := int64(HeatGID)
	return &corev1.SecurityContext{
		RunAsUser:                &runAsUser,
		RunAsGroup:               &runAsGroup,
		RunAsNonRoot:             &trueVal,
		AllowPrivilegeEscalation: &falseVal,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
	}
}

func GetHeatSecurityContext() *corev1.SecurityContext {
	var runAsUser = HeatUID
	var runAsGroup = HeatGID

	return &corev1.SecurityContext{
		RunAsUser:  &runAsUser,
		RunAsGroup: &runAsGroup,
	}
}

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
