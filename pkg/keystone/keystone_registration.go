// Package keystone contains helpers to interact with Keystone-related CRs.
package keystone

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
)

// ServiceRegistration captures the inputs required to register a Keystone service and its endpoints.
type ServiceRegistration struct {
	Namespace          string
	ServiceType        string
	ServiceName        string
	ServiceDescription string
	ServiceUser        string
	Secret             string
	PasswordSelector   string
	Endpoints          map[string]string
	Labels             map[string]string
	TimeoutSeconds     int
}

// EnsureRegistration ensures KeystoneService and KeystoneEndpoint exist and mirrors their conditions.
func EnsureRegistration(
	ctx context.Context,
	h *helper.Helper,
	registration ServiceRegistration,
	conditions *condition.Conditions,
) (ctrl.Result, error) {
	ksSvcSpec := keystonev1.KeystoneServiceSpec{
		ServiceType:        registration.ServiceType,
		ServiceName:        registration.ServiceName,
		ServiceDescription: registration.ServiceDescription,
		Enabled:            true,
		ServiceUser:        registration.ServiceUser,
		Secret:             registration.Secret,
		PasswordSelector:   registration.PasswordSelector,
	}

	ksSvcObj := keystonev1.NewKeystoneService(ksSvcSpec, registration.Namespace, registration.Labels, time.Duration(registration.TimeoutSeconds))
	ctrlResult, err := ksSvcObj.CreateOrPatch(ctx, h)
	if err != nil {
		return ctrlResult, err
	}
	// mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
	// into a local condition with the type condition.KeystoneEndpointReadyCondition
	if c := ksSvcObj.GetConditions().Mirror(condition.KeystoneServiceReadyCondition); c != nil {
		conditions.Set(c)
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// register endpoints
	ksEndptSpec := keystonev1.KeystoneEndpointSpec{
		ServiceName: registration.ServiceName,
		Endpoints:   registration.Endpoints,
	}
	ksEndpt := keystonev1.NewKeystoneEndpoint(
		registration.ServiceName,
		registration.Namespace,
		ksEndptSpec,
		registration.Labels,
		time.Duration(registration.TimeoutSeconds),
	)
	ctrlResult, err = ksEndpt.CreateOrPatch(ctx, h)
	if err != nil {
		return ctrlResult, err
	}
	// mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
	// into a local condition with the type condition.KeystoneEndpointReadyCondition
	if c := ksEndpt.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition); c != nil {
		conditions.Set(c)
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	return ctrl.Result{}, nil
}
