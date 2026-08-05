package ironicinspector

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
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &config0440AccessMode,
					SecretName:  name + "-config-data",
				},
			},
		},
		volume.WritableDirVolume("var-lib-ironic"),
		volume.WritableDirVolume("var-lib-ironic-inspector-dhcp-hostsdir"),
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
		volume.WritableDirVolume(volume.RunHttpdVolumeName),
	}

}

// GetInitVolumeMounts - Ironic Inspector init task VolumeMounts
func GetInitVolumeMounts() []corev1.VolumeMount {

	return []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      "var-lib-ironic",
			MountPath: "/var/lib/ironic",
			ReadOnly:  false,
		},
		{
			Name:      "etc-podinfo",
			MountPath: "/etc/podinfo",
			ReadOnly:  false,
		},
	}

}

// getCommonVolumeMounts - mounts shared by every ironic-inspector container:
// the scripts dir, var-lib-ironic (shared with pxe-init, which writes
// dnsmasq.conf/boot assets there before any of these containers start),
// the dhcp-hostsdir EmptyDir, and podinfo.
func getCommonVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "var-lib-ironic",
			MountPath: "/var/lib/ironic",
			ReadOnly:  false,
		},
		{
			Name:      "var-lib-ironic-inspector-dhcp-hostsdir",
			MountPath: "/var/lib/ironic-inspector/dhcp-hostsdir",
			ReadOnly:  false,
		},
		{
			Name:      "etc-podinfo",
			MountPath: "/etc/podinfo",
			ReadOnly:  false,
		},
	}
}

// GetRunHttpdVolumeMount - writable emptyDir for httpd's PID file directory,
// same fix as ironic-api (httpd.conf's PidFile resolves to
// /etc/httpd/run/httpd.pid, and /etc/httpd/run is a symlink to
// ../../run/httpd in the image).
func GetRunHttpdVolumeMount() corev1.VolumeMount {
	return volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath)
}

// GetHttpdVolumeMounts - the reverse-proxy httpd container (5050 -> 5051).
func GetHttpdVolumeMounts() []corev1.VolumeMount {
	vm := append(getCommonVolumeMounts(),
		corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpd.conf",
			ReadOnly:  true,
		},
		corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		GetRunHttpdVolumeMount(),
	)
	return vm
}

// GetInspectorVolumeMounts - the main ironic-inspector process.
func GetInspectorVolumeMounts() []corev1.VolumeMount {
	return append(getCommonVolumeMounts(),
		corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/ironic-inspector/inspector.conf.d/01-inspector.conf",
			SubPath:   "01-inspector.conf",
			ReadOnly:  true,
		},
		corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/ironic-inspector/inspector.conf.d/02-inspector-custom.conf",
			SubPath:   "02-inspector-custom.conf",
			ReadOnly:  true,
		},
		corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	)
}

// GetHttpbootVolumeMounts - httpboot's own httpd instance (port 8088),
// serving PXE assets. inspector.ipxe is mounted directly into the shared
// var-lib-ironic EmptyDir tree (already included via getCommonVolumeMounts) --
// a nested mount at a specific sub-path, unrelated to the SubPath-of-a-
// not-yet-existing-EmptyDir-path gotcha, since this mounts from the
// "config" Secret, not from var-lib-ironic itself.
func GetHttpbootVolumeMounts() []corev1.VolumeMount {
	return append(getCommonVolumeMounts(),
		corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpboot-httpd.conf",
			ReadOnly:  true,
		},
		corev1.VolumeMount{
			Name:      "config",
			MountPath: "/var/lib/ironic/httpboot/inspector.ipxe",
			SubPath:   "inspector.ipxe",
			ReadOnly:  true,
		},
		GetRunHttpdVolumeMount(),
	)
}

// GetDnsmasqVolumeMounts - dnsmasq reads its real, final config from
// /etc/dnsmasq.conf, SubPath-mounted from the SAME "var-lib-ironic" EmptyDir
// pxe-init (an earlier-running init container in this same pod) already
// wrote the dynamically-generated dnsmasq.conf into via a whole-directory
// mount -- by the time this container starts, the file already exists, so
// this SubPath mount is safe (see the doc comment on horizon-operator's
// equivalent fix for why a SubPath mount of a *not-yet-existing* path would
// otherwise be auto-created as a directory by kubelet).
func GetDnsmasqVolumeMounts() []corev1.VolumeMount {
	return append(getCommonVolumeMounts(),
		corev1.VolumeMount{
			Name:      "var-lib-ironic",
			MountPath: "/etc/dnsmasq.conf",
			SubPath:   "dnsmasq.conf",
			ReadOnly:  true,
		},
	)
}

// GetRamdiskLogsVolumeMounts - ramdisk-logs has no config of its own
// (kolla's own config.json for it never listed any config_files either) --
// just the common mounts, so it can watch var-lib-ironic for new ramdisk
// logs.
func GetRamdiskLogsVolumeMounts() []corev1.VolumeMount {
	return getCommonVolumeMounts()
}
