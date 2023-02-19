/*
Copyright 2023 Red Hat Inc.

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

package v1beta1

import (
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var ironiclog = logf.Log.WithName("ironic-resource")

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *Ironic) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-ironic-openstack-org-v1beta1-ironic,mutating=false,failurePolicy=fail,sideEffects=None,groups=ironic.openstack.org,resources=ironics,verbs=create;update,versions=v1beta1,name=vironic.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Ironic{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Ironic) ValidateCreate() error {
	ironiclog.Info("validate create", "name", r.Name)
	var allErrs field.ErrorList

	if err := r.validateConductoGroupsUnique(); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) != 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "ironic.openstack.org", Kind: "Ironic"},
			r.Name, allErrs)
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Ironic) ValidateUpdate(old runtime.Object) error {
	ironiclog.Info("validate update", "name", r.Name)
	var allErrs field.ErrorList

	if err := r.validateConductoGroupsUnique(); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) != 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "ironic.openstack.org", Kind: "Ironic"},
			r.Name, allErrs)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Ironic) ValidateDelete() error {
	ironiclog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

// validateConductoGroupsUnique implements validation of IronicConductor ConductorGroup
func (r *Ironic) validateConductoGroupsUnique() *field.Error {
	fieldPath := field.NewPath("spec").Child("ironicConductors")
	var groupName string
	seenGrps := make(map[string]int)
	dupes := make(map[int]string)

	for groupIdx, c := range r.Spec.IronicConductors {
		if c.ConductorGroup == "" {
			groupName = "null_conductor_group_null"
		} else {
			groupName = c.ConductorGroup
		}

		if seenIdx, found := seenGrps[groupName]; found {
			dupes[groupIdx] = fmt.Sprintf(
				"#%d: \"%s\" duplicate of #%d: \"%s\"",
				seenIdx, c.ConductorGroup, groupIdx, c.ConductorGroup)
			continue
		}

		seenGrps[groupName] = groupIdx
	}

	if len(dupes) != 0 {
		eStrings := make([]string, 0, len(dupes))
		for _, eStr := range dupes {
			eStrings = append(eStrings, eStr)
		}
		err := fmt.Sprintf(
			"ConductorGroup must be unique: %s", strings.Join(eStrings, ", "))

		return field.Invalid(fieldPath, r.Spec.IronicConductors, err)
	}

	return nil
}
