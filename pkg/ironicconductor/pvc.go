package ironicconductor

import (
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
		common.AppSelector:       ironic.ServiceName,
		ironic.ComponentSelector: ironic.ConductorComponent,
	}
	pv := &corev1.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:      ironic.ServiceName + "-" + ironic.ConductorComponent,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(instance.Spec.StorageRequest),
				},
			},
			StorageClassName: &instance.Spec.StorageClass,
		},
	}
	return pv
}
