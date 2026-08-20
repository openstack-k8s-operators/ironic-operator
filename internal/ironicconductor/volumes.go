package ironicconductor

import (
	"context"
	"fmt"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ironic-operator/internal/ironic"
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GetVolumes -
func GetVolumes(ctx context.Context, instance *ironicv1.IronicConductor) []corev1.Volume {
	Log := log.FromContext(ctx).WithName("IronicConductor").WithName("GetVolumes")
	var config0440AccessMode int32 = 0440
	parentName := ironicv1.GetOwningIronicName(instance)

	var conductorVolumes []corev1.Volume

	// Only include config-data-custom volume when parentName is present
	if parentName != "" {
		conductorVolumes = append(conductorVolumes,
			corev1.Volume{
				Name: "config-data-custom",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						DefaultMode: &config0440AccessMode,
						SecretName:  fmt.Sprintf("%s-config-data", parentName),
					},
				},
			})
	} else {
		Log.Info("parentName is not present for IronicConductor instance", "instance", instance.Name, "namespace", instance.Namespace)
	}

	conductorVolumes = append(conductorVolumes, volume.WritableDirVolume(volume.RunHttpdVolumeName))

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

// varLibIronicVolumeMount - shared by every conductor container: pxe-init
// (an earlier init container) writes boot assets and the dynamically
// -generated dnsmasq.conf here via this same whole-directory mount before
// any of these containers start.
func varLibIronicVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "var-lib-ironic",
		MountPath: "/var/lib/ironic",
		ReadOnly:  false,
	}
}

// GetConductorVolumeMounts - the main ironic-conductor process. Each file
// mounted directly at its final destination via SubPath from the same
// Secret/EmptyDir config.json used to stage-then-copy.
// 03-init-container-conductor.conf is the one file this migration's
// EmptyDir-seed pattern applies to (see ironic.GetMergedConfVolumeMount).
func GetConductorVolumeMounts(instance *ironicv1.IronicConductor) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/ironic/ironic.conf",
			SubPath:   "ironic.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/ironic/ironic.conf.d/01-conductor.conf",
			SubPath:   "01-conductor.conf",
			ReadOnly:  true,
		},
		ironic.GetMergedConfVolumeMount("/etc/ironic/ironic.conf.d/03-init-container-conductor.conf", "03-init-container-conductor.conf"),
		{
			Name:      "config-data",
			MountPath: "/etc/ironic/ironic.conf.d/04-conductor-custom.conf",
			SubPath:   "04-conductor-custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		varLibIronicVolumeMount(),
	}

	// 02-ironic-custom.conf is cross-cutting (shared by api/conductor/
	// inspector), stored in the parent Ironic CR's own "-config-data"
	// Secret ("config-data-custom" here), not this component's own.
	if ironicv1.GetOwningIronicName(instance) != "" {
		vm = append(vm, corev1.VolumeMount{
			Name:      "config-data-custom",
			MountPath: "/etc/ironic/ironic.conf.d/02-ironic-custom.conf",
			SubPath:   "02-ironic-custom.conf",
			ReadOnly:  true,
		})
	}

	return append(ironic.GetVolumeMounts(), vm...)
}

// GetHttpbootVolumeMounts - conductor's own httpboot httpd instance (port
// 8088), serving PXE assets pxe-init wrote into var-lib-ironic.
func GetHttpbootVolumeMounts() []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpboot-httpd.conf",
			ReadOnly:  true,
		},
		varLibIronicVolumeMount(),
		volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath),
	}
	return append(ironic.GetVolumeMounts(), vm...)
}

// GetDnsmasqVolumeMounts - dnsmasq reads its real, final config from
// /etc/dnsmasq.conf, SubPath-mounted from the same "var-lib-ironic" volume
// pxe-init already wrote the dynamically-generated dnsmasq.conf into via a
// whole-directory mount -- safe for the same reason as
// ironicinspector.GetDnsmasqVolumeMounts().
func GetDnsmasqVolumeMounts() []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "var-lib-ironic",
			MountPath: "/etc/dnsmasq.conf",
			SubPath:   "dnsmasq.conf",
			ReadOnly:  true,
		},
		varLibIronicVolumeMount(),
	}
	return append(ironic.GetVolumeMounts(), vm...)
}

// GetRamdiskLogsVolumeMounts - no config of its own, just var-lib-ironic to
// watch for new ramdisk logs.
func GetRamdiskLogsVolumeMounts() []corev1.VolumeMount {
	return append(ironic.GetVolumeMounts(), varLibIronicVolumeMount())
}
