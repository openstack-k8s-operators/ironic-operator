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
	"fmt"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IronicAPISpec defines the desired state of IronicAPI
type IronicAPISpec struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Whether to deploy a standalone Ironic.
	Standalone bool `json:"standalone"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// ServiceUser - optional username used for this service to register in ironic
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// ContainerImage - Ironic API Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// Replicas - Ironic API Replicas
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// DatabaseHostname - Ironic Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// +kubebuilder:validation:Optional
	// Secret containing OpenStack password information for IronicDatabasePassword, AdminPassword
	Secret string `json:"secret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: IronicDatabasePassword, service: IronicPassword}
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
	// Secret containing RabbitMq transport URL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=json-rpc
	// RPC transport type - Which RPC transport implementation to use between
	// conductor and API services. 'oslo' to use oslo.messaging transport
	// or 'json-rpc' to use JSON RPC transport. NOTE -> ironic-inspector
	// requires oslo.messaging transport when not in standalone mode.
	RPCTransport string `json:"rpcTransport"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments,omitempty"`

	// +kubebuilder:validation:Optional
	// ExternalEndpoints, expose a VIP using a pre-created IPAddressPool
	ExternalEndpoints []MetalLBConfig `json:"externalEndpoints,omitempty"`

	// +kubebuilder:validation:Required
	// ServiceAccount - service account name used internally to provide the default SA name
	ServiceAccount string `json:"serviceAccount"`
}

// MetalLBConfig to configure the MetalLB loadbalancer service
type MetalLBConfig struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=internal;public
	// Endpoint, OpenStack endpoint this service maps to
	Endpoint endpoint.Endpoint `json:"endpoint"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// IPAddressPool expose VIP via MetalLB on the IPAddressPool
	IPAddressPool string `json:"ipAddressPool"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	// SharedIP if true, VIP/VIPs get shared with multiple services
	SharedIP bool `json:"sharedIP"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	// SharedIPKey specifies the sharing key which gets set as the annotation on the LoadBalancer service.
	// Services which share the same VIP must have the same SharedIPKey. Defaults to the IPAddressPool if
	// SharedIP is true, but no SharedIPKey specified.
	SharedIPKey string `json:"sharedIPKey"`

	// +kubebuilder:validation:Optional
	// LoadBalancerIPs, request given IPs from the pool if available. Using a list to allow dual stack (IPv4/IPv6) support
	LoadBalancerIPs []string `json:"loadBalancerIPs,omitempty"`
}

// IronicAPIStatus defines the observed state of IronicAPI
type IronicAPIStatus struct {
	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]map[string]string `json:"apiEndpoints,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ReadyCount of ironic API instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// ServiceIDs
	ServiceIDs map[string]string `json:"serviceIDs,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// IronicAPI is the Schema for the ironicapis API
type IronicAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IronicAPISpec   `json:"spec,omitempty"`
	Status IronicAPIStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IronicAPIList contains a list of IronicAPI
type IronicAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IronicAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IronicAPI{}, &IronicAPIList{})
}

// IsReady - returns true if service is ready to server requests
func (instance IronicAPI) IsReady() bool {
	return instance.Status.ReadyCount >= 1
}

// GetEndpoint - returns the Ironic endpoint url for type
func (instance IronicAPI) GetEndpoint(endpointType endpoint.Endpoint) (string, error) {
	if endpoints, found := instance.Status.APIEndpoints["ironic"]; found {
		if url, found := endpoints[string(endpointType)]; found {
			return url, nil
		}
	}
	return "", fmt.Errorf("%s endpoint not found", string(endpointType))
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
