/*

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

package ironic

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"

	corev1 "k8s.io/api/core/v1"
)

// APIDetails information
type APIDetails struct {
	ContainerImage         string
	PxeContainerImage      string
	IronicPythonAgentImage string
	DatabaseHost           string
	DatabaseName           string
	TransportURLSecret     string
	OSPSecret              string
	UserPasswordSelector   string
	VolumeMounts           []corev1.VolumeMount
	Privileged             bool
	PxeInit                bool
	ConductorInit          bool
	DeployHTTPURL          string
	IngressDomain          string
	ProvisionNetwork       string
	ImageDirectory         string
}

const (
	// InitContainerCommand -
	InitContainerCommand = "/usr/local/bin/container-scripts/init.sh"

	// PxeInitContainerCommand -
	PxeInitContainerCommand = "/usr/local/bin/container-scripts/pxe-init.sh"
)

// InitContainer - init container for Ironic pods
func InitContainer(init APIDetails) []corev1.Container {
	runAsUser := int64(0)

	envVars := map[string]env.Setter{}
	envVars["DatabaseHost"] = env.SetValue(init.DatabaseHost)
	envVars["DatabaseName"] = env.SetValue(init.DatabaseName)
	envVars["DeployHTTPURL"] = env.SetValue(init.DeployHTTPURL)
	envVars["IngressDomain"] = env.SetValue(init.IngressDomain)

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
			Name:  "ProvisionNetwork",
			Value: init.ProvisionNetwork,
		},
	}
	if init.TransportURLSecret != "" {
		envTransport := corev1.EnvVar{
			Name: "TransportURL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: init.TransportURLSecret,
					},
					Key: "transport_url",
				},
			},
		}
		envs = append(envs, envTransport)

		envQuorumQueues := corev1.EnvVar{
			Name: "QuorumQueues",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: init.TransportURLSecret,
					},
					Key:      "quorumqueues",
					Optional: &[]bool{true}[0],
				},
			},
		}
		envs = append(envs, envQuorumQueues)
	}
	envs = env.MergeEnvs(envs, envVars)
	imageCopyEnvs := []corev1.EnvVar{
		{
			Name:  "DEST_DIR",
			Value: init.ImageDirectory,
		},
	}

	var containers []corev1.Container

	initContainer := corev1.Container{
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
	}
	containers = append(containers, initContainer)

	if init.ConductorInit {
		ipaInit := corev1.Container{
			Name:  "ironic-python-agent-init",
			Image: init.IronicPythonAgentImage,
			SecurityContext: &corev1.SecurityContext{
				Privileged: &init.Privileged,
			},
			Env:          imageCopyEnvs,
			VolumeMounts: init.VolumeMounts,
		}
		containers = append(containers, ipaInit)
	}

	if init.PxeInit {
		pxeInit := corev1.Container{
			Name:  "pxe-init",
			Image: init.PxeContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  &runAsUser,
				Privileged: &init.Privileged,
			},
			Command: []string{
				"/bin/bash",
			},
			Args: []string{
				"-c",
				PxeInitContainerCommand,
			},
			Env:          envs,
			VolumeMounts: init.VolumeMounts,
		}
		containers = append(containers, pxeInit)
	}

	return containers
}
