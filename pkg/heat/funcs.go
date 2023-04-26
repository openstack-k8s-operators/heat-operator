package heat

import "sigs.k8s.io/controller-runtime/pkg/client"

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
