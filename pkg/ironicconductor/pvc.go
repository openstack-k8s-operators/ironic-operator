package ironicconductor

import (
	"fmt"
	"strings"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Pvc - Returns the deployment object for the Database
func Pvc(instance *ironicv1.IronicConductor) *corev1.PersistentVolumeClaim {
	labels := map[string]string{
		common.AppSelector:            ironic.ServiceName,
		common.ComponentSelector:      ironic.ConductorComponent,
		ironic.ConductorGroupSelector: ironicv1.ConductorGroupNull,
	}
	pvcName := fmt.Sprintf("%s-%s", ironic.ServiceName, ironic.ConductorComponent)
	if instance.Spec.ConductorGroup != "" {
		pvcName = strings.ToLower(fmt.Sprintf("%s-%s", pvcName, instance.Spec.ConductorGroup))
		labels[ironic.ConductorGroupSelector] = strings.ToLower(instance.Spec.ConductorGroup)
	}
	pv := &corev1.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:      pvcName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(instance.Spec.StorageRequest),
				},
			},
			StorageClassName: &instance.Spec.StorageClass,
		},
	}
	return pv
}
