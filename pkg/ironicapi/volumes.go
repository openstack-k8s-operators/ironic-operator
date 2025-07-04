package ironicapi

import (
	"fmt"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(instance *ironicv1.IronicAPI) []corev1.Volume {
	var config0640AccessMode int32 = 0640
	parentName := ironicv1.GetOwningIronicName(instance)

	var apiVolumes []corev1.Volume

	if parentName == "" {
		// TODO: Add proper logging
		fmt.Println("parentName is not present")
		// Only include logs volume when parentName is not present
		apiVolumes = append(apiVolumes,
			corev1.Volume{
				Name: "logs",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
				},
			})
	} else {
		// Include both volumes when parentName is present
		apiVolumes = append(apiVolumes,
			corev1.Volume{
				Name: "config-data-custom",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						DefaultMode: &config0640AccessMode,
						SecretName:  fmt.Sprintf("%s-config-data", parentName),
					},
				},
			},
			corev1.Volume{
				Name: "logs",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
				},
			})
	}

	return append(ironic.GetVolumes(instance.Name), apiVolumes...)
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
func GetInitVolumeMounts(instance *ironicv1.IronicAPI) []corev1.VolumeMount {
	parentName := ironicv1.GetOwningIronicName(instance)

	var initVolumeMounts []corev1.VolumeMount

	// Only include config-data-custom volume mount when parentName is present
	if parentName != "" {
		initVolumeMounts = append(initVolumeMounts,
			corev1.VolumeMount{
				Name:      "config-data-custom",
				MountPath: "/var/lib/config-data/custom",
				ReadOnly:  true,
			})
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
