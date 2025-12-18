/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package v1beta1 contains webhook implementations for ironic-operator v1beta1 API
package v1beta1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	ironicv1beta1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
)

// nolint:unused
// log is for logging in this package.
var ironicneutronagentlog = logf.Log.WithName("ironicneutronagent-resource")

// Static errors for webhook validation
var (
	errExpectedIronicNeutronAgentObject = errors.New("expected an IronicNeutronAgent object")
)

// SetupIronicNeutronAgentWebhookWithManager registers the webhook for IronicNeutronAgent in the manager.
func SetupIronicNeutronAgentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&ironicv1beta1.IronicNeutronAgent{}).
		WithDefaulter(&IronicNeutronAgentCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-ironic-openstack-org-v1beta1-ironicneutronagent,mutating=true,failurePolicy=fail,sideEffects=None,groups=ironic.openstack.org,resources=ironicneutronagents,verbs=create;update,versions=v1beta1,name=mironicneutronagent-v1beta1.kb.io,admissionReviewVersions=v1

// IronicNeutronAgentCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind IronicNeutronAgent when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type IronicNeutronAgentCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &IronicNeutronAgentCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind IronicNeutronAgent.
func (d *IronicNeutronAgentCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ironicneutronagent, ok := obj.(*ironicv1beta1.IronicNeutronAgent)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedIronicNeutronAgentObject, obj)
	}
	ironicneutronagentlog.Info("Defaulting for IronicNeutronAgent", "name", ironicneutronagent.GetName())

	// Call the defaulting logic from the API package
	ironicneutronagent.Spec.IronicNeutronAgentTemplate.Default()

	return nil
}
