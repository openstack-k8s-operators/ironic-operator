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
	"fmt"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// IronicNeutronAgentTemplate defines the input parameters for ML2 baremetal - ironic-neutron-agent agents
type IronicNeutronAgentTemplate struct {
	// Common input parameters for all Ironic services
	IronicServiceTemplate `json:",inline"`

	// +kubebuilder:validation:Optional
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Ironic
	// Deprecated: Use MessagingBus.Cluster instead
	RabbitMqClusterName string `json:"rabbitMqClusterName,omitempty" deprecated:"messagingBus.cluster"`

	// +kubebuilder:validation:Optional
	// MessagingBus configuration (username, vhost, and cluster) for RPC messaging
	MessagingBus rabbitmqv1.RabbitMqConfig `json:"messagingBus,omitempty"`

	// +kubebuilder:validation:Optional
	// NotificationsBus configuration (username, vhost, and cluster) for notifications
	NotificationsBus *rabbitmqv1.RabbitMqConfig `json:"notificationsBus,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS IronicNeutronAgentTLS `json:"tls,omitempty"`

	// ExtraMounts containing conf files
	// +kubebuilder:validation:Optional
	ExtraMounts []IronicNeutronAgentExtraVolMounts `json:"extraMounts,omitempty"`
}

// IronicNeutronAgentTLS defines the TLS input parameters
type IronicNeutronAgentTLS struct {
	// +kubebuilder:validation:Optional
	// Secret containing CA bundle
	tls.Ca `json:",inline"`

	// +kubebuilder:validation:Optional
	// Ovn holds the secret for the OVN DB client cert
	Ovn tls.GenericService `json:"ovn,omitempty"`
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
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// Auth - Parameters related to authentication (inherited from parent Ironic CR)
	Auth AuthSpec `json:"auth,omitempty"`
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

	// NotificationsURLSecret - Secret containing RabbitMQ notifications URL
	NotificationsURLSecret *string `json:"notificationsURLSecret,omitempty"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the openstack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// AppliedInputSecretHash is the hash of the input Secret consumed by the
	// currently ready workload.
	AppliedInputSecretHash string `json:"appliedInputSecretHash,omitempty"`

	// LastAppliedTopology - the last applied Topology
	LastAppliedTopology *topologyv1.TopoRef `json:"lastAppliedTopology,omitempty"`
}

// Default implements webhook defaulting
func (spec *IronicNeutronAgentTemplate) Default() {
	// Default MessagingBus.Cluster if not set
	// Migration from deprecated RabbitMqClusterName is handled by openstack-operator
	rabbitmqv1.DefaultRabbitMqConfig(&spec.MessagingBus, "rabbitmq")
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

// IronicNeutronAgentExtraVolMounts exposes additional parameters processed by
// the ironic-operator and defines the common VolMounts structure provided by
// the main storage module
type IronicNeutronAgentExtraVolMounts struct {
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`
	// +kubebuilder:validation:Required
	VolMounts []storage.VolMounts `json:"extraVol"`
}

// Volume names owned by the operator in the ironic-neutron-agent pod.
// Submitting an extraMounts volume with one of these names would create
// duplicate PodSpec volume names, which Kubernetes rejects. They are
// therefore reserved and rejected at admission.
const (
	// ConfigVolumeName is the volume name of the rendered config-data Secret
	ConfigVolumeName = "config"
	// LogsVolumeName is the volume name of the writable log directory
	LogsVolumeName = "var-log-neutron"
	// OvnDBCertVolumeName is the volume name of the OVN DB client
	// certificate. It matches the name derived by
	// tls.Service.CreateVolume("ovndb") when the OVN DB client cert is
	// enabled in the deployment.
	OvnDBCertVolumeName = "ovndb-tls-certs"
)

// isReservedVolumeName reports whether name is reserved for a volume owned
// by the operator in the ironic-neutron-agent pod. The names are reserved
// unconditionally, even when the corresponding volume is not currently
// added to the pod, so that enabling TLS later cannot break the Deployment.
func isReservedVolumeName(name string) bool {
	switch name {
	case ConfigVolumeName, LogsVolumeName, OvnDBCertVolumeName, tls.CABundleLabel:
		return true
	}
	return false
}

// ValidateExtraMounts - rejects extraMounts volumes whose name collides with
// a volume owned by the operator in the ironic-neutron-agent pod or with
// another extraMounts volume, since duplicate PodSpec volume names make the
// Deployment unschedulable.
func (spec *IronicNeutronAgentTemplate) ValidateExtraMounts(basePath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	seen := map[string]bool{}
	for i, em := range spec.ExtraMounts {
		emPath := basePath.Child("extraMounts").Index(i)
		for j, vm := range em.VolMounts {
			volumesPath := emPath.Child("extraVol").Index(j).Child("volumes")
			for k, vol := range vm.Volumes {
				volPath := volumesPath.Index(k).Child("name")
				switch {
				case isReservedVolumeName(vol.Name):
					allErrs = append(allErrs, field.Invalid(
						volPath, vol.Name,
						fmt.Sprintf("volume name %q is reserved for an operator-owned volume of the ironic-neutron-agent pod", vol.Name),
					))
				case seen[vol.Name]:
					allErrs = append(allErrs, field.Duplicate(volPath, vol.Name))
				default:
					seen[vol.Name] = true
				}
			}
		}
	}

	return allErrs
}

// Propagate is a function used to filter VolMounts according to the specified
// PropagationType array
func (c *IronicNeutronAgentExtraVolMounts) Propagate(svc []storage.PropagationType) []storage.VolMounts {
	var vl []storage.VolMounts

	for _, gv := range c.VolMounts {
		vl = append(vl, gv.Propagate(svc)...)
	}

	return vl
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

// GetSpecTopologyRef - Returns the LastAppliedTopology Set in the Status
func (instance *IronicNeutronAgent) GetSpecTopologyRef() *topologyv1.TopoRef {
	return instance.Spec.TopologyRef
}

// GetLastAppliedTopology - Returns the LastAppliedTopology Set in the Status
func (instance *IronicNeutronAgent) GetLastAppliedTopology() *topologyv1.TopoRef {
	return instance.Status.LastAppliedTopology
}

// SetLastAppliedTopology - Sets the LastAppliedTopology value in the Status
func (instance *IronicNeutronAgent) SetLastAppliedTopology(topologyRef *topologyv1.TopoRef) {
	instance.Status.LastAppliedTopology = topologyRef
}
