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
	corev1 "k8s.io/api/core/v1"
)

// IronicServiceTemplate defines the common input parameters for Ironic services
type IronicServiceTemplate struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas -
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service. Setting here overrides
	// any global NodeSelector settings within the Ironic CR
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

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
}

// PasswordSelector to identify the AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="IronicPassword"
	// Service - Selector to get the ironic service password from the Secret
	Service string `json:"service"`
}

// KeystoneEndpoints defines keystone endpoint parameters for service
type KeystoneEndpoints struct {
	// +kubebuilder:validation:Optional
	// Internal endpoint URL
	Internal string `json:"internal"`
	// +kubebuilder:validation:Optional
	// Public endpoint URL
	Public string `json:"public"`
}

// HttpdCustomization - customize the httpd service
type HttpdCustomization struct {
	// +kubebuilder:validation:Optional
	// CustomConfigSecret - customize the httpd vhost config using this parameter to specify
	// a secret that contains service config data. The content of each provided snippet gets
	// rendered as a go template and placed into /etc/httpd/conf/httpd_custom_<key> .
	// In the default httpd template at the end of the vhost those custom configs get
	// included using `Include conf/httpd_custom_<endpoint>_*`.
	// For information on how sections in httpd configuration get merged, check section
	// "How the sections are merged" in https://httpd.apache.org/docs/current/sections.html#merging
	CustomConfigSecret *string `json:"customConfigSecret,omitempty"`
}
