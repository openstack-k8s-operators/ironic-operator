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
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IronicInspectorPasswordSelector to identify the DB and AdminUser password from the Secret
type IronicInspectorPasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="IronicInspectorDatabasePassword"
	// Database - Selector to get the ironic-inspector Database user password from the Secret
	// TODO: not used, need change in mariadb-operator
	Database string `json:"database"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="IronicInspectorPassword"
	// Service - Selector to get the ironic-inspector service password from the Secret
	Service string `json:"service"`
}

// IronicInspectorTemplate defines the input parameters for Ironic Inspector service
type IronicInspectorTemplate struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic-inspector
	// ServiceUser - optional username used for this service to register in ironic-inspector
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// Replicas - Ironic Inspector Replicas
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: IronicInspectorDatabasePassword, service: IronicInspectorPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password from the Secret
	PasswordSelectors IronicInspectorPasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting here overrides
	// any global NodeSelector settings within the Ironic CR
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

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
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments,omitempty"`

	// +kubebuilder:validation:Optional
	// InspectionNetwork - Additional network to attach to expose boot DHCP, TFTP, HTTP services.
	InspectionNetwork string `json:"inspectionNetwork,omitempty"`

	// +kubebuilder:validation:Optional
	// DHCPRanges - List of DHCP ranges to use for provisioning
	DHCPRanges []DHCPRange `json:"dhcpRanges,omitempty"`

	// +kubebuilder:validation:Optional
	// Override, provides the ability to override the generated manifest of several child resources.
	Override InspectorOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS tls.API `json:"tls,omitempty"`
}

// InspectorOverrideSpec to override the generated manifest of several child resources.
type InspectorOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	// The key must be the endpoint type (public, internal)
	Service map[service.Endpoint]service.RoutedOverrideSpec `json:"service,omitempty"`
}

// IronicInspectorSpec defines the desired state of IronicInspector
type IronicInspectorSpec struct {
	// Input parameters for the Ironic Inspector service
	IronicInspectorTemplate `json:",inline"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Standalone - Whether to deploy a standalone Ironic Inspector.
	Standalone bool `json:"standalone"`

	// +kubebuilder:validation:Optional
	// ContainerImage - Ironic Inspector Container Image
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// PxeContainerImage - Ironic Inspector DHCP/TFTP/HTTP Container Image
	PxeContainerImage string `json:"pxeContainerImage"`

	// +kubebuilder:validation:Optional
	// IronicPythonAgentImage - Image containing the ironic-python-agent kernel and ramdisk
	IronicPythonAgentImage string `json:"ironicPythonAgentImage"`

	// +kubebuilder:validation:Optional
	// MariaDB instance name.
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB.
	// Might not be required in future.
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// Secret containing OpenStack password information for IronicInspectorDatabasePassword, AdminPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Ironic Inspector
	RabbitMqClusterName string `json:"rabbitMqClusterName"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:=oslo;json-rpc
	// +kubebuilder:default=json-rpc
	// RPC transport type - Which RPC transport implementation to use between
	// conductor and API services. 'oslo' to use oslo.messaging transport
	// or 'json-rpc' to use JSON RPC transport. NOTE -> ironic-inspector
	// requires oslo.messaging transport when not in standalone mode.
	RPCTransport string `json:"rpcTransport"`
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

	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

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

// IsReady - returns true if IronicInspector is reconciled successfully
func (instance IronicInspector) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance IronicInspector) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance IronicInspector) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance IronicInspector) RbacResourceName() string {
	owningIronicName := GetOwningIronicName(&instance)
	if owningIronicName != "" {
		return "ironic-" + owningIronicName
	}
	return "ironicinspector-" + instance.Name
}
