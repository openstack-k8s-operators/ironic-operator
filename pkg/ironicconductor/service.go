package ironicconductor

import (
	routev1 "github.com/openshift/api/route/v1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Service - Service for conductor pod services
func Service(
	serviceName string,
	instance *ironicv1.IronicConductor,
	serviceLabels map[string]string,
) *corev1.Service {

	var ports []corev1.ServicePort
	jsonRPCPort := corev1.ServicePort{
		Name:     ironic.JSONRPCComponent,
		Port:     8089,
		Protocol: corev1.ProtocolTCP,
	}

	if instance.Spec.ProvisionNetwork == "" {
		// There is no provision network so expose the deploy HTTP interface
		// as a service to enable virtual media boot
		httpbootPort := corev1.ServicePort{
			Name:     ironic.HttpbootComponent,
			Port:     8088,
			Protocol: corev1.ProtocolTCP,
		}
		ports = []corev1.ServicePort{
			jsonRPCPort,
			httpbootPort,
		}
	} else {
		ports = []corev1.ServicePort{
			jsonRPCPort,
		}
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: serviceLabels,
			Ports:    ports,
		},
	}
}

// Route - Route for httpboot service when no provisioning network
func Route(
	serviceName string,
	instance *ironicv1.IronicConductor,
	routeLabels map[string]string,
) *routev1.Route {
	serviceRef := routev1.RouteTargetReference{
		Kind: "Service",
		Name: serviceName,
	}
	routePort := &routev1.RoutePort{
		TargetPort: intstr.FromString(ironic.HttpbootComponent),
	}
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    routeLabels,
		},
		Spec: routev1.RouteSpec{
			To:   serviceRef,
			Port: routePort,
		},
	}
}
