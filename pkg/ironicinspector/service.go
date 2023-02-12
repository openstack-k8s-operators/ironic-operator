package ironicinspector

import (
	"context"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Service - Service for conductor pod services
func Service(
	serviceName string,
	instance *ironicv1.IronicInspector,
	serviceLabels map[string]string,
) *corev1.Service {

	var ports []corev1.ServicePort

	// There is no provision network so expose the deploy HTTP interface
	// as a service to enable virtual media boot
	if instance.Spec.InspectionNetwork == "" {
		httpbootPort := corev1.ServicePort{
			Name:     ironic.HttpbootComponent,
			Port:     8088,
			Protocol: corev1.ProtocolTCP,
		}
		ports = append(ports, httpbootPort)
	}

	if len(ports) == 0 {
		return nil
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

// Route - Route for httpboot service when no inspection network
func Route(
	serviceName string,
	instance *ironicv1.IronicInspector,
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

// IngressDomain - List existing conductor routes to extract the IngressDomain
func IngressDomain(
	ctx context.Context,
	instance *ironicv1.IronicInspector,
	helper *helper.Helper,
	routeLabels map[string]string,
) string {
	routeList := &routev1.RouteList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	labels := client.MatchingLabels(routeLabels)
	listOpts = append(listOpts, labels)
	err := helper.GetClient().List(ctx, routeList, listOpts...)
	if err != nil {
		return ""
	}

	if len(routeList.Items) == 0 {
		return ""
	}
	hostname := routeList.Items[0].Spec.Host
	return strings.SplitN(hostname, ".", 2)[1]
}
