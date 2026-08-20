package ironic

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(name string) []corev1.Volume {
	var scriptsVolumeDefaultMode int32 = 0755
	var config0440AccessMode int32 = 0440

	return []corev1.Volume{
		{
			Name: "scripts",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					SecretName:  name + "-scripts",
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0440AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		volume.WritableDirVolume("config-data-merged"),
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

// GetVolumeMounts - Ironic VolumeMounts. Note: does NOT mount
// "config-data-merged" as a whole directory -- only GetInitVolumeMounts()
// (the init container that actually writes into it via crudini-merge)
// needs that. Consumers that need one specific merged file mount it
// directly at its final destination via SubPath (see
// GetMergedConfVolumeMount()) once the init container has already run.
func GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "etc-podinfo",
			MountPath: "/etc/podinfo",
			ReadOnly:  false,
		},
	}
}

// GetMergedConfVolumeMount - a single file out of the "config-data-merged"
// EmptyDir, SubPath-mounted directly at its final destination. Safe despite
// being a SubPath mount of an EmptyDir: the init container (an earlier
// container in the same pod) already wrote the real file there through its
// own *whole-directory* mount of the same EmptyDir (GetInitVolumeMounts()),
// so by the time any of these later containers start, the file already
// exists -- see horizon-operator's equivalent fix for why a SubPath mount
// of a *not-yet-existing* path would otherwise be auto-created by kubelet
// as a directory.
func GetMergedConfVolumeMount(finalPath, subPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "config-data-merged",
		MountPath: finalPath,
		SubPath:   subPath,
		ReadOnly:  true,
	}
}

// GetDBSyncVolumeMounts - Ironic db-sync VolumeMounts. Sources ironic.conf/
// 02-ironic-custom.conf/my.cnf from the merged EmptyDir (matching what
// db-sync-config.json's kolla copy step used to read from), not from the
// raw "config-data"/"config-data-custom" Secrets directly -- db-sync always
// ran through the merge step, even though it doesn't need
// 03-init-container-conductor.conf (conductor-only).
func GetDBSyncVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		GetMergedConfVolumeMount("/etc/ironic/ironic.conf", "ironic.conf"),
		GetMergedConfVolumeMount("/etc/ironic/ironic.conf.d/02-ironic-custom.conf", "02-ironic-custom.conf"),
		GetMergedConfVolumeMount("/etc/my.cnf", "my.cnf"),
	}

	return append(GetVolumeMounts(), volumeMounts...)
}
