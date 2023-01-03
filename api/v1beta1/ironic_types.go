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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"
)

// IronicSpec defines the desired state of Ironic
type IronicSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Whether to deploy a single node standalone Ironic.
	Standalone bool `json:"standalone"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// ServiceUser - optional username used for this service to register in ironic
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// MariaDB instance name.
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB.
	// Might not be required in future.
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// DatabaseUser - optional username used for ironic DB, defaults to ironic.
	// TODO: -> implement needs work in mariadb-operator, right now only ironic.
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for ironic IronicDatabasePassword, AdminPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: IronicDatabasePassword, service: IronicPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password and TransportURL from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

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

	// +kubebuilder:validation:Required
	// IronicAPI - Spec definition for the API service of this Ironic deployment
	IronicAPI IronicAPISpec `json:"ironicAPI"`

	// +kubebuilder:validation:Required
	// IronicAPI - Spec definition for the conductor service of this Ironic deployment
	IronicConductor IronicConductorSpec `json:"ironicConductor"`

	// +kubebuilder:validation:Required
	// IronicAPI - Spec definition for the conductor service of this Ironic deployment
	IronicInspector IronicInspectorSpec `json:"ironicInspector"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Ironic
	RabbitMqClusterName string `json:"rabbitMqClusterName"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=json-rpc
	// RPC transport type - Which RPC transport implementation to use between
	// conductor and API services. 'oslo' to use oslo.messaging transport
	// or 'json-rpc' to use JSON RPC transport. NOTE -> ironic-inspector
	// requires oslo.messaging transport when not in standalone mode.
	RPCTransport string `json:"rpcTransport"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting
	// NodeSelector here acts as a default value and can be overridden by service
	// specific NodeSelector Settings.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="IronicDatabasePassword"
	// Database - Selector to get the ironic Database user password from the Secret
	// TODO: not used, need change in mariadb-operator
	Database string `json:"database"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="IronicPassword"
	// Database - Selector to get the ironic service password from the Secret
	Service string `json:"service"`
}

// DHCPRange to define address range for DHCP requestes
type DHCPRange struct {
	// +kubebuilder:validation:Optional
	// Name - Name of the DHCPRange (used for tagging in dnsmasq)
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Optional
	// Start - Start of DHCP range
	Start string `json:"start,omitempty"`
	// +kubebuilder:validation:Optional
	// End - End of DHCP range
	End string `json:"end,omitempty"`
	// +kubebuilder:validation:Optional
	// Gateway - IP address for the router
	Gateway string `json:"gateway,omitempty"`
	// +kubebuilder:validation:Optional
	// Prefix - IP network prefix (network mask bits) for IPv6
	Prefix int `json:"prefix,omitempty"`
	// +kubebuilder:validation:Optional
	// Netmask - IP network netmask (network mask bits) for IPv4
	Netmask string `json:"netmask,omitempty"`
	// +kubebuilder:validation:Optional
	// MTU - Maximum Transmission Unit
	MTU int `json:"mtu,omitempty"`
	// +kubebuilder:validation:Optional
	// PodIndex - Maps the DHCPRange to a specific statefulset pod index
	PodIndex int `json:"podIndex,omitempty"`
}

// IronicDebug defines the observed state of Ironic
type IronicDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// DBSync enable debug
	DBSync bool `json:"dbSync"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// ReadyCount enable debug
	Bootstrap bool `json:"bootstrap"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Service enable debug
	Service bool `json:"service"`
}

// IronicStatus defines the observed state of Ironic
type IronicStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Ironic Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// API endpoint
	APIEndpoints map[string]map[string]string `json:"apiEndpoints,omitempty"`

	// ServiceIDs
	ServiceIDs map[string]string `json:"serviceIDs,omitempty"`

	// ReadyCount of Ironic API instance
	IronicAPIReadyCount int32 `json:"ironicAPIReadyCount,omitempty"`

	// ReadyCount of Ironic Conductor instance
	IronicConductorReadyCount int32 `json:"ironicConductorReadyCount,omitempty"`

	// IronicInspectorServiceIDs
	InspectorServiceIDs map[string]string `json:"inspectorServiceIDs,omitempty"`

	// InspectorApiEndpoints
	InspectorAPIEndpoints map[string]map[string]string `json:"inspectorAPIEndpoints,omitempty"`

	// ReadyCount of Ironic Inspector instance
	InspectorReadyCount int32 `json:"ironicInspectorReadyCount,omitempty"`

	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Ironic is the Schema for the ironics API
type Ironic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IronicSpec   `json:"spec,omitempty"`
	Status IronicStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IronicList contains a list of Ironic
type IronicList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ironic `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Ironic{}, &IronicList{})
}

// // GetEndpoint - returns OpenStack endpoint url for type
// func (instance Ironic) GetEndpoint(endpointType endpoint.Endpoint) (string, error) {
// 	if url, found := instance.Status.APIEndpoints[string(endpointType)]; found {
// 		return url, nil
// 	}
// 	return "", fmt.Errorf("%s endpoint not found", string(endpointType))
// }

// IsReady - returns true if service is ready to server requests
func (instance Ironic) IsReady() bool {
	ready := instance.Status.IronicAPIReadyCount > 0

	ready = ready && instance.Status.IronicConductorReadyCount > 0

	// ready = ready && instance.Status.IronicInspectorReadyCount > 0

	return ready
}
