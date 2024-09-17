package ironicapi

import (
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(name string) []corev1.Volume {
	var config0640AccessMode int32 = 0640

	apiVolumes := []corev1.Volume{
		{
			Name: "config-data-custom",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0640AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		{
			Name: "logs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
	}

	return append(ironic.GetVolumes(name), apiVolumes...)
}

// GetLogVolumeMount - Ironic API LogVolumeMount
func GetLogVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "logs",
		MountPath: "/var/log/ironic",
		ReadOnly:  false,
	}
}

// GetInitVolumeMounts - Ironic API init task VolumeMounts
func GetInitVolumeMounts() []corev1.VolumeMount {

	initVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-data-custom",
			MountPath: "/var/lib/config-data/custom",
			ReadOnly:  true,
		},
	}

	return append(ironic.GetInitVolumeMounts(), initVolumeMounts...)
}

// GetVolumeMounts - Ironic API VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   "ironic-api-config.json",
			ReadOnly:  true,
		},
		GetLogVolumeMount(),
	}

	return append(ironic.GetVolumeMounts(), volumeMounts...)
}
