package ironicconductor

import (
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service - Service for conductor pod services
func Service(
	serviceName string,
	instance *ironicv1.IronicConductor,
	serviceLabels map[string]string,
) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: serviceLabels,
			Ports: []corev1.ServicePort{
				{
					Name:     ironic.JSONRPCComponent,
					Port:     8089,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
}
