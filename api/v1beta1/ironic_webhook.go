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

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	k8snet "k8s.io/utils/net"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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
	errInvalidRPCTransport     = "RPCTransport must be either oslo or json-rpc"
)

type netIPStartEnd struct {
	start net.IP      // Start address of DHCP Range
	end   net.IP      // End address of DHCP Range
	path  *field.Path // Field path to DHCP Range in Ironic spec
}

var imageDefaults IronicImages

// SetupIronicImageDefaults - initialize Ironic spec defaults for use with either internal or external webhooks
func SetupIronicImageDefaults(images IronicImages) {
	imageDefaults = images
	ironiclog.Info("Ironic defaults initialized", "images", imageDefaults)
}

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *Ironic) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-ironic-openstack-org-v1beta1-ironic,mutating=false,failurePolicy=fail,sideEffects=None,groups=ironic.openstack.org,resources=ironics,verbs=create;update,versions=v1beta1,name=vironic.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/mutate-ironic-openstack-org-v1beta1-ironic,mutating=true,failurePolicy=fail,sideEffects=None,groups=ironic.openstack.org,resources=ironics,verbs=create;update,versions=v1beta1,name=mironic.kb.io,admissionReviewVersions=v1

var (
	_ webhook.Validator = &Ironic{}
	_ webhook.Defaulter = &Ironic{}
)

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Ironic) ValidateCreate() (admission.Warnings, error) {
	ironiclog.Info("validate create", "name", r.Name)
	var allErrs field.ErrorList
	basePath := field.NewPath("spec")

	if err := r.Spec.ValidateCreate(basePath, r.Namespace); err != nil {
		allErrs = append(allErrs, err...)
	}

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ironic.openstack.org", Kind: "Ironic"},
			r.Name, allErrs)
	}

	return nil, nil
}

// ValidateCreate - Exported function wrapping non-exported validate functions,
// this function can be called externally to validate an ironic spec.
func (spec *IronicSpec) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
	return spec.IronicSpecCore.ValidateCreate(basePath, namespace)
}

func (spec *IronicSpecCore) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
	var allErrs field.ErrorList

	if err := validateRPCTransport(spec, basePath); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := validateConductorGroupsUnique(spec, basePath); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := validateAPISpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateConductorSpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateInspectorSpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateDHCPRangesOverlap(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateNeutronAgentSpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	allErrs = append(allErrs, spec.ValidateIronicTopology(basePath, namespace)...)
	return allErrs
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Ironic) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	ironiclog.Info("validate update", "name", r.Name)

	oldIronic, ok := old.(*Ironic)
	if !ok || oldIronic == nil {
		return nil, apierrors.NewInternalError(fmt.Errorf("unable to convert existing object"))
	}

	var allErrs field.ErrorList
	basePath := field.NewPath("spec")

	if err := r.Spec.ValidateUpdate(oldIronic.Spec, basePath, r.Namespace); err != nil {
		allErrs = append(allErrs, err...)
	}

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ironic.openstack.org", Kind: "Ironic"},
			r.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate - Exported function wrapping non-exported validate functions,
// this function can be called externally to validate an ironic spec.
func (spec *IronicSpec) ValidateUpdate(old IronicSpec, basePath *field.Path, namespace string) field.ErrorList {
	return spec.IronicSpecCore.ValidateUpdate(old.IronicSpecCore, basePath, namespace)
}

func (spec *IronicSpecCore) ValidateUpdate(old IronicSpecCore, basePath *field.Path, namespace string) field.ErrorList {
	var allErrs field.ErrorList

	if err := validateRPCTransport(spec, basePath); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := validateConductorGroupsUnique(spec, basePath); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := validateAPISpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateConductorSpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateInspectorSpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateDHCPRangesOverlap(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	if err := validateNeutronAgentSpec(spec, basePath); err != nil {
		allErrs = append(allErrs, err...)
	}

	allErrs = append(allErrs, spec.ValidateIronicTopology(basePath, namespace)...)
	return allErrs
}

// NOTE: webhook.Validator requires this function to exist even as a no-op
func (r *Ironic) ValidateDelete() (admission.Warnings, error) {
	ironiclog.Info("validate delete", "name", r.Name)

	return nil, nil
}

func validateAPISpec(spec *IronicSpecCore, basePath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// validate the service override key is valid
	allErrs = append(allErrs, service.ValidateRoutedOverrides(
		basePath.Child("ironicAPI").Child("override").Child("service"),
		spec.IronicAPI.Override.Service)...)

	return allErrs
}

func validateInspectorSpec(spec *IronicSpecCore, basePath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// validate the service override key is valid
	allErrs = append(allErrs, service.ValidateRoutedOverrides(
		basePath.Child("ironicInspector").Child("override").Child("service"),
		spec.IronicInspector.Override.Service)...)

	fieldPath := basePath.Child("ironicInspector").Child("dhcpRanges")

	// Validate DHCP ranges
	for idx, dhcpRange := range spec.IronicInspector.DHCPRanges {
		if err := validateDHCPRange(dhcpRange, fieldPath.Index(idx)); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	return allErrs
}

// validateConductorSpec
func validateConductorSpec(spec *IronicSpecCore, basePath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.IronicConductors == nil || len(spec.IronicConductors) == 0 {
		allErrs = append(allErrs, field.Required(
			basePath.Child("ironicConductors"),
			"IronicConductors must be provided",
		))
	}

	// validateConductorGroupsUnique
	if err := validateConductorGroupsUnique(spec, basePath); err != nil {
		allErrs = append(allErrs, err)
	}

	// Validate DHCP ranges - Ironic Conductor
	for condIdx, conductor := range spec.IronicConductors {
		for idx, dhcpRange := range conductor.DHCPRanges {
			path := basePath.Child("ironicConductors").Index(condIdx).Child("dhcpRanges").Index(idx)
			if err := validateDHCPRange(dhcpRange, path); err != nil {
				allErrs = append(allErrs, err...)
			}
		}
	}

	return allErrs
}

// validateConductorGroupsUnique implements validation of IronicConductor ConductorGroup
func validateConductorGroupsUnique(spec *IronicSpecCore, basePath *field.Path) *field.Error {
	fieldPath := basePath.Child("ironicConductors")
	var groupName string
	seenGrps := make(map[string]int)
	dupes := make(map[int]string)

	for groupIdx, c := range spec.IronicConductors {
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

		return field.Invalid(fieldPath, spec.IronicConductors, err)
	}

	return nil
}

// validateNeutronAgentSpec
func validateNeutronAgentSpec(spec *IronicSpecCore, basePath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.Standalone && *spec.IronicNeutronAgent.Replicas > 0 {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("ironicNeutronAgent").Child("replicas"),
			*spec.IronicNeutronAgent.Replicas,
			"IronicNeutronAgent is not supported with standalone mode",
		))
	}

	return allErrs
}

func validateDHCPRange(
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
func validateDHCPRangesOverlap(spec *IronicSpecCore, basePath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	var netIPStartEnds []netIPStartEnd
	conductorPath := basePath.Child("ironicConductors")
	inspectorPath := basePath.Child("ironicInspector").Child("dhcpRanges")

	for idx, dhcpRange := range spec.IronicInspector.DHCPRanges {
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
				end:   end,
				path:  inspectorPath.Index(idx),
			},
		)
	}

	for condIdx, conductor := range spec.IronicConductors {
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
					end:   end,
					path:  conductorPath.Index(condIdx).Child("dhcpRanges").Index(idx),
				},
			)
		}
	}

	for ax := 0; ax < len(netIPStartEnds); ax++ {
		for bx := 0; bx < len(netIPStartEnds); bx++ {
			if bx == ax {
				continue
			}
			allErrs = validateStartEndOverlap(netIPStartEnds[ax], netIPStartEnds[bx])
		}
	}

	return allErrs
}

// validateStartEndOverlap -
// Check that start->end does not overlap
func validateStartEndOverlap(
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

// validateRPCTransport
// Check that rpcTransport is either oslo or json-rpc. If it is done as an kubebuilder enum
// validation ironic as an optional service can not be omitted from the OpenStackControlPlane CR.
func validateRPCTransport(spec *IronicSpecCore, basePath *field.Path) *field.Error {
	rpcTransportPath := basePath.Child("rpcTransport")

	// validate that rpcTransport is either oslo or json-rpc
	rpcTransportOptions := []string{"oslo", "json-rpc"}
	if !util.StringInSlice(spec.RPCTransport, rpcTransportOptions) {
		return field.Invalid(rpcTransportPath, spec.RPCTransport, errInvalidRPCTransport)
	}

	return nil
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Ironic) Default() {
	ironiclog.Info("webhook - calling defaulter")

	// All defaulter functions called in Spec's Default method,
	// so that the defaulter can be triggered externally.
	r.Spec.Default()

	ironiclog.Info("webhook - defaulter called")
}

// Default - Exported function wrapping non-exported defaulter functions,
// this function can be called externally to default an ironic spec.
func (spec *IronicSpec) Default() {
	// only validate default images in this function
	if spec.Images.API == "" {
		spec.Images.API = imageDefaults.API
	}
	if spec.Images.Conductor == "" {
		spec.Images.Conductor = imageDefaults.Conductor
	}
	if spec.Images.Inspector == "" {
		spec.Images.Inspector = imageDefaults.Inspector
	}
	if spec.Images.NeutronAgent == "" {
		spec.Images.NeutronAgent = imageDefaults.NeutronAgent
	}
	if spec.Images.Pxe == "" {
		spec.Images.Pxe = imageDefaults.Pxe
	}
	if spec.Images.IronicPythonAgent == "" {
		spec.Images.IronicPythonAgent = imageDefaults.IronicPythonAgent
	}
	spec.IronicSpecCore.Default()
}

// Default - Exported function wrapping non-exported defaulter functions,
// this function can be called externally to default an ironic spec.
// NOTE: this version is called by OpenStackControlplane
func (spec *IronicSpecCore) Default() {
	if spec.RPCTransport == "" {
		spec.RPCTransport = "json-rpc"
	}
}

// ValidateIronicTopology - Returns an ErrorList if the Topology is referenced
// on a different namespace
func (spec *IronicSpecCore) ValidateIronicTopology(basePath *field.Path, namespace string) field.ErrorList {
	var allErrs field.ErrorList

	// When a TopologyRef CR is referenced, fail if a different Namespace is
	// referenced because is not supported
	allErrs = append(allErrs, topologyv1.ValidateTopologyRef(
		spec.TopologyRef, *basePath.Child("topologyRef"), namespace)...)

	// When a TopologyRef CR is referenced with an override to IronicAPI, fail
	// if a different Namespace is referenced because not supported
	apiPath := basePath.Child("ironicAPI")
	allErrs = append(allErrs,
		spec.IronicAPI.ValidateTopology(apiPath, namespace)...)

	// When a TopologyRef CR is referenced with an override to an instance of
	// IronicConductor(s),  fail if a different Namespace is referenced because
	// not supported
	for _, cs := range spec.IronicConductors {
		path := basePath.Child("ironicConductors")
		allErrs = append(allErrs,
			cs.ValidateTopology(path, namespace)...)
	}

	// When a TopologyRef CR is referenced with an override to an instance of
	// IronicInspector, fail if a different Namespace is referenced because not
	// supported
	insPath := basePath.Child("ironicInspector")
	allErrs = append(allErrs,
		spec.IronicInspector.ValidateTopology(insPath, namespace)...)

	// When a TopologyRef CR is referenced with an override to an instance of
	// IronicNeutronAgent, fail if a different Namespace is referenced because
	// not supported
	nagentPath := basePath.Child("ironicNeutronAgent")
	allErrs = append(allErrs,
		spec.IronicNeutronAgent.ValidateTopology(nagentPath, namespace)...)

	return allErrs
}

// SetDefaultRouteAnnotations sets HAProxy timeout values of the route
func (spec *IronicSpecCore) SetDefaultRouteAnnotations(annotations map[string]string) {
	const haProxyAnno = "haproxy.router.openshift.io/timeout"
	// Use a custom annotation to flag when the operator has set the default HAProxy timeout
	// The annotation func determines when to overwrite existing HAProxy timeout with the APITimeout
	const ironicAnno = "api.ironic.openstack.org/timeout"

	valIronic, okIronic := annotations[ironicAnno]
	valHAProxy, okHAProxy := annotations[haProxyAnno]

	// Human operator sets the HAProxy timeout manually
	if !okIronic && okHAProxy {
		return
	}

	// Human operator modified the HAProxy timeout manually without removing the Ironic flag
	if okIronic && okHAProxy && valIronic != valHAProxy {
		delete(annotations, ironicAnno)
		return
	}

	timeout := fmt.Sprintf("%ds", spec.APITimeout)
	annotations[ironicAnno] = timeout
	annotations[haProxyAnno] = timeout
}

// SetDefaultRouteAnnotations sets HAProxy timeout values of the route
func (spec *IronicSpecCore) SetDefaultInspectorRouteAnnotations(annotations map[string]string) {
	const haProxyAnno = "haproxy.router.openshift.io/timeout"
	// Use a custom annotation to flag when the operator has set the default HAProxy timeout
	// The annotation func determines when to overwrite existing HAProxy timeout with the APITimeout
	const ironicInspectorAnno = "inspector.ironic.openstack.org/timeout"

	valIronic, okIronic := annotations[ironicInspectorAnno]
	valHAProxy, okHAProxy := annotations[haProxyAnno]

	// Human operator sets the HAProxy timeout manually
	if !okIronic && okHAProxy {
		return
	}

	// Human operator modified the HAProxy timeout manually without removing the Ironic flag
	if okIronic && okHAProxy && valIronic != valHAProxy {
		delete(annotations, ironicInspectorAnno)
		return
	}

	timeout := fmt.Sprintf("%ds", spec.APITimeout)
	annotations[ironicInspectorAnno] = timeout
	annotations[haProxyAnno] = timeout
}
