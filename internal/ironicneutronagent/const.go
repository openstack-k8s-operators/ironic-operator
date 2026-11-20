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

// Package ironicneutronagent contains ironic neutron agent constants and configuration.
package ironicneutronagent

import (
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
)

const (
	// ServiceName -
	ServiceName = "ironic-neutron-agent"

	// ServiceType -
	ServiceType = "baremetal"

	// IronicNeutronAgent is the definition of the ironic-neutron-agent group
	IronicNeutronAgent storage.PropagationType = "IronicNeutronAgent"
	// Ironic is the global ServiceType that refers to all the components
	// deployed by the ironic-operator
	Ironic storage.PropagationType = "Ironic"
)

// IronicNeutronAgentPropagation is the definition of the IronicNeutronAgent
// propagation group. It allows the ironic-neutron-agent pod to mount volumes
// destined to Ironic and IronicNeutronAgent ServiceTypes
var IronicNeutronAgentPropagation = []storage.PropagationType{Ironic, IronicNeutronAgent}
