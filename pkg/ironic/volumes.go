package ironic

import (
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(name string) []corev1.Volume {
	var scriptsVolumeDefaultMode int32 = 0755
	var config0640AccessMode int32 = 0640

	return []corev1.Volume{
		{
			Name: "scripts",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-scripts",
					},
				},
			},
		},
		{
			Name: "config-data",
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
			Name: "config-data-merged",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
		{
			Name: "etc-podinfo",
			VolumeSource: corev1.VolumeSource{
				DownwardAPI: &corev1.DownwardAPIVolumeSource{
					Items: []corev1.DownwardAPIVolumeFile{
						{
							Path: "network-status",
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "metadata.annotations['k8s.v1.cni.cncf.io/network-status']",
							},
						},
					},
				},
			},
		},
	}

}

// GetInitVolumeMounts - Ironic init task VolumeMounts
func GetInitVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      "config-data-merged",
			MountPath: "/var/lib/config-data/merged",
			ReadOnly:  false,
		},
		{
			Name:      "etc-podinfo",
			MountPath: "/etc/podinfo",
			ReadOnly:  false,
		},
	}

}

// GetVolumeMounts - Ironic VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "config-data-merged",
			MountPath: "/var/lib/config-data/merged",
			ReadOnly:  false,
		},
		{
			Name:      "etc-podinfo",
			MountPath: "/etc/podinfo",
			ReadOnly:  false,
		},
	}

}
