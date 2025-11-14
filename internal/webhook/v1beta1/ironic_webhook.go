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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ironicv1beta1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
)

// nolint:unused
// log is for logging in this package.
var ironiclog = logf.Log.WithName("ironic-resource")

// Static errors for webhook validation
var (
	errExpectedIronicObject = errors.New("expected an Ironic object")
)

// SetupIronicWebhookWithManager registers the webhook for Ironic in the manager.
func SetupIronicWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&ironicv1beta1.Ironic{}).
		WithValidator(&IronicCustomValidator{}).
		WithDefaulter(&IronicCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-ironic-openstack-org-v1beta1-ironic,mutating=true,failurePolicy=fail,sideEffects=None,groups=ironic.openstack.org,resources=ironics,verbs=create;update,versions=v1beta1,name=mironic-v1beta1.kb.io,admissionReviewVersions=v1

// IronicCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Ironic when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type IronicCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &IronicCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Ironic.
func (d *IronicCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ironic, ok := obj.(*ironicv1beta1.Ironic)

	if !ok {
		return fmt.Errorf("%w but got %T", errExpectedIronicObject, obj)
	}
	ironiclog.Info("Defaulting for Ironic", "name", ironic.GetName())

	// Call the defaulting logic from the API package
	ironic.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-ironic-openstack-org-v1beta1-ironic,mutating=false,failurePolicy=fail,sideEffects=None,groups=ironic.openstack.org,resources=ironics,verbs=create;update,versions=v1beta1,name=vironic-v1beta1.kb.io,admissionReviewVersions=v1

// IronicCustomValidator struct is responsible for validating the Ironic resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type IronicCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &IronicCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Ironic.
func (v *IronicCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ironic, ok := obj.(*ironicv1beta1.Ironic)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedIronicObject, obj)
	}
	ironiclog.Info("Validation for Ironic upon creation", "name", ironic.GetName())

	// Call the validation logic from the API package
	return ironic.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Ironic.
func (v *IronicCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	ironic, ok := newObj.(*ironicv1beta1.Ironic)
	if !ok {
		return nil, fmt.Errorf("%w for the newObj but got %T", errExpectedIronicObject, newObj)
	}
	ironiclog.Info("Validation for Ironic upon update", "name", ironic.GetName())

	// Call the validation logic from the API package
	return ironic.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Ironic.
func (v *IronicCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ironic, ok := obj.(*ironicv1beta1.Ironic)
	if !ok {
		return nil, fmt.Errorf("%w but got %T", errExpectedIronicObject, obj)
	}
	ironiclog.Info("Validation for Ironic upon deletion", "name", ironic.GetName())

	// Call the validation logic from the API package
	return ironic.ValidateDelete()
}
