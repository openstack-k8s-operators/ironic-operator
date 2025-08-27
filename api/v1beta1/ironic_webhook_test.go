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
			dhcpRange: 	DHCPRange{
						Cidr:  "not-a-cidr",
						Start: "192.168.1.10",
						End:   "192.168.1.20",
			},
			path: 		field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs: 	field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("cidr"),
					"not-a-cidr",
					errInvalidCidr,
				),
			},
		},
		{
			name: "Invalid Start IP Address",
			dhcpRange: 	DHCPRange{
						Cidr: 	"192.168.1.0/24",
						Start: 	"not-an-ipaddr",
						End: 	"192.168.1.20",
			},
			path: 		field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs:	field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("start"),
					"not-an-ipaddr",
					errNotIPAddr,
				),
			},
		},
		{
			name: "Invalid End IP Address",
			dhcpRange: 	DHCPRange{
						Cidr: 	"192.168.1.0/24",
						Start: 	"192.168.1.10",
						End: 	"not-an-ipaddr",
			},
			path: 		field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs:	field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("end"),
					"not-an-ipaddr",
					errNotIPAddr,
				),
			},
		},
		{
			name: "Invalid Gateway Address",
			dhcpRange: 	DHCPRange{
						Cidr: 	 "192.168.1.0/24",
						Start: 	 "192.168.1.10",
						End: 	 "192.168.1.20",
						Gateway: "not-an-ipaddr",
			},
			path: 		field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs:	field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("gateway"),
					"not-an-ipaddr",
					errNotIPAddr,
				),
			},
		},
		{
			name: "Mixed Address Family (start)",
			dhcpRange: 	DHCPRange{
						Cidr: 	 "192.168.1.0/24",
						Start: 	 "2001:db8::1",
						End: 	 "192.168.1.20",
						Gateway: "192.168.1.1",
			},
			path: 		field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs:	field.ErrorList{
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
					DHCPRange{
						Cidr: 	 "192.168.1.0/24",
						Start: 	 "2001:db8::1",
						End: 	 "192.168.1.20",
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
					fmt.Sprintf(errInvalidRange,"2001:db8::1", "192.168.1.20"),
				),
				field.Invalid(
					field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0).Child("end"),
					"192.168.1.20",
					fmt.Sprintf(errInvalidRange,"2001:db8::1", "192.168.1.20"),
				),
			},
		},
		{
			name: "Invalid IPRange",
			dhcpRange: 	DHCPRange{
						Cidr: 	 "192.168.1.0/24",
						Start: 	 "192.168.1.20",
						End: 	 "192.168.1.10",
						Gateway: "192.168.1.1",
			},
			path: 		field.NewPath("spec").Child("ironicInspector").Child("dhcpRanges").Index(0),
			expectedErrs:	field.ErrorList{
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
		name 		string
		spec 		*IronicSpecCore
		basePath 	*field.Path
		expectedErrs	*field.Error
	}{
		{
			name: 		"No Conductor Group",
			spec: 		&IronicSpecCore{IronicConductors: []IronicConductorTemplate{}},
			basePath:	field.NewPath("spec"),
			expectedErrs: 	nil,
		},

		{
			name: 		"Empty Conductor and One Normal Group",
			spec: 		&IronicSpecCore{IronicConductors: []IronicConductorTemplate{
						{ConductorGroup: ""},
						{ConductorGroup: "Group1"},
			}},
			basePath:	field.NewPath("spec"),
			expectedErrs: 	nil,
		},
		{
			name:		"Duplicate Conductor Group",
			spec:		&IronicSpecCore{IronicConductors: []IronicConductorTemplate{
						{ConductorGroup: "Group1"},
						{ConductorGroup: "Group2"},
						{ConductorGroup: "Group1"},
			}},
			basePath:	field.NewPath("spec"),
			expectedErrs:	field.Invalid(
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
		name		string
		spec		*IronicSpecCore
		basePath	*field.Path
		expectedErrs	*field.Error
	}{
		{
			name:		"Validate rpcTransport is oslo",
			spec:		&IronicSpecCore{RPCTransport: "oslo"},
			basePath:	field.NewPath("spec").Child("RPCTransport"),
			expectedErrs:	nil,
		},
		{
			name:		"Validate rpcTransport is json-rpc",
			spec:		&IronicSpecCore{RPCTransport: "json-rpc"},
			basePath:	field.NewPath("spec").Child("RPCTransport"),
			expectedErrs:	nil,
		},
		{
			name:		"Invalid rpcTransport Value",
			spec:		&IronicSpecCore{RPCTransport: "yaml"},
			basePath:	field.NewPath("spec"),
			expectedErrs:	field.Invalid(
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
		name		string
		spec		*IronicSpecCore
		basePath	*field.Path
		expectedErrs	field.ErrorList
	}{
		{
			name:		"IronicConductors is not",
			spec:		&IronicSpecCore{},
			basePath:	field.NewPath("spec"),
			expectedErrs:	field.ErrorList{
						field.Required(
							field.NewPath("spec").Child("ironicConductors"),
							"IonicConductors must be provided",
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
