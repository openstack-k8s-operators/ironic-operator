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
var ironicinspectorlog = logf.Log.WithName("ironicinspector-resource")

// Static errors for webhook validation
var (
	errExpectedIronicInspectorObject = errors.New("expected an IronicInspector object")
)

// SetupIronicInspectorWebhookWithManager registers the webhook for IronicInspector in the manager.
func SetupIronicInspectorWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&ironicv1beta1.IronicInspector{}).
		WithDefaulter(&IronicInspectorCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-ironic-openstack-org-v1beta1-ironicinspector,mutating=true,failurePolicy=fail,sideEffects=None,groups=ironic.openstack.org,resources=ironicinspectors,verbs=create;update,versions=v1beta1,name=mironicinspector-v1beta1.kb.io,admissionReviewVersions=v1

// IronicInspectorCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind IronicInspector when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type IronicInspectorCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &IronicInspectorCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind IronicInspector.
func (d *IronicInspectorCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ironicinspector, ok := obj.(*ironicv1beta1.IronicInspector)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedIronicInspectorObject, obj)
	}
	ironicinspectorlog.Info("Defaulting for IronicInspector", "name", ironicinspector.GetName())

	// Call the defaulting logic from the API package
	ironicinspector.Spec.Default()

	return nil
}
