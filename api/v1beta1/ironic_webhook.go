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
	"bytes"
	"fmt"
	"net"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	k8snet "k8s.io/utils/net"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var ironiclog = logf.Log.WithName("ironic-resource")

const (
	errNotIPAddr               = "not an IP address"
	errInvalidCidr             = "IP address prefix (CIDR) %s"
	errNotInCidr               = "address not in IP address prefix (CIDR) %s"
	errMixedAddressFamily      = "cannot mix IPv4 and IPv6"
	errInvalidRange            = "Start address: %s > End address %s"
	errForbiddenAddressOverlap = "%v overlap with %v: %v"
)

type netIPStartEnd struct {
	start net.IP       // Start address of DHCP Range
	end   net.IP       // End address of DHCP Range
	path  *field.Path  // Field path to DHCP Range in Ironic spec
}

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

	if err := r.validateConductorSpec(); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := r.validateInspectorSpec(); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := r.validateDHCPRangesOverlap(); err != nil {
		allErrs = append(allErrs, err...)
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

	if err := r.validateConductorSpec(); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := r.validateInspectorSpec(); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := r.validateDHCPRangesOverlap(); err != nil {
		allErrs = append(allErrs, err...)
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

func (r *Ironic) validateInspectorSpec() field.ErrorList {
	var allErrs field.ErrorList
	basePath := field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges")

	// Validate DHCP ranges
	for idx, dhcpRange := range r.Spec.IronicInspector.DHCPRanges {
		if err := r.validateDHCPRange(dhcpRange, basePath.Index(idx)); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	return allErrs
}

// validateConductorSpec
func (r *Ironic) validateConductorSpec() field.ErrorList {
	var allErrs field.ErrorList

	// validateConductoGroupsUnique
	if err := r.validateConductoGroupsUnique(); err != nil {
		allErrs = append(allErrs, err)
	}

	// Validate DHCP ranges - Ironic Conductor
	for condIdx, conductor := range r.Spec.IronicConductors {
		for idx, dhcpRange := range conductor.DHCPRanges {
			path := field.NewPath("spec").Child("ironicConductors").Index(condIdx).Child("dhcpRanges").Index(idx)
			if err := r.validateDHCPRange(dhcpRange, path); err != nil {
				allErrs = append(allErrs, err...)
			}
		}
	}

	return allErrs
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

func (r *Ironic) validateDHCPRange(
	dhcpRange DHCPRange,
	path *field.Path,
) field.ErrorList {
	var allErrs field.ErrorList

	cidr := dhcpRange.Cidr
	start := dhcpRange.Start
	end := dhcpRange.End
	gateway := dhcpRange.Gateway

	var startAddr net.IP
	var endAddr net.IP
	var gatewayAddr net.IP

	_, ipPrefix, ipPrefixErr := net.ParseCIDR(cidr)
	if ipPrefixErr != nil {
		allErrs = append(allErrs, field.Invalid(path.Child("cidr"), cidr, errInvalidCidr))
	}
	startAddr = net.ParseIP(start)
	if startAddr == nil {
		allErrs = append(allErrs, field.Invalid(path.Child("start"), start, errNotIPAddr))
	}
	endAddr = net.ParseIP(end)
	if endAddr == nil {
		allErrs = append(allErrs, field.Invalid(path.Child("end"), end, errNotIPAddr))
	}
	if gateway != "" {
		gatewayAddr = net.ParseIP(gateway)
		if gatewayAddr == nil {
			allErrs = append(allErrs, field.Invalid(path.Child("gateway"), gateway, errNotIPAddr))
		}
	}

	//
	// NOTE! return if any of these are invalid, continuing will potentially cause panic!
	//
	if ipPrefixErr != nil || startAddr == nil || endAddr == nil || gatewayAddr == nil {
		return allErrs
	}

	// Validate IP Family for IPv4
	if k8snet.IsIPv4CIDR(ipPrefix) {
		if !(k8snet.IsIPv4(startAddr) && k8snet.IsIPv4(endAddr)) {
			allErrs = append(allErrs, field.Invalid(path, dhcpRange, errMixedAddressFamily))
		} else if gateway != "" && !k8snet.IsIPv4(gatewayAddr) {
			allErrs = append(allErrs, field.Invalid(path, dhcpRange, errMixedAddressFamily))
		}
	}

	// Validate IP Family for IPv6
	if k8snet.IsIPv6CIDR(ipPrefix) {
		if !(k8snet.IsIPv6(startAddr) && k8snet.IsIPv6(endAddr)) {
			allErrs = append(allErrs, field.Invalid(path, dhcpRange, errMixedAddressFamily))
		} else if gateway != "" && !k8snet.IsIPv6(gatewayAddr) {
			allErrs = append(allErrs, field.Invalid(path, dhcpRange, errMixedAddressFamily))
		}
	}

	// Validate start, end and gateway in cidr
	if !ipPrefix.Contains(startAddr) {
		allErrs = append(allErrs, field.Invalid(path.Child("start"), start, fmt.Sprintf(errNotInCidr, cidr)))
	}
	if !ipPrefix.Contains(endAddr) {
		allErrs = append(allErrs, field.Invalid(path.Child("end"), end, fmt.Sprintf(errNotInCidr, cidr)))
	}
	if gateway != "" && !ipPrefix.Contains(gatewayAddr) {
		allErrs = append(allErrs, field.Invalid(path.Child("gateway"), gateway, fmt.Sprintf(errNotInCidr, cidr)))
	}

	// Start address should be < End address
	// The result will be 0 if a == b, -1 if a < b, and +1 if a > b.
	if bytes.Compare(endAddr, startAddr) != 1 {
		allErrs = append(allErrs, field.Invalid(path.Child("start"), start, fmt.Sprintf(errInvalidRange, start, end)))
		allErrs = append(allErrs, field.Invalid(path.Child("end"), end, fmt.Sprintf(errInvalidRange, start, end)))
	}

	return allErrs
}


// validateDHCPRangesOverlap
// Check for overlapping start->end in all DHCP ranges. (Conductor and Inspector)
func (r *Ironic) validateDHCPRangesOverlap() field.ErrorList {
	var allErrs field.ErrorList
	var netIPStartEnds []netIPStartEnd
	conductorPath := field.NewPath("spec").Child("ironicConductors")
	inspectorPath := field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges")

	for idx, dhcpRange := range r.Spec.IronicInspector.DHCPRanges {
		start := net.ParseIP(dhcpRange.Start)
		end := net.ParseIP(dhcpRange.End)
		if start == nil || end == nil {
			// If net.ParseIP returns 'nil' the address is not valid, the issue
			// has already been detected by previous validation ...
			// can safely skip here.
			continue
		}
		netIPStartEnds = append(
			netIPStartEnds, 
			netIPStartEnd{
				start: start,
				end: end,
				path: inspectorPath.Index(idx),
			},
		)
	}
	
	for condIdx, conductor := range r.Spec.IronicConductors {
		for idx, dhcpRange := range conductor.DHCPRanges {
			start := net.ParseIP(dhcpRange.Start)
			end := net.ParseIP(dhcpRange.End)
			if start == nil || end == nil {
				// If net.ParseIP returns 'nil' the address is not valid, the issue
			    // has already been detected by previous validation ... 
				// can safely skip here.
				continue
			}
			netIPStartEnds = append(
				netIPStartEnds, 
				netIPStartEnd{
					start: start,
					end: end,
					path: conductorPath.Index(condIdx).Child("dhcpRanges").Index(idx),
				},
			)
		}
	}

	for ax := 0; ax < len(netIPStartEnds); ax++ {
		for bx := 0; bx < len(netIPStartEnds); bx++ {
			if bx == ax {
				continue
			}
			allErrs = r.validateStartEndOverlap(netIPStartEnds[ax], netIPStartEnds[bx])	
		}
	}

	return allErrs
}


// validateStartEndOverlap -
// Check that start->end does not overlap
func (r *Ironic) validateStartEndOverlap(
	a netIPStartEnd,
	b netIPStartEnd,
) field.ErrorList {
	var allErrs field.ErrorList

	// bytes.Compare() return values:
	//   if a < b => -1
	//   if a = b =>  0
	//   if a > b =>  1
	switch x := bytes.Compare(a.start, b.start); x {
	case -1: // a.Start < b.Start -> CHECK: a.End < b.Start
		switch x := bytes.Compare(a.end, b.start); x {
		case -1: // a.End < b.Start :: OK
			return allErrs
		case 0, 1: // a.Start < b.Start && a.End >= b.Start :: FORBIDDEN
		    aRange := fmt.Sprintf("%v->%v", a.start.String(), a.end.String())
		    bRange := fmt.Sprintf("%v->%v", b.start.String(), b.end.String())
			allErrs = append(
				allErrs,
				field.Forbidden(
					a.path,
					fmt.Sprintf(errForbiddenAddressOverlap, aRange, b.path, bRange)),
			)
			return allErrs
		}
	case 0: // a.Start == b.Start :: FORBIDDEN
	    aRange := fmt.Sprintf("%v->%v", a.start.String(), a.end.String())
		bRange := fmt.Sprintf("%v->%v", b.start.String(), b.end.String())
		allErrs = append(
			allErrs,
			field.Forbidden(
				a.path,
				fmt.Sprintf(errForbiddenAddressOverlap, aRange, b.path, bRange)),
		)
		return allErrs
	case 1: // a.Start > theirStart -> CHECK: a.Start > b.End
		switch x := bytes.Compare(a.start, b.end); x {
		case 1: // a.Start > b.End -> OK
			return allErrs
		case 0, -1: // a.start > b.Start && a.Start <= b.End :: FORBIDDEN
		    aRange := fmt.Sprintf("%v->%v", a.start.String(), a.end.String())
			bRange := fmt.Sprintf("%v->%v", b.start.String(), b.end.String())
			allErrs = append(
				allErrs,
				field.Forbidden(
					a.path,
					fmt.Sprintf(errForbiddenAddressOverlap, aRange, b.path, bRange)),
			)
			return allErrs
		}
	}

	return allErrs
}
