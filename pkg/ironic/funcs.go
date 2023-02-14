package ironic

import (
	"context"
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

// GetIngressDomain - Get the Ingress Domain of cluster
func GetIngressDomain(
	ctx context.Context,
	helper *helper.Helper,
) (string, error) {
	Log := helper.GetLogger()

	ingress := &unstructured.Unstructured{}
	ingress.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "operator.openshift.io",
			Version: "v1",
			Kind:    "IngressController",
		},
	)
	err := helper.GetClient().Get(
		context.Background(),
		client.ObjectKey{
			Namespace: "openshift-ingress-operator",
			Name:      "default",
		},
		ingress,
	)
	if err != nil {
		return "", fmt.Errorf("unable to retrieve ingress domain %v", err)
	}
	ingressDomain := ""

	ingressStatus := ingress.UnstructuredContent()["status"]
	ingressStatusMap, ok := ingressStatus.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unable to retrieve ingress domain - wanted type map[string]interface{}; got %T", ingressStatus)
	}
	for k, v := range ingressStatusMap {
		if k == "domain" {
			ingressDomain = v.(string)
			// Break out of the loop, we got what we need
			break
		}
	}
	if ingressDomain != "" {
		Log.Info(fmt.Sprintf("Found ingress domain: %s", ingressDomain))
	} else {
		return "", fmt.Errorf("unable to retrieve ingress domain")
	}

	return ingressDomain, nil
}
