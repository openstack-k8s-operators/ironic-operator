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
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(name string) []corev1.Volume {
	var config0440AccessMode int32 = 0440

	return []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0440AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		volume.WritableDirVolume("var-log-neutron"),
	}

}

// GetVolumeMounts - IronicNeutronAgent VolumeMounts. Each file mounted
// directly at its final destination via SubPath from the same "config"
// Secret config.json used to stage-then-copy.
func GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/etc/neutron/neutron.conf.d/01-ironic_neutron_agent.conf",
			SubPath:   "01-ironic_neutron_agent.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/neutron/neutron.conf.d/02-ironic_neutron_agent-custom.conf",
			SubPath:   "02-ironic_neutron_agent-custom.conf",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount("var-log-neutron", "/var/log/neutron"),
	}
}
