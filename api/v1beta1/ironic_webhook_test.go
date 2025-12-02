/*
Copyright 2025 Red Hat Inc.

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
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateDHCPRange(t *testing.T) {
	testCases := []struct {
		name         string
		dhcpRange    DHCPRange
		path         *field.Path
		expectedErrs field.ErrorList
	}{
		{
			name: "Valid DHCP range",
			dhcpRange: DHCPRange{
				Cidr:    "192.168.1.0/24",
				Start:   "192.168.1.10",
				End:     "192.168.1.20",
				Gateway: "192.168.1.1",
			},
			path:         field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: nil,
		},
		{
			name: "Invalid CIDR",
			dhcpRange: DHCPRange{
				Cidr:  "not-a-cidr",
				Start: "192.168.1.10",
				End:   "192.168.1.20",
			},
			path: field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("cidr"),
					"not-a-cidr",
					errInvalidCidr,
				),
			},
		},
		{
			name: "Invalid Start IP Address",
			dhcpRange: DHCPRange{
				Cidr:  "192.168.1.0/24",
				Start: "not-an-ipaddr",
				End:   "192.168.1.20",
			},
			path: field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("start"),
					"not-an-ipaddr",
					errNotIPAddr,
				),
			},
		},
		{
			name: "Invalid End IP Address",
			dhcpRange: DHCPRange{
				Cidr:  "192.168.1.0/24",
				Start: "192.168.1.10",
				End:   "not-an-ipaddr",
			},
			path: field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("end"),
					"not-an-ipaddr",
					errNotIPAddr,
				),
			},
		},
		{
			name: "Invalid Gateway Address",
			dhcpRange: DHCPRange{
				Cidr:    "192.168.1.0/24",
				Start:   "192.168.1.10",
				End:     "192.168.1.20",
				Gateway: "not-an-ipaddr",
			},
			path: field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("gateway"),
					"not-an-ipaddr",
					errNotIPAddr,
				),
			},
		},
		{
			name: "Mixed Address Family (start)",
			dhcpRange: DHCPRange{
				Cidr:    "192.168.1.0/24",
				Start:   "2001:db8::1",
				End:     "192.168.1.20",
				Gateway: "192.168.1.1",
			},
			path: field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
					DHCPRange{
						Cidr:    "192.168.1.0/24",
						Start:   "2001:db8::1",
						End:     "192.168.1.20",
						Gateway: "192.168.1.1",
					},
					errMixedAddressFamily,
				),
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("start"),
					"2001:db8::1",
					fmt.Sprintf(errNotInCidr, "192.168.1.0/24"),
				),
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("start"),
					"2001:db8::1",
					fmt.Sprintf(errInvalidRange, "2001:db8::1", "192.168.1.20"),
				),
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("end"),
					"192.168.1.20",
					fmt.Sprintf(errInvalidRange, "2001:db8::1", "192.168.1.20"),
				),
			},
		},
		{
			name: "Invalid IPRange",
			dhcpRange: DHCPRange{
				Cidr:    "192.168.1.0/24",
				Start:   "192.168.1.20",
				End:     "192.168.1.10",
				Gateway: "192.168.1.1",
			},
			path: field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("start"),
					"192.168.1.20",
					fmt.Sprintf(errInvalidRange, "192.168.1.20", "192.168.1.10"),
				),
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("end"),
					"192.168.1.10",
					fmt.Sprintf(errInvalidRange, "192.168.1.20", "192.168.1.10"),
				),
			},
		},
		{
			name: "IPv6 DHCP range",
			dhcpRange: DHCPRange{
				Cidr:  "2620:cf:cf:ffff::/64",
				Start: "2620:cf:cf:ffff::190",
				End:   "2620:cf:cf:ffff::199",
			},
			path:         field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: nil,
		},
		{
			name: "Invalid IPv6 DHCP range with gateway",
			dhcpRange: DHCPRange{
				Cidr:    "2620:cf:cf:ffff::/64",
				Start:   "2620:cf:cf:ffff::190",
				End:     "2620:cf:cf:ffff::199",
				Gateway: "2620:cf:cf:ffff::1",
			},
			path: field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("gateway"),
					"2620:cf:cf:ffff::1",
					errIPv6Gateway,
				),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateDHCPRange(tc.dhcpRange, tc.path)

			if !reflect.DeepEqual(errs, tc.expectedErrs) {
				t.Errorf("validateDHCPRange() failed:\n    expected: %v\n    got:      %v", tc.expectedErrs, errs)
			}
		})
	}
}

func TestValidateConductorGroupsUnique(t *testing.T) {
	testCases := []struct {
		name         string
		spec         *IronicSpecCore
		basePath     *field.Path
		expectedErrs *field.Error
	}{
		{
			name:         "No Conductor Group",
			spec:         &IronicSpecCore{IronicConductors: []IronicConductorTemplate{}},
			basePath:     field.NewPath("spec"),
			expectedErrs: nil,
		},

		{
			name: "Empty Conductor and One Normal Group",
			spec: &IronicSpecCore{IronicConductors: []IronicConductorTemplate{
				{ConductorGroup: ""},
				{ConductorGroup: "Group1"},
			}},
			basePath:     field.NewPath("spec"),
			expectedErrs: nil,
		},
		{
			name: "Duplicate Conductor Group",
			spec: &IronicSpecCore{IronicConductors: []IronicConductorTemplate{
				{ConductorGroup: "Group1"},
				{ConductorGroup: "Group2"},
				{ConductorGroup: "Group1"},
			}},
			basePath: field.NewPath("spec"),
			expectedErrs: field.Invalid(
				field.NewPath("spec").Child("ironicConductors"),
				[]IronicConductorTemplate{
					{ConductorGroup: "Group1"},
					{ConductorGroup: "Group2"},
					{ConductorGroup: "Group1"},
				},
				"ConductorGroup must be unique: #0: \"Group1\" duplicate of #2: \"Group1\"",
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateConductorGroupsUnique(tc.spec, tc.basePath)
			if !reflect.DeepEqual(errs, tc.expectedErrs) {
				t.Errorf("validateConductorGroupsUnique() failed:\n    expected: %v\n    got:      %v", tc.expectedErrs, errs)
			}
		})
	}
}

func TestValidateRPCTransport(t *testing.T) {
	testCases := []struct {
		name         string
		spec         *IronicSpecCore
		basePath     *field.Path
		expectedErrs *field.Error
	}{
		{
			name:         "Validate rpcTransport is oslo",
			spec:         &IronicSpecCore{RPCTransport: "oslo"},
			basePath:     field.NewPath("spec").Child("RPCTransport"),
			expectedErrs: nil,
		},
		{
			name:         "Validate rpcTransport is json-rpc",
			spec:         &IronicSpecCore{RPCTransport: "json-rpc"},
			basePath:     field.NewPath("spec").Child("RPCTransport"),
			expectedErrs: nil,
		},
		{
			name:     "Invalid rpcTransport Value",
			spec:     &IronicSpecCore{RPCTransport: "yaml"},
			basePath: field.NewPath("spec"),
			expectedErrs: field.Invalid(
				field.NewPath("spec").Child("rpcTransport"),
				"yaml",
				errInvalidRPCTransport,
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateRPCTransport(tc.spec, tc.basePath)

			if !reflect.DeepEqual(errs, tc.expectedErrs) {
				t.Errorf("validateRPCTransport() failed:\n    expected: %v\n    got:      %v", tc.expectedErrs, errs)
			}
		})
	}
}

func TestValidateConductorSpec(t *testing.T) {
	testCases := []struct {
		name         string
		spec         *IronicSpecCore
		basePath     *field.Path
		expectedErrs field.ErrorList
	}{
		{
			name:     "IronicConductors is not",
			spec:     &IronicSpecCore{},
			basePath: field.NewPath("spec"),
			expectedErrs: field.ErrorList{
				field.Required(
					field.NewPath("spec").Child("ironicConductors"),
					"IronicConductors must be provided",
				),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateConductorSpec(tc.spec, tc.basePath)

			if !reflect.DeepEqual(errs, tc.expectedErrs) {
				t.Errorf("validateConductorSpec() failed:\n    expected: %v\n    got:      %v", tc.expectedErrs, errs)
			}
		})
	}
}

func TestValidateDHCPRangesOverlap(t *testing.T) {
	testCases := []struct {
		name         string
		spec         *IronicSpecCore
		basePath     *field.Path
		expectedErrs field.ErrorList
	}{
		{
			name: "No overlap",
			spec: &IronicSpecCore{
				IronicConductors: []IronicConductorTemplate{
					{DHCPRanges: []DHCPRange{{Start: "192.168.1.10", End: "192.168.1.20"}}},
				},
				IronicInspector: IronicInspectorTemplate{
					DHCPRanges: []DHCPRange{{Start: "192.168.2.10", End: "192.168.2.20"}},
				},
			},
			basePath:     field.NewPath("spec"),
			expectedErrs: field.ErrorList{},
		},
		{
			name: "Overlap between Conductor and Inspector",
			spec: &IronicSpecCore{
				IronicConductors: []IronicConductorTemplate{
					{DHCPRanges: []DHCPRange{{Start: "192.168.1.10", End: "192.168.1.20"}}},
				},
				IronicInspector: IronicInspectorTemplate{
					DHCPRanges: []DHCPRange{{Start: "192.168.1.15", End: "192.168.1.25"}},
				},
			},
			basePath: field.NewPath("spec"),
			expectedErrs: field.ErrorList{
				field.Forbidden(
					field.NewPath("spec").Child("ironicConductors").Index(0).Child("dhcpRanges").Index(0),
					fmt.Sprintf(
						errForbiddenAddressOverlap,
						"192.168.1.10->192.168.1.20",
						field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
						"192.168.1.15->192.168.1.25",
					),
				),
			},
		},
		{
			name: "Overlap within same Conductors",
			spec: &IronicSpecCore{
				IronicConductors: []IronicConductorTemplate{
					{DHCPRanges: []DHCPRange{
						{Start: "192.168.1.10", End: "192.168.1.20"},
						{Start: "192.168.1.15", End: "192.168.1.25"},
					}},
				},
			},
			basePath: field.NewPath("spec"),
			expectedErrs: field.ErrorList{
				field.Forbidden(
					field.NewPath("spec").Child("ironicConductors").Index(0).Child("dhcpRanges").Index(1),
					fmt.Sprintf(
						errForbiddenAddressOverlap,
						"192.168.1.15->192.168.1.25",
						field.NewPath("spec").Child("ironicConductors").Index(0).Child("dhcpRanges").Index(0),
						"192.168.1.10->192.168.1.20",
					),
				),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateDHCPRangesOverlap(tc.spec, tc.basePath)

			// A successful validation can return nil or an empty list. We normalize to an empty list for comparison.
			if errs == nil {
				errs = field.ErrorList{}
			}

			if !reflect.DeepEqual(errs, tc.expectedErrs) {
				t.Errorf("validateDHCPRangesOverlap failed:\n    expected: %v\n    got:      %v", tc.expectedErrs, errs)
			}
		})
	}
}

func TestValidateNeutronAgentSpec(t *testing.T) {
	testCases := []struct {
		name         string
		spec         *IronicSpecCore
		basePath     *field.Path
		expectedErrs field.ErrorList
	}{
		{
			name: "IronicNeutronAgent replicas: 2 and Ironic standalone is true",
			spec: &IronicSpecCore{
				Standalone: true,
				IronicNeutronAgent: IronicNeutronAgentTemplate{
					IronicServiceTemplate: IronicServiceTemplate{
						Replicas: func(i int32) *int32 { return &i }(2),
					},
					RabbitMqClusterName: "rabbitmq"},
			},
			basePath: field.NewPath("spec"),
			expectedErrs: field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicNeutronAgent").Child("replicas"),
					func(i int32) *int32 { return &i }(2),
					"IronicNeutronAgent is not supported with standalone mode",
				),
			},
		},
		{
			name: "IronicNeutronAgent replicas: 1 and Ironic standalone is false",
			spec: &IronicSpecCore{
				Standalone: false,
				IronicNeutronAgent: IronicNeutronAgentTemplate{
					IronicServiceTemplate: IronicServiceTemplate{
						Replicas: func(i int32) *int32 { return &i }(1),
					},
					RabbitMqClusterName: "rabbitmq"},
			},
			basePath:     field.NewPath("spec"),
			expectedErrs: field.ErrorList{},
		},
		{
			name: "IronicNeutronAgent with Ironic standalone true",
			spec: &IronicSpecCore{
				Standalone: true,
				IronicNeutronAgent: IronicNeutronAgentTemplate{
					IronicServiceTemplate: IronicServiceTemplate{
						Replicas: func(i int32) *int32 { return &i }(0),
					},
					RabbitMqClusterName: "rabbitmq"},
			},
			basePath:     field.NewPath("spec"),
			expectedErrs: field.ErrorList{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateNeutronAgentSpec(tc.spec, tc.basePath)

			// A successful validation can return nil or an empty list. We normalize to an empty list for comparison.
			if errs == nil {
				errs = field.ErrorList{}
			}

			// Instead, compare the lengths and the string representation of each error.
			if len(errs) != len(tc.expectedErrs) {
				t.Fatalf("validateNeutronAgentSpec() wrong number of errors:\n    expected: %d (%v)\n    got:      %d (%v)",
					len(tc.expectedErrs), tc.expectedErrs, len(errs), errs)
			}

			for i := range errs {
				if errs[i].Error() != tc.expectedErrs[i].Error() {
					t.Errorf("validateNeutronAgentSpec() error mismatch at index %d:\n    expected: %s\n    got:      %s",
						i, tc.expectedErrs[i].Error(), errs[i].Error())
				}
			}
		})
	}
}
