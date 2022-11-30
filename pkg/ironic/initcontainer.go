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
	ContainerImage       string
	PxeContainerImage    string
	DatabaseHost         string
	DatabaseUser         string
	DatabaseName         string
	TransportURL         string
	OSPSecret            string
	DBPasswordSelector   string
	UserPasswordSelector string
	VolumeMounts         []corev1.VolumeMount
	Privileged           bool
	PxeInit              bool
	ConductorInit        bool
	DeployHTTPURL        string
}

const (
	// InitContainerCommand -
	InitContainerCommand = "/usr/local/bin/container-scripts/init.sh"

	// PxeInitContainerCommand -
	PxeInitContainerCommand = "/usr/local/bin/container-scripts/pxe-init.sh"

	// ConductorInitContainerCommand -
	ConductorInitContainerCommand = "/usr/local/bin/container-scripts/conductor-init.sh"
)

// InitContainer - init container for Ironic pods
func InitContainer(init APIDetails) []corev1.Container {
	runAsUser := int64(0)
	trueVar := true

	securityContext := &corev1.SecurityContext{
		RunAsUser: &runAsUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{
				"MKNOD",
			},
		},
	}

	if init.Privileged {
		securityContext.Privileged = &trueVar
	}

	envVars := map[string]env.Setter{}
	envVars["DatabaseHost"] = env.SetValue(init.DatabaseHost)
	envVars["DatabaseUser"] = env.SetValue(init.DatabaseUser)
	envVars["DatabaseName"] = env.SetValue(init.DatabaseName)
	envVars["DeployHTTPURL"] = env.SetValue(init.DeployHTTPURL)

	envs := []corev1.EnvVar{
		{
			Name: "DatabasePassword",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: init.OSPSecret,
					},
					Key: init.DBPasswordSelector,
				},
			},
		},
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
			Name: "NodeName",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
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
	if init.PxeInit {
		pxeInit := corev1.Container{
			Name:  "pxe-init",
			Image: init.PxeContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
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
	if init.ConductorInit {
		priv := true
		conductorInit := corev1.Container{
			Name:  "conductor-init",
			Image: init.ContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  &runAsUser,
				Privileged: &priv, // required for building esp.img (mount)
				// TODO: try again to get this working with Capability SYS_ADMIN
				// instead of privileged
			},
			Command: []string{
				"/bin/bash",
			},
			Args: []string{
				"-c",
				ConductorInitContainerCommand,
			},
			Env:          envs,
			VolumeMounts: init.VolumeMounts,
		}
		containers = append(containers, conductorInit)
	}
	return containers
}
