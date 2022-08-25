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
	"fmt"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"

	// BootstrapHash completed
	BootstrapHash = "bootstrap"
)

// IronicAPISpec defines the desired state of IronicAPI
type IronicAPISpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:default=true
	// Whether to deploy a single node standalone Ironic.
	// TODO: -> not implemented, always standalone for now
	Standalone bool `json:"standalone,omitempty"`

	// +kubebuilder:validation:Required
	// MariaDB instance name.
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB.
	// Might not be required in future.
	DatabaseInstance string `json:"databaseInstance,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=ironic
	// DatabaseUser - optional username used for ironic DB, defaults to ironic.
	// TODO: -> implement needs work in mariadb-operator, right now only ironic.
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Required
	// Ironic Container Image URLs
	ContainerImages ContainerImages `json:"containerImages,omitempty"`

	// +kubebuilder:validation:Optional
	// PasswordSelectors - Selectors to identify the DB and AdminUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors,omitempty"`

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
	// ConfigOverwrite - interface to overwrite default config files like e.g. logging.conf or policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ContainerImages to identify the container image URLs
type ContainerImages struct {

	// +kubebuilder:validation:Required
	// API Container Image URL
	API string `json:"API,omitempty"`

	// +kubebuilder:validation:Required
	// Conductor Container Image URL
	Conductor string `json:"Conductor,omitempty"`

	// +kubebuilder:validation:Required
	// Inspector Container Image URL
	Inspector string `json:"Inspector,omitempty"`

	// +kubebuilder:validation:Required
	// PXE Container Image URL
	PXE string `json:"PXE,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="IronicDatabasePassword"
	// Database - Selector to get the ironic Database user password from the Secret
	// TODO: not used, need change in mariadb-operator
	Database string `json:"database,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="AdminPassword"
	// Database - Selector to get the ironic Database user password from the Secret
	Admin string `json:"admin,omitempty"`
}

// IronicDebug defines the observed state of IronicAPI
type IronicDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// DBSync enable debug
	DBSync bool `json:"dbSync,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// ReadyCount enable debug
	Bootstrap bool `json:"bootstrap,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Service enable debug
	Service bool `json:"service,omitempty"`
}

// IronicAPIStatus defines the observed state of IronicAPI
type IronicAPIStatus struct {
	// ReadyCount of ironic API instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// API endpoint
	APIEndpoints map[string]string `json:"apiEndpoint,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Ironic Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
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

// GetEndpoint - returns OpenStack endpoint url for type
func (instance IronicAPI) GetEndpoint(endpointType endpoint.Endpoint) (string, error) {
	if url, found := instance.Status.APIEndpoints[string(endpointType)]; found {
		return url, nil
	}
	return "", fmt.Errorf("%s endpoint not found", string(endpointType))
}

// IsReady - returns true if service is ready to server requests
func (instance IronicAPI) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ExposeServiceReadyCondition) &&
		instance.Status.Conditions.IsTrue(condition.DeploymentReadyCondition)
}
