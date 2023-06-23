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
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"

	// ConductorGroupNull - Used in IronicConductorReadyCount map and resource labels when ConductorGroup is not set
	ConductorGroupNull = "null_conductor_group_null"

	// Container image fall-back defaults

	// IronicAPIContainerImage is the fall-back container image for IronicAPI
	IronicAPIContainerImage = "quay.io/podified-antelope-centos9/openstack-ironic-api:current-podified"
	// IronicConductorContainerImage is the fall-back container image for IronicConductor
	IronicConductorContainerImage = "quay.io/podified-antelope-centos9/openstack-ironic-conductor:current-podified"
	// IronicInspectorContainerImage is the fall-back container image for IronicInspector
	IronicInspectorContainerImage = "quay.io/podified-antelope-centos9/openstack-ironic-inspector:current-podified"
	// IronicPXEContainerImage is the fall-back container image for IronicPXE
	IronicPXEContainerImage = "quay.io/podified-antelope-centos9/openstack-ironic-pxe:current-podified"
	// IronicNeutronAgentContainerImage is the fall-back container image for IronicConductor
	IronicNeutronAgentContainerImage = "quay.io/podified-antelope-centos9/openstack-ironic-neutron-agent:current-podified"
	// IronicPythonAgentContainerImage is the fall-back container image for IronicPythonAgent
	IronicPythonAgentContainerImage = "quay.io/podified-antelope-centos9/ironic-python-agent:current-podified"
)

// IronicSpec defines the desired state of Ironic
type IronicSpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Whether to deploy a standalone Ironic.
	Standalone bool `json:"standalone"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={}
	// Images - Container images for all ironic services
	Images IronicImages `json:"images"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// ServiceUser - optional username used for this service to register in ironic
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// MariaDB instance name.
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB.
	// Might not be required in future.
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for ironic IronicDatabasePassword, IronicPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: IronicDatabasePassword, service: IronicPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug IronicDBSyncDebug `json:"debug,omitempty"`

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
	IronicAPI IronicAPITemplate `json:"ironicAPI"`

	// +kubebuilder:validation:Required
	// IronicConductors - Spec definitions for the conductor service of this Ironic deployment
	IronicConductors []IronicConductorTemplate `json:"ironicConductors"`

	// +kubebuilder:validation:Required
	// IronicInspector - Spec definition for the inspector service of this Ironic deployment
	IronicInspector IronicInspectorTemplate `json:"ironicInspector"`

	// +kubebuilder:validation:Required
	// IronicNeutronAgent - Spec definition for the ML2 baremetal ironic-neutron-agent
	// service of this Ironic deployment
	IronicNeutronAgent IronicNeutronAgentTemplate `json:"ironicNeutronAgent"`

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

	// Storage class to host data. This is passed to IronicConductors unless
	// storageClass is explicitly set for the conductor.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=""
	StorageClass string `json:"storageClass"`
}

// IronicImages to specify container images required by all ironic services
type IronicImages struct {
	// +kubebuilder:validation:Optional
	// API - Ironic API Container Image (will be set to environmental default if empty)
	API string `json:"api"`

	// +kubebuilder:validation:Optional
	// Conductor - Ironic Conductor Container Image (will be set to environmental default if empty)
	Conductor string `json:"conductor"`

	// +kubebuilder:validation:Optional
	// Inspector - Ironic Inspector Container Image (will be set to environmental default if empty)
	Inspector string `json:"inspector"`

	// +kubebuilder:validation:Optional
	// NeutronAgent - ML2 baremtal - Ironic Neutron Agent Image (will be set to environmental default if empty)
	NeutronAgent string `json:"neutronAgent"`

	// +kubebuilder:validation:Optional
	// Pxe- Ironic DHCP/TFTP/HTTP Container Image URL (will be set to environmental default if empty)
	Pxe string `json:"pxe"`

	// +kubebuilder:validation:Optional
	// IronicPythonAgent - Image containing the ironic-python-agent kernel and ramdisk
	IronicPythonAgent string `json:"ironicPythonAgent"`
}

// DHCPRange to define address range for DHCP requestes
type DHCPRange struct {
	// +kubebuilder:validation:Optional
	// Name - Name of the DHCPRange (used for tagging in dnsmasq)
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Required
	// Cidr - IP address prefix (CIDR) representing an IP network.
	Cidr string `json:"cidr"`
	// +kubebuilder:validation:Required
	// Start - Start of DHCP range
	Start string `json:"start"`
	// +kubebuilder:validation:Required
	// End - End of DHCP range
	End string `json:"end"`
	// +kubebuilder:validation:Optional
	// Gateway - IP address for the router
	Gateway string `json:"gateway,omitempty"`
	// +kubebuilder:validation:Optional
	// MTU - Maximum Transmission Unit
	MTU int `json:"mtu,omitempty"`
	// +kubebuilder:validation:Optional
	// PodIndex - Maps the DHCPRange to a specific statefulset pod index
	PodIndex int `json:"podIndex,omitempty"`
	// Prefix - (Hidden) Internal use only, prefix (mask bits) for IPv6 is autopopulated from Cidr
	Prefix int `json:"-"`
	// Netmask - (Hidden) Inernal use only, netmask for IPv4 is autopopulated from Cidr
	Netmask string `json:"-"`
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

	// ReadyCount of Ironic API instance
	IronicAPIReadyCount int32 `json:"ironicAPIReadyCount,omitempty"`

	// ReadyCount of Ironic Conductor instance
	IronicConductorReadyCount map[string]int32 `json:"ironicConductorReadyCount,omitempty"`

	// ReadyCount of Ironic Inspector instance
	InspectorReadyCount int32 `json:"ironicInspectorReadyCount,omitempty"`

	// ReadyCount of Ironic Neutron Agent instance
	IronicNeutronAgentReadyCount int32 `json:"ironicNeutronAgentReadyCount,omitempty"`

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

// IsReady - returns true if Ironic is reconciled successfully
func (instance Ironic) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance Ironic) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance Ironic) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance Ironic) RbacResourceName() string {
	return "ironic-" + instance.Name
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize Ironic defaults with them
	imageDefaults := IronicImages{
		API:               util.GetEnvVar("IRONIC_API_IMAGE_URL_DEFAULT", IronicAPIContainerImage),
		Conductor:         util.GetEnvVar("IRONIC_CONDUCTOR_IMAGE_URL_DEFAULT", IronicConductorContainerImage),
		Inspector:         util.GetEnvVar("IRONIC_INSPECTOR_IMAGE_URL_DEFAULT", IronicInspectorContainerImage),
		Pxe:               util.GetEnvVar("IRONIC_PXE_IMAGE_URL_DEFAULT", IronicPXEContainerImage),
		NeutronAgent:      util.GetEnvVar("IRONIC_NEUTRON_AGENT_IMAGE_URL_DEFAULT", IronicNeutronAgentContainerImage),
		IronicPythonAgent: util.GetEnvVar("IRONIC_PYTHON_AGENT_IMAGE_URL_DEFAULT", IronicPythonAgentContainerImage),
	}

	SetupIronicImageDefaults(imageDefaults)
}
