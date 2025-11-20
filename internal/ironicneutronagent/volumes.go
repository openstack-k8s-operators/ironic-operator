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
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(name string) []corev1.Volume {
	var config0640AccessMode int32 = 0640

	return []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0640AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
	}

}

// GetVolumeMounts - IronicNeutronAgent VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "ironic-neutron-agent-config.json",
			ReadOnly:  true,
		},
	}
}
