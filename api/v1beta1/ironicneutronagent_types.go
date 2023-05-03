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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IronicNeutronAgentPasswordSelector to identify the AdminUser password from the Secret
type IronicNeutronAgentPasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="IronicPassword"
	// Service - Selector to get the ironic service password from the Secret
	Service string `json:"service"`
}

// IronicNeutronAgentDebug defines the observed state of IronicNeutronAgent
type IronicNeutronAgentDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Service enable debug
	Service bool `json:"service"`
}

// IronicNeutronAgentSpec defines the desired state of ML2 baremetal - ironic-neutron-agent agents
type IronicNeutronAgentSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// ServiceUser - optional username used for this service to register in ironic
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// ContainerImage - ML2 baremtal - Ironic Neutron Agent Image URL
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// Replicas - ML2 baremetal - Ironic Neutron Agent Replicas
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for IronicPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: IronicPassword}
	// PasswordSelectors - Selectors to identify the ServiceUser password from the Secret
	PasswordSelectors IronicNeutronAgentPasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug IronicNeutronAgentDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="# add your customization here"
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Ironic
	RabbitMqClusterName string `json:"rabbitMqClusterName"`

	// +kubebuilder:validation:Required
	// ServiceAccount - service account name used internally to provide the default SA name
	ServiceAccount string `json:"serviceAccount"`
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

// IsReady - returns true if service is ready to server requests
func (instance IronicNeutronAgent) IsReady() bool {
	return instance.Status.ReadyCount >= 1
}
