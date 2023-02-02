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

package ironicneutronagent

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"

	corev1 "k8s.io/api/core/v1"
)

const (
	// InitContainerCommand -
	InitContainerCommand = "/usr/local/bin/container-scripts/init.sh"
)

// APIDetails information
type APIDetails struct {
	ContainerImage       string
	TransportURLSecret   string
	OSPSecret            string
	UserPasswordSelector string
	VolumeMounts         []corev1.VolumeMount
}

// InitContainer - init container for IronicNeutronAgent pods
func InitContainer(init APIDetails) []corev1.Container {
	runAsUser := int64(0)

	envVars := map[string]env.Setter{}

	envs := []corev1.EnvVar{
		{
			Name: "IronicPassword",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: init.OSPSecret,
					},
					Key: init.UserPasswordSelector,
				},
			},
		},
		{
			Name: "PodName",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "PodNamespace",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "PodNetworksStatus",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.annotations['k8s.v1.cni.cncf.io/networks-status']",
				},
			},
		},
		{
			Name: "TransportURL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: init.TransportURLSecret,
					},
					Key: "transport_url",
				},
			},
		},
	}

	envs = env.MergeEnvs(envs, envVars)

	containers := []corev1.Container{
		{
			Name:  "init",
			Image: init.ContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
			},
			Command: []string{
				"/bin/bash",
			},
			Args: []string{
				"-c",
				InitContainerCommand,
			},
			Env:          envs,
			VolumeMounts: init.VolumeMounts,
		},
	}

	return containers
}
