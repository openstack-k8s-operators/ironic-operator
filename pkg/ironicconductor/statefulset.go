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

package ironicconductor

import (
	"fmt"
	"net"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	ironic "github.com/openstack-k8s-operators/ironic-operator/pkg/ironic"
	common "github.com/openstack-k8s-operators/lib-common/modules/common"
	affinity "github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	env "github.com/openstack-k8s-operators/lib-common/modules/common/env"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	k8snet "k8s.io/utils/net"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
)

// StatefulSet func
func StatefulSet(
	instance *ironicv1.IronicConductor,
	configHash string,
	labels map[string]string,
	ingressDomain string,
	annotations map[string]string,
	topology *topologyv1.Topology,
) (*appsv1.StatefulSet, error) {
	runAsUser := int64(0)

	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      5,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
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

	if instance.Spec.RPCTransport == "json-rpc" {
		// (TODO) Make a http request to the JSON-RPC port ?
		livenessProbe.TCPSocket = &corev1.TCPSocketAction{
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8089)},
		}
		readinessProbe.TCPSocket = &corev1.TCPSocketAction{
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8089)},
		}
	} else {
		// TODO
		livenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
		// TODO
		readinessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/bin/true",
			},
		}
	}

	// (TODO): Use http request if we can create a good request path
	httpbootLivenessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8088)},
	}
	httpbootReadinessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8088)},
	}

	// Parse the storageRequest defined in the CR
	storageRequest, err := resource.ParseQuantity(instance.Spec.StorageRequest)
	if err != nil {
		return nil, err
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

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	dnsmasqEnvVars := map[string]env.Setter{}
	dnsmasqEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	dnsmasqEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	httpbootEnvVars := map[string]env.Setter{}
	httpbootEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	httpbootEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	ramdiskLogsEnvVars := map[string]env.Setter{}
	ramdiskLogsEnvVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	ramdiskLogsEnvVars["CONFIG_HASH"] = env.SetValue(configHash)

	volumes := GetVolumes(instance)
	conductorVolumeMounts := GetVolumeMounts("ironic-conductor")
	httpbootVolumeMounts := GetVolumeMounts("httpboot")
	dnsmasqVolumeMounts := GetVolumeMounts("dnsmasq")
	ramdiskLogsVolumeMounts := GetVolumeMounts("ramdisk-logs")
	initVolumeMounts := GetInitVolumeMounts(instance)

	// Add the CA bundle
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		conductorVolumeMounts = append(conductorVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		httpbootVolumeMounts = append(httpbootVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		dnsmasqVolumeMounts = append(dnsmasqVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		ramdiskLogsVolumeMounts = append(ramdiskLogsVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		initVolumeMounts = append(initVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	resourceName := fmt.Sprintf("%s-%s", ironic.ServiceName, ironic.ConductorComponent)
	conductorContainer := corev1.Container{
		Name: resourceName,
		Command: []string{
			"/bin/bash",
		},
		Args:  args,
		Image: instance.Spec.ContainerImage,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser: &runAsUser,
		},
		Env:            env.MergeEnvs([]corev1.EnvVar{}, envVars),
		VolumeMounts:   conductorVolumeMounts,
		Resources:      instance.Spec.Resources,
		ReadinessProbe: readinessProbe,
		LivenessProbe:  livenessProbe,
		// StartupProbe:   startupProbe,
	}
	httpbootContainer := corev1.Container{
		Name: "httpboot",
		Command: []string{
			"/bin/bash",
		},
		Args:  args,
		Image: instance.Spec.PxeContainerImage,
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
		// inotifywait doesn't terminate on SIGTERM so call SIGKILL as a
		// pre-stop command
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"/usr/bin/pkill",
						"inotifywait",
					},
				},
			},
		},
	}

	containers := []corev1.Container{
		conductorContainer,
		httpbootContainer,
		ramdiskLogsContainer,
	}

	if instance.Spec.ProvisionNetwork != "" {
		// Only include the dnsmasq container if there is a provisioning network to listen on.
		dnsmasqContainer := corev1.Container{
			Name: "dnsmasq",
			Command: []string{
				"/bin/bash",
			},
			Args:  args,
			Image: instance.Spec.PxeContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{
						"NET_ADMIN",
						"NET_RAW",
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
		containers = []corev1.Container{
			conductorContainer,
			httpbootContainer,
			dnsmasqContainer,
		}
	}

	// Default oslo.service graceful_shutdown_timeout is 60, so align with that
	terminationGracePeriod := int64(60)

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
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
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "var-lib-ironic",
						Labels: labels,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							"ReadWriteOnce",
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: storageRequest,
							},
						},
						StorageClassName: &instance.Spec.StorageClass,
					},
				},
			},
		},
	}
	if instance.Spec.NodeSelector != nil {
		statefulset.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}
	if topology != nil {
		topology.ApplyTo(&statefulset.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				ironic.ServiceName,
			},
			corev1.LabelHostname,
		)
	}

	// init.sh needs to detect and set ProvisionNetworkIP
	deployHTTPURL := "http://%(ProvisionNetworkIP)s:8088/"
	if instance.Spec.ProvisionNetwork == "" {
		// Build what the fully qualified Route hostname will be when the Route exists
		deployHTTPURL = "http://%(PodName)s-%(PodNamespace)s.%(IngressDomain)s/"
	}

	initContainerDetails := ironic.APIDetails{
		ContainerImage:         instance.Spec.ContainerImage,
		PxeContainerImage:      instance.Spec.PxeContainerImage,
		IronicPythonAgentImage: instance.Spec.IronicPythonAgentImage,
		ImageDirectory:         ironic.ImageDirectory,
		DatabaseHost:           instance.Spec.DatabaseHostname,
		DatabaseName:           ironic.DatabaseName,
		OSPSecret:              instance.Spec.Secret,
		TransportURLSecret:     instance.Spec.TransportURLSecret,
		UserPasswordSelector:   instance.Spec.PasswordSelectors.Service,
		VolumeMounts:           initVolumeMounts,
		PxeInit:                true,
		ConductorInit:          true,
		Privileged:             true,
		DeployHTTPURL:          deployHTTPURL,
		IngressDomain:          ingressDomain,
		ProvisionNetwork:       instance.Spec.ProvisionNetwork,
	}
	statefulset.Spec.Template.Spec.InitContainers = ironic.InitContainer(initContainerDetails)

	return statefulset, nil
}
