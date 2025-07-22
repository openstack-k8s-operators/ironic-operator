package ironicconductor

import (
	"fmt"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(instance *ironicv1.IronicConductor) []corev1.Volume {
	var config0640AccessMode int32 = 0640
	parentName := ironicv1.GetOwningIronicName(instance)

	var conductorVolumes []corev1.Volume

	// Only include config-data-custom volume when parentName is present
	if parentName != "" {
		conductorVolumes = append(conductorVolumes,
			corev1.Volume{
				Name: "config-data-custom",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						DefaultMode: &config0640AccessMode,
						SecretName:  fmt.Sprintf("%s-config-data", parentName),
					},
				},
			})
	} else {
		// TODO: Add proper logging
		fmt.Println("parentName is not present")
	}

	return append(ironic.GetVolumes(instance.Name), conductorVolumes...)
}

// GetInitVolumeMounts - Ironic Conductor init task VolumeMounts
func GetInitVolumeMounts(instance *ironicv1.IronicConductor) []corev1.VolumeMount {
	parentName := ironicv1.GetOwningIronicName(instance)

	initVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "var-lib-ironic",
			MountPath: "/var/lib/ironic",
			ReadOnly:  false,
		},
	}

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

// GetVolumeMounts - Ironic Conductor VolumeMounts
func GetVolumeMounts(serviceName string) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/var/lib/kolla/config_files/config.json",
			SubPath:   serviceName + "-config.json",
			ReadOnly:  true,
		},
		{
			Name:      "var-lib-ironic",
			MountPath: "/var/lib/ironic",
			ReadOnly:  false,
		},
	}
	return append(ironic.GetVolumeMounts(), volumeMounts...)
}
