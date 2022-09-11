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
	trueVar := true

	securityContext := &corev1.SecurityContext{
		RunAsUser: &runAsUser,
	}

	if init.Privileged {
		securityContext.Privileged = &trueVar
	}

	envVars := map[string]env.Setter{}
	envVars["DatabaseHost"] = env.SetValue(init.DatabaseHost)
	envVars["DatabaseUser"] = env.SetValue(init.DatabaseUser)
	envVars["DatabaseName"] = env.SetValue(init.DatabaseName)

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
	}
	envs = env.MergeEnvs(envs, envVars)

	containers := []corev1.Container{
		{
			Name:            "init",
			Image:           init.ContainerImage,
			SecurityContext: securityContext,
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
			Name:            "pxe-init",
			Image:           init.PxeContainerImage,
			SecurityContext: securityContext,
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
