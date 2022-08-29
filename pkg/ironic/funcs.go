package ironic

import "sigs.k8s.io/controller-runtime/pkg/client"

// GetOwningIronicName - Given a IronicAPI, IronicConductor
// object, returning the parent Ironic object that created it (if any)
func GetOwningIronicName(instance client.Object) string {
	for _, ownerRef := range instance.GetOwnerReferences() {
		if ownerRef.Kind == "Ironic" {
			return ownerRef.Name
		}
	}

	return ""
}
