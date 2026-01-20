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
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IronicConductorTemplate defines the input parameters for Ironic Conductor service
type IronicConductorTemplate struct {
	// Common input parameters for all Ironic services
	IronicServiceTemplate `json:",inline"`

	// +kubebuilder:validation:Optional
	// ConductorGroup - Ironic Conductor conductor group.
	ConductorGroup string `json:"conductorGroup"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=""
	// StorageClass
	StorageClass string `json:"storageClass"`

	// +kubebuilder:validation:Required
	// StorageRequest
	StorageRequest string `json:"storageRequest"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments,omitempty"`

	// +kubebuilder:validation:Optional
	// ProvisionNetwork - Additional network to attach to expose boot DHCP, TFTP, HTTP services.
	ProvisionNetwork string `json:"provisionNetwork,omitempty"`

	// +kubebuilder:validation:Optional
	// DHCPRanges - List of DHCP ranges to use for provisioning
	DHCPRanges []DHCPRange `json:"dhcpRanges,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=120
	// +kubebuilder:validation:Minimum=30
	// terminationGracePeriodSeconds - Pod termination grace period in seconds.
	// Controls how long to wait for conductor pods to shut down gracefully before they are forcefully terminated.
	// The default value is long enough for periodic tasks to complete but not for long-running tasks (deployment, cleaning).
	// A node reserved for a long running task will be put into a failed state when the conductor reserving it is forcefully terminated.
	// The terminationGracePeriodSeconds should be chosen with this and maintenance processes in mind.
	// Changing terminationGracePeriodSeconds results in conductor pods being restarted with the old value.
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds"`
}

// IronicConductorSpec defines the desired state of IronicConductor
type IronicConductorSpec struct {
	// Input parameters for the Ironic Conductor service
	IronicConductorTemplate `json:",inline"`

	// +kubebuilder:validation:Optional
	// ContainerImage - Ironic Conductor Container Image
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Whether to deploy a standalone Ironic.
	Standalone bool `json:"standalone"`

	// +kubebuilder:validation:Optional
	// PxeContainerImage - Ironic DHCP/TFTP/HTTP Container Image
	PxeContainerImage string `json:"pxeContainerImage"`

	// +kubebuilder:validation:Optional
	// IronicPythonAgentImage - Image containing the ironic-python-agent kernel and ramdisk
	IronicPythonAgentImage string `json:"ironicPythonAgentImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// ServiceUser - optional username used for this service to register in ironic
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// Secret containing OpenStack password information for AdminPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: IronicPassword}
	// PasswordSelectors - Selectors to identify the ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Required
	// DatabaseHostname - Ironic Database Hostname
	DatabaseHostname string `json:"databaseHostname"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// DatabaseAccount - optional MariaDBAccount used for ironic DB, defaults to ironic.
	DatabaseAccount string `json:"databaseAccount"`

	// +kubebuilder:validation:Optional
	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:=oslo;json-rpc
	// +kubebuilder:default=json-rpc
	// RPC transport type - Which RPC transport implementation to use between
	// conductor and API services. 'oslo' to use oslo.messaging transport
	// or 'json-rpc' to use JSON RPC transport. NOTE -> ironic requires
	// oslo.messaging transport when not in standalone mode.
	RPCTransport string `json:"rpcTransport"`

	// +kubebuilder:validation:Optional
	// KeystoneEndpoints - Internally used Keystone API endpoints
	KeystoneEndpoints KeystoneEndpoints `json:"keystoneEndpoints"`

	// +kubebuilder:validation:Optional
	// Region - OpenStack region name
	Region string `json:"region,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS tls.Ca `json:"tls,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Disabled
	// +kubebuilder:validation:Enum:=Enabled;Disabled;""
	// Whether to enable graphical consoles. NOTE: Setting this to Enabled is not supported.
	GraphicalConsoles string `json:"graphicalConsoles"`

	// +kubebuilder:validation:Optional
	// NoVNCProxyImage - Ironic NoVNCProxy Container Image
	NoVNCProxyImage string `json:"novncproxyImage,omitempty"`

	// +kubebuilder:validation:Optional
	// ConsoleImage - Ironic Graphical Console Container Image
	ConsoleImage string `json:"consoleImage"`
}

// IronicConductorStatus defines the observed state of IronicConductor
type IronicConductorStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount of ironic Conductor instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the openstack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastAppliedTopology - the last applied Topology
	LastAppliedTopology *topologyv1.TopoRef `json:"lastAppliedTopology,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
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

// IsReady - returns true if IronicConductor is reconciled successfully
func (instance IronicConductor) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance IronicConductor) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance IronicConductor) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance IronicConductor) RbacResourceName() string {
	owningIronicName := GetOwningIronicName(&instance)
	if owningIronicName != "" {
		return "ironic-" + owningIronicName
	}
	return "ironicconductor-" + instance.Name
}

// GetSpecTopologyRef - Returns the LastAppliedTopology Set in the Status
func (instance *IronicConductor) GetSpecTopologyRef() *topologyv1.TopoRef {
	return instance.Spec.TopologyRef
}

// GetLastAppliedTopology - Returns the LastAppliedTopology Set in the Status
func (instance *IronicConductor) GetLastAppliedTopology() *topologyv1.TopoRef {
	return instance.Status.LastAppliedTopology
}

// SetLastAppliedTopology - Sets the LastAppliedTopology value in the Status
func (instance *IronicConductor) SetLastAppliedTopology(topologyRef *topologyv1.TopoRef) {
	instance.Status.LastAppliedTopology = topologyRef
}
