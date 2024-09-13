/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ironicinspector

import (
	"net"

	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	k8snet "k8s.io/utils/net"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
	// IronicInspectorHttpdCommand -
	IronicInspectorHttpdCommand = "/usr/sbin/httpd -DFOREGROUND"
)

// StatefulSet func
func StatefulSet(
	instance *ironicv1.IronicInspector,
	configHash string,
	labels map[string]string,
	ingressDomain string,
	annotations map[string]string,
) (*appsv1.StatefulSet, error) {
	runAsUser := int64(0)

	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 3,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 3,
	}
	startupProbe := &corev1.Probe{
		FailureThreshold: 6,
		PeriodSeconds:    10,
	}
	dnsmasqLivenessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 3,
	}
	dnsmasqReadinessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 3,
	}
	httpbootLivenessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	httpbootReadinessProbe := &corev1.Probe{
		TimeoutSeconds:      10,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}

	args := []string{"-c", ServiceCommand}

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/v1",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(IronicInspectorInternalPort)},
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/v1",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(IronicInspectorInternalPort)},
	}
	startupProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/v1",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(IronicInspectorInternalPort)},
	}
	if instance.Spec.TLS.API.Enabled(service.EndpointPublic) {
		livenessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		readinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		startupProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
	}

	// (TODO): Use http request if we can create a good request path
	httpbootLivenessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8088)},
	}
	httpbootReadinessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8088)},
	}

	// dnsmasq only listen on ports 67 and/or 547 when DHCPRanges are configured.
	dnsmasqProbeCommand := []string{"sh", "-c", "ss -lun | grep :69"}
	ipv6Probe := false
	ipv4Probe := false
	for _, dhcpRangeSpec := range instance.Spec.DHCPRanges {
		_, ipPrefix, _ := net.ParseCIDR(dhcpRangeSpec.Cidr)
		if k8snet.IsIPv4CIDR(ipPrefix) {
			ipv4Probe = true
		}
		if k8snet.IsIPv6CIDR(ipPrefix) {
			ipv6Probe = true
		}
	}
	if ipv4Probe && !ipv6Probe {
		dnsmasqProbeCommand = []string{"sh", "-c", "ss -lun | grep :67 && ss -lun | grep :69"}
	} else if !ipv4Probe && ipv6Probe {
		dnsmasqProbeCommand = []string{"sh", "-c", "ss -lun | grep :547 && ss -lun | grep :69"}
	} else if ipv4Probe && ipv6Probe {
		dnsmasqProbeCommand = []string{"sh", "-c", "ss -lun | grep :547 && ss -lun | grep :67 && ss -lun | grep :69"}
	}
	dnsmasqLivenessProbe.Exec = &corev1.ExecAction{Command: dnsmasqProbeCommand}
	dnsmasqReadinessProbe.Exec = &corev1.ExecAction{Command: dnsmasqProbeCommand}

	// create Volume and VolumeMounts
	volumes := GetVolumes(ironic.ServiceName + "-" + ironic.InspectorComponent)
	httpbootVolumeMounts := GetVolumeMounts("httpboot")
	httpdVolumeMounts := GetVolumeMounts("httpd")
	inspectorVolumeMounts := GetVolumeMounts("ironic-inspector")
	dnsmasqVolumeMounts := GetVolumeMounts("dnsmasq")
	ramdiskLogsVolumeMounts := GetVolumeMounts("ramdisk-logs")
	initVolumeMounts := GetInitVolumeMounts()

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		httpdVolumeMounts = append(httpdVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		inspectorVolumeMounts = append(inspectorVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		httpbootVolumeMounts = append(httpbootVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		dnsmasqVolumeMounts = append(dnsmasqVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		ramdiskLogsVolumeMounts = append(ramdiskLogsVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		initVolumeMounts = append(initVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case service.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case service.EndpointInternal:
				tlsEndptCfg = instance.Spec.TLS.API.Internal
			}

			svc, err := tlsEndptCfg.ToService()
			if err != nil {
				return nil, err
			}
			volumes = append(volumes, svc.CreateVolume(endpt.String()))
			httpdVolumeMounts = append(httpdVolumeMounts, svc.CreateVolumeMounts(endpt.String())...)
		}
	}

	containers := []corev1.Container{}

	httpdEnvVars := map[string]env.Setter{}
	httpdEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	httpdEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
	httpdContainer := corev1.Container{
		Name:  ironic.ServiceName + "-" + ironic.InspectorComponent + "-httpd",
		Image: instance.Spec.ContainerImage,
		Command: []string{
			"/bin/bash",
		},
		Args:            args,
		SecurityContext: &corev1.SecurityContext{RunAsUser: &runAsUser},
		Env:             env.MergeEnvs([]corev1.EnvVar{}, httpdEnvVars),
		VolumeMounts:    httpdVolumeMounts,
		Resources:       instance.Spec.Resources,
		ReadinessProbe:  readinessProbe,
		LivenessProbe:   livenessProbe,
		StartupProbe:    startupProbe,
	}
	containers = append(containers, httpdContainer)

	inspectorEnvVars := map[string]env.Setter{}
	inspectorEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	inspectorEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
	inspectorContainer := corev1.Container{
		Name:  ironic.ServiceName + "-" + ironic.InspectorComponent,
		Image: instance.Spec.ContainerImage,
		Command: []string{
			"/bin/bash",
		},
		Args:            args,
		SecurityContext: &corev1.SecurityContext{RunAsUser: &runAsUser},
		Env:             env.MergeEnvs([]corev1.EnvVar{}, inspectorEnvVars),
		VolumeMounts:    inspectorVolumeMounts,
		Resources:       instance.Spec.Resources,
		ReadinessProbe:  readinessProbe,
		LivenessProbe:   livenessProbe,
		StartupProbe:    startupProbe,
	}
	containers = append(containers, inspectorContainer)

	httpbootEnvVars := map[string]env.Setter{}
	httpbootEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	httpbootEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
	httpbootContainer := corev1.Container{
		Name:  ironic.InspectorComponent + "-" + "httpboot",
		Image: instance.Spec.PxeContainerImage,
		Command: []string{
			"/bin/bash",
		},
		Args: args,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		Env:            env.MergeEnvs([]corev1.EnvVar{}, httpbootEnvVars),
		VolumeMounts:   httpbootVolumeMounts,
		Resources:      instance.Spec.Resources,
		ReadinessProbe: httpbootReadinessProbe,
		LivenessProbe:  httpbootLivenessProbe,
		// StartupProbe:   startupProbe,
	}
	containers = append(containers, httpbootContainer)

	ramdiskLogsEnvVars := map[string]env.Setter{}
	ramdiskLogsEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	ramdiskLogsEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
	ramdiskLogsContainer := corev1.Container{
		Name: "ramdisk-logs",
		Command: []string{
			"/bin/bash",
		},
		Args:         args,
		Image:        instance.Spec.ContainerImage,
		Env:          env.MergeEnvs([]corev1.EnvVar{}, ramdiskLogsEnvVars),
		VolumeMounts: ramdiskLogsVolumeMounts,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
	}
	containers = append(containers, ramdiskLogsContainer)

	if instance.Spec.InspectionNetwork != "" {
		// Only include dnsmasq container if there is an inspection network
		dnsmasqEnvVars := map[string]env.Setter{}
		dnsmasqEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
		dnsmasqEnvVars["CONFIG_HASH"] = env.SetValue(configHash)
		dnsmasqContainer := corev1.Container{
			Name:  ironic.InspectorComponent + "-" + "dnsmasq",
			Image: instance.Spec.ContainerImage,
			Command: []string{
				"/bin/bash",
			},
			Args: args,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{
						"NET_ADMIN", "NET_RAW",
					},
				},
			},
			Env:            env.MergeEnvs([]corev1.EnvVar{}, dnsmasqEnvVars),
			VolumeMounts:   dnsmasqVolumeMounts,
			Resources:      instance.Spec.Resources,
			ReadinessProbe: dnsmasqReadinessProbe,
			LivenessProbe:  dnsmasqLivenessProbe,
			// StartupProbe:   startupProbe,
		}
		containers = append(containers, dnsmasqContainer)
	}

	// Default oslo.service graceful_shutdown_timeout is 60, so align with that
	terminationGracePeriod := int64(60)

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ironic.ServiceName + "-" + ironic.InspectorComponent,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas:            instance.Spec.Replicas,
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            instance.RbacResourceName(),
					Containers:                    containers,
					TerminationGracePeriodSeconds: &terminationGracePeriod,
					Volumes:                       volumes,
				},
			},
		},
	}

	// If possible two pods of the same service should not
	// run on the same worker node. If this is not possible
	// the get still created on the same worker node.
	statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
		common.AppSelector,
		[]string{ironic.ServiceName},
		corev1.LabelHostname,
	)
	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		statefulset.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	// init.sh needs to detect and set InspectionNetworkIP
	inspectorHTTPURL := "http://%(InspectorNetworkIP)s:8088/"
	if instance.Spec.InspectionNetwork == "" {
		// Build what the fully qualified Route hostname will be when the Route exists
		inspectorHTTPURL = "http://%(PodName)s-%(PodNamespace)s.%(IngressDomain)s/"
	}

	initContainerDetails := APIDetails{
		ContainerImage:         instance.Spec.ContainerImage,
		PxeContainerImage:      instance.Spec.PxeContainerImage,
		IronicPythonAgentImage: instance.Spec.IronicPythonAgentImage,
		ImageDirectory:         ironic.ImageDirectory,
		DatabaseHost:           instance.Status.DatabaseHostname,
		DatabaseName:           DatabaseName,
		OSPSecret:              instance.Spec.Secret,
		TransportURLSecret:     instance.Status.TransportURLSecret,
		UserPasswordSelector:   instance.Spec.PasswordSelectors.Service,
		VolumeMounts:           initVolumeMounts,
		PxeInit:                true,
		IpaInit:                true,
		InspectorHTTPURL:       inspectorHTTPURL,
		IngressDomain:          ingressDomain,
		InspectionNetwork:      instance.Spec.InspectionNetwork,
	}
	statefulset.Spec.Template.Spec.InitContainers = InitContainer(initContainerDetails)

	return statefulset, nil
}
