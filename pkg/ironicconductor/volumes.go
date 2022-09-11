package ironicconductor

import (
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(parentName string, name string) []corev1.Volume {
	var config0640AccessMode int32 = 0640

	conductorVolumes := []corev1.Volume{
		{
			Name: "config-data-custom",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &config0640AccessMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-config-data",
					},
				},
			},
		},
		{
			Name: "var-lib-ironic",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	return append(ironic.GetVolumes(parentName), conductorVolumes...)
}

// GetInitVolumeMounts - Ironic Conductor init task VolumeMounts
func GetInitVolumeMounts() []corev1.VolumeMount {

	initVolumdMounts := []corev1.VolumeMount{
		{
			Name:      "config-data-custom",
			MountPath: "/var/lib/config-data/custom",
			ReadOnly:  true,
		},
		{
			Name:      "var-lib-ironic",
			MountPath: "/var/lib/ironic",
			ReadOnly:  false,
		},
	}

	return append(ironic.GetInitVolumeMounts(), initVolumdMounts...)
}

// GetVolumeMounts - Ironic Conductor VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	conductorVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "var-lib-ironic",
			MountPath: "/var/lib/ironic",
			ReadOnly:  false,
		},
	}
	return append(ironic.GetVolumeMounts(), conductorVolumeMounts...)
}
