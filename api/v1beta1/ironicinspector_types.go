/*
Copyright 2022 Red Hat Inc.

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

// IronicInspectorSpec defines the desired state of IronicInspector
type IronicInspectorSpec struct {

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Standalone - Whether to deploy a standalone Ironic Inspector.
	Standalone bool `json:"standalone"`

	// +kubebuilder:validation:Optional
	// ContainerImage - Ironic Inspector Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`

	// +kubebuilder:validation:Optional
	// PxeContainerImage - Ironic Inspector DHCP/TFTP/HTTP Container Image URL
	PxeContainerImage string `json:"pxeContainerImage,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic-inspector
	// ServiceUser - optional username used for this service to register in ironic-inspector
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:default=1
	// Replicas - Ironic Inspector Replicas
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// MariaDB instance name.
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB.
	// Might not be required in future.
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic_inspector
	// DatabaseUser - optional username used for ironic DB, defaults to ironic
	// TODO: -> implement needs work in mariadb-operator, right now only ironic
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Optional
	// Secret containing OpenStack password information for IronicInspectorDatabasePassword, AdminPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: IronicInspectorDatabasePassword, service: IronicInspectorPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password and TransportURL from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting here overrides
	// any global NodeSelector settings within the Ironic CR
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug IronicDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="# add your customization here"
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// StorageClass
	StorageClass string `json:"storageClass,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Ironic
	RabbitMqClusterName string `json:"rabbitMqClusterName"`

	// +kubebuilder:validation:Optional
	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=json-rpc
	// RPC transport type - Which RPC transport implementation to use between
	// conductor and API services. 'oslo' to use oslo.messaging transport
	// or 'json-rpc' to use JSON RPC transport. NOTE -> ironic-inspector
	// requires oslo.messaging transport when not in standalone mode.
	RPCTransport string `json:"rpcTransport"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments list of network attachment definitions the pods get attached to.
	NetworkAttachments []string `json:"networkAttachments"`

	// +kubebuilder:validation:Optional
	// InspectionNetwork - Additional network to attach to expose boot DHCP, TFTP, HTTP services.
	InspectionNetwork string `json:"inspectionNetwork,omitempty"`

	// +kubebuilder:validation:Optional
	// DHCPRanges - List of DHCP ranges to use for provisioning
	DHCPRanges []DHCPRange `json:"dhcpRanges,omitempty"`
}

// IronicInspectorStatus defines the observed state of IronicInspector
type IronicInspectorStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]map[string]string `json:"apiEndpoints,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// IronicInspector Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// ReadyCount of Ironic Inspector instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// ServiceIDs
	ServiceIDs map[string]string `json:"serviceIDs,omitempty"`

	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// Networks in addtion to the cluster network, the service is attached to
	Networks []string `json:"networks,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"
//+kubebuilder:printcolumn:name="Networks",type="string",JSONPath=".status.networks",description="Networks"

// IronicInspector is the Schema for the IronicInspector
type IronicInspector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IronicInspectorSpec   `json:"spec,omitempty"`
	Status IronicInspectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IronicInspectorList contains a list of IronicInspector
type IronicInspectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IronicInspector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IronicInspector{}, &IronicInspectorList{})
}

// IsReady - returns true if service is ready to server requests
func (instance IronicInspector) IsReady() bool {
	return instance.Status.ReadyCount >= 1
}
