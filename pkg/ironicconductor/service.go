package ironicconductor

import (
	routev1 "github.com/openshift/api/route/v1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// Service - Service for conductor pod services
func Service(
	serviceName string,
	instance *ironicv1.IronicConductor,
	serviceLabels map[string]string,
	externalIPs []string,
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
				{
					Name:     ironic.HttpbootComponent,
					Port:     8088,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Name:     ironic.DhcpComponent,
					Port:     67,
					Protocol: corev1.ProtocolUDP,
				},
			},
			// ExternalIPs: externalIPs,
		},
	}
}

// HttpbootRoute - Route for conductor httpboot service
func HttpbootRoute(
	serviceName string,
	instance *ironicv1.IronicConductor,
	serviceLabels map[string]string,
) *routev1.Route {

	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName + "-" + ironic.HttpbootComponent,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: serviceName,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(ironic.HttpbootComponent),
			},
		},
	}
}

// DhcpRoute - Route for conductor httpboot service
func DhcpRoute(
	serviceName string,
	instance *ironicv1.IronicConductor,
	serviceLabels map[string]string,
) *routev1.Route {

	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName + "-" + ironic.DhcpComponent,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: serviceName,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(ironic.DhcpComponent),
			},
		},
	}
}
