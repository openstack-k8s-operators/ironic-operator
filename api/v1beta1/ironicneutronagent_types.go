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
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IronicNeutronAgentTemplate defines the input parameters for ML2 baremetal - ironic-neutron-agent agents
type IronicNeutronAgentTemplate struct {
	// Common input parameters for all Ironic services
	IronicServiceTemplate `json:",inline"`
}

// IronicNeutronAgentSpec defines the desired state of ML2 baremetal - ironic-neutron-agent agents
type IronicNeutronAgentSpec struct {
	// Input parameters for ironic-neutron-agent
	IronicNeutronAgentTemplate `json:",inline"`

	// +kubebuilder:validation:Optional
	// ContainerImage - ML2 baremtal - Ironic Neutron Agent Image
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// ServiceUser - optional username used for this service to register in ironic
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// Secret containing OpenStack password information for IronicPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: IronicPassword}
	// PasswordSelectors - Selectors to identify the ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Ironic
	RabbitMqClusterName string `json:"rabbitMqClusterName"`
}

// IronicNeutronAgentStatus defines the observed state of ML2 baremetal - ironic-neutron-agent
type IronicNeutronAgentStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount of ironic Conductor instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Networks",type="string",JSONPath=".status.networks",description="Networks"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// IronicNeutronAgent is the Schema for the ML2 baremetal - ironic-neutron-agent agents
type IronicNeutronAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IronicNeutronAgentSpec   `json:"spec,omitempty"`
	Status IronicNeutronAgentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IronicNeutronAgentList contains a list of IronicConductor
type IronicNeutronAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IronicNeutronAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&IronicNeutronAgent{},
		&IronicNeutronAgentList{},
	)
}

// IsReady - returns true if IronicNeutronAgent is reconciled successfully
func (instance IronicNeutronAgent) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance IronicNeutronAgent) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance IronicNeutronAgent) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance IronicNeutronAgent) RbacResourceName() string {
	owningIronicName := GetOwningIronicName(&instance)
	if owningIronicName != "" {
		return "ironic-" + owningIronicName
	}
	return "ironicneutronagent-" + instance.Name
}
