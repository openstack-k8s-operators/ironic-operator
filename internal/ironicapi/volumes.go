package ironicapi

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
func GetVolumes(ctx context.Context, instance *ironicv1.IronicAPI) []corev1.Volume {
	Log := log.FromContext(ctx).WithName("IronicAPI").WithName("GetVolumes")
	var config0440AccessMode int32 = 0440
	parentName := ironicv1.GetOwningIronicName(instance)

	var apiVolumes []corev1.Volume

	if parentName == "" {
		Log.Info("parentName is not present for IronicAPI instance", "instance", instance.Name, "namespace", instance.Namespace)
		// Only include logs volume when parentName is not present
		apiVolumes = append(apiVolumes,
			volume.WritableDirVolume("logs"))
	} else {
		// Include both volumes when parentName is present
		apiVolumes = append(apiVolumes,
			corev1.Volume{
				Name: "config-data-custom",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						DefaultMode: &config0440AccessMode,
						SecretName:  fmt.Sprintf("%s-config-data", parentName),
					},
				},
			},
			volume.WritableDirVolume("logs"))
	}

	apiVolumes = append(apiVolumes, volume.WritableDirVolume(volume.RunHttpdVolumeName))

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

// GetRunHttpdVolumeMount - writable emptyDir for httpd's PID file directory,
// needed once httpd runs as a non-root, FSGroup-only user (kolla used to
// chown /etc/httpd/run at startup). Mounted at /run/httpd, not /etc/httpd/run
// directly: ironic-api-httpd.conf's "PidFile run/httpd.pid" resolves
// relative to ServerRoot ("/etc/httpd"), and /etc/httpd/run is itself a
// symlink to ../../run/httpd in the image.
func GetRunHttpdVolumeMount() corev1.VolumeMount {
	return volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath)
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

// GetVolumeMounts - Ironic API VolumeMounts. Each file is mounted directly
// at its final destination via SubPath from the same "config-data" Secret
// config.json used to stage-then-copy -- note this bypasses the shared
// "config-data-merged" EmptyDir entirely (ironic.GetVolumeMounts()'s mount
// of it is still inherited below since the init container's merge step
// still runs unconditionally, but ironic-api-config.json never actually
// sourced from "merged/" in the first place, only conductor does).
func GetVolumeMounts(instance *ironicv1.IronicAPI) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/ironic/ironic.conf",
			SubPath:   "ironic.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/ironic/ironic.conf.d/01-api.conf",
			SubPath:   "01-api.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/ironic/ironic.conf.d/03-api-custom.conf",
			SubPath:   "03-api-custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "ironic-api-httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		GetLogVolumeMount(),
		GetRunHttpdVolumeMount(),
	}

	// 02-ironic-custom.conf is cross-cutting (shared by api/conductor/
	// inspector), stored in the parent Ironic CR's own "-config-data"
	// Secret ("config-data-custom" here), not this component's own.
	parentName := ironicv1.GetOwningIronicName(instance)
	if parentName != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "config-data-custom",
			MountPath: "/etc/ironic/ironic.conf.d/02-ironic-custom.conf",
			SubPath:   "02-ironic-custom.conf",
			ReadOnly:  true,
		})
	}

	return append(ironic.GetVolumeMounts(), volumeMounts...)
}
