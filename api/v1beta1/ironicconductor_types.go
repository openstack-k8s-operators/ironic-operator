/*
Copyright 2022.

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

// IronicConductorSpec defines the desired state of IronicConductor
type IronicConductorSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:default=true
	// Whether to deploy a single node standalone Ironic.
	// TODO: -> not implemented, always standalone for now
	Standalone bool `json:"standalone,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// ServiceUser - optional username used for this service to register in ironic
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// ContainerImage - Ironic Conductor Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`

	// +kubebuilder:validation:Optional
	// PxeContainerImage - Ironic DHCP/TFTP/HTTP Container Image URL
	PxeContainerImage string `json:"pxeContainerImage,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// Replicas - Ironic Conductor Replicas
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// DatabaseHostname - Ironic Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// DatabaseUser - optional username used for ironic DB, defaults to ironic
	// TODO: -> implement needs work in mariadb-operator, right now only ironic
	DatabaseUser string `json:"databaseUser,omitempty"`

	// +kubebuilder:validation:Optional
	// Secret containing OpenStack password information for IronicDatabasePassword, AdminPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// PasswordSelectors - Selectors to identify the DB and AdminUser password and TransportURL from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors,omitempty"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes for running the Conductor service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug IronicDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="# add your customization here"
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// IronicConductorStatus defines the observed state of IronicConductor
type IronicConductorStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount of ironic Conductor instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// ServiceIDs
	ServiceIDs map[string]string `json:"serviceIDs,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// IronicConductor is the Schema for the ironicconductors Conductor
type IronicConductor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IronicConductorSpec   `json:"spec,omitempty"`
	Status IronicConductorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IronicConductorList contains a list of IronicConductor
type IronicConductorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IronicConductor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IronicConductor{}, &IronicConductorList{})
}

// IsReady - returns true if service is ready to server requests
func (instance IronicConductor) IsReady() bool {
	return instance.Status.ReadyCount >= 1
}
